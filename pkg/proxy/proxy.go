package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/auth"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/config"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/discovery"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/logger"
)

// Manager manages multiple proxy instances
type Manager struct {
	config            *config.Config
	proxies           []*Proxy
	tokenSource       *auth.IAMTokenProvider
	authPassword      string // For Redis password auth
	authorizationMode string // From discovery: IAM_AUTH, PASSWORD_AUTH, AUTH_DISABLED
	tlsConfig         *tls.Config
	nodeMap           map[string]string // Maps remote "ip:port" -> local "ip:port" for cluster redirects
	isClusterMode     bool              // True if cluster mode is detected
	mu                sync.Mutex
}

// Proxy represents a single proxy instance
type Proxy struct {
	localAddr     string
	remoteAddr    string
	endpoint      discovery.Endpoint
	listener      net.Listener
	config        *config.Config
	tokenSource   *auth.IAMTokenProvider
	authPassword  string // For Redis password auth
	tlsConfig     *tls.Config
	isClusterMode bool              // True if cluster mode redirect rewriting is enabled
	nodeMap       map[string]string // Maps remote "ip:port" -> local "ip:port" for cluster redirects
	connections   sync.WaitGroup
	shutdown      chan struct{}
	shutdownOnce  sync.Once
}

// NewManager creates a new proxy manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config:  cfg,
		proxies: make([]*Proxy, 0),
		nodeMap: make(map[string]string),
	}
}

// SetTLSConfig sets the TLS configuration for all proxies
func (m *Manager) SetTLSConfig(caCert string, skipVerify bool) error {
	if caCert != "" {
		// Create a certificate pool with the CA certificate
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
			return fmt.Errorf("failed to parse CA certificate")
		}

		m.tlsConfig = &tls.Config{
			RootCAs:            caCertPool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipVerify,
		}

		logger.Info("TLS configuration initialized with instance CA certificate")
	} else {
		// No CA cert provided
		m.tlsConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipVerify,
		}

		if skipVerify {
			logger.Info("TLS configuration initialized (certificate verification disabled)")
		} else {
			logger.Info("TLS configuration initialized with system CA certificates")
		}
	}

	return nil
}

// SetAuthPassword sets the password for Redis authentication
func (m *Manager) SetAuthPassword(password string) {
	m.authPassword = password
	if password != "" {
		logger.Info("Password authentication configured")
	}
}

// SetAuthorizationMode sets the authorization mode from discovery
func (m *Manager) SetAuthorizationMode(mode string) {
	m.authorizationMode = mode
	logger.Info(fmt.Sprintf("Authorization mode: %s", mode))
}

// AddProxy adds and starts a new proxy
func (m *Manager) AddProxy(ctx context.Context, endpoint discovery.Endpoint, localPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize token source if IAM auth is discovered AND no password is set (shared across all proxies)
	// Password auth takes precedence over IAM auth
	if m.authorizationMode == "IAM_AUTH" && m.authPassword == "" && m.tokenSource == nil {
		tokenSource, err := auth.NewIAMTokenProvider(ctx)
		if err != nil {
			return fmt.Errorf("failed to create IAM token provider: %w", err)
		}
		m.tokenSource = tokenSource
		logger.Info("IAM authentication initialized")
	}

	localAddr := fmt.Sprintf("%s:%d", m.config.LocalAddr, localPort)
	remoteAddr := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	proxy := &Proxy{
		localAddr:     localAddr,
		remoteAddr:    remoteAddr,
		endpoint:      endpoint,
		config:        m.config,
		tokenSource:   m.tokenSource,
		authPassword:  m.authPassword,
		tlsConfig:     m.tlsConfig,
		isClusterMode: m.isClusterMode,
		nodeMap:       m.nodeMap,
		shutdown:      make(chan struct{}),
	}

	if err := proxy.Start(); err != nil {
		return err
	}

	// Track this node in the map for cluster redirect rewriting
	m.nodeMap[remoteAddr] = localAddr

	m.proxies = append(m.proxies, proxy)
	return nil
}

// Shutdown shuts down all proxies
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, proxy := range m.proxies {
		proxy.Shutdown()
	}
}

// DiscoverAndAddClusterNodes discovers all nodes in a cluster and creates proxies for them
// Returns the number of additional nodes added (excluding the primary endpoint)
func (m *Manager) DiscoverAndAddClusterNodes(ctx context.Context, primaryEndpoint discovery.Endpoint, startPort int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Connect to the primary endpoint to discover cluster topology
	remoteAddr := net.JoinHostPort(primaryEndpoint.Host, fmt.Sprintf("%d", primaryEndpoint.Port))

	var conn net.Conn
	var err error

	if m.tlsConfig != nil {
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, err = tls.DialWithDialer(dialer, "tcp", remoteAddr, m.tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", remoteAddr, 5*time.Second)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to connect to primary endpoint: %w", err)
	}
	defer conn.Close()

	// Authenticate before running CLUSTER NODES
	if m.authPassword != "" {
		if err := m.authenticatePasswordOnConn(conn, m.authPassword); err != nil {
			return 0, fmt.Errorf("authentication failed: %w", err)
		}
	} else if m.tokenSource != nil {
		if err := m.authenticateIAMOnConn(ctx, conn); err != nil {
			return 0, fmt.Errorf("IAM authentication failed: %w", err)
		}
	}

	// Discover cluster nodes
	nodes, err := DiscoverClusterTopology(conn)
	if err != nil {
		return 0, fmt.Errorf("failed to discover cluster topology: %w", err)
	}

	if len(nodes) == 0 {
		return 0, fmt.Errorf("no cluster nodes found")
	}

	logger.Info(fmt.Sprintf("Discovered %d cluster nodes", len(nodes)))

	// Filter out the current node and duplicates
	newNodes := FilterUniqueNodes(nodes, remoteAddr)

	if len(newNodes) == 0 {
		logger.Info("No additional cluster nodes to proxy (single-node cluster)")
		return 0, nil
	}

	// Enable cluster mode
	m.isClusterMode = true

	// Create proxies for each new node
	addedCount := 0
	for i, node := range newNodes {
		localPort := startPort + i
		endpoint := discovery.Endpoint{
			Host: extractHost(node.Address),
			Port: node.Port,
			Type: fmt.Sprintf("cluster-%s", node.Role),
		}

		// Create proxy without holding the lock (AddProxy needs it)
		m.mu.Unlock()
		err := m.AddProxy(ctx, endpoint, localPort)
		m.mu.Lock()

		if err != nil {
			logger.Error(fmt.Sprintf("Failed to create proxy for cluster node %s: %v", node.Address, err))
			continue
		}

		logger.Info(fmt.Sprintf("Added cluster node proxy: %s:%d -> %s (%s)",
			m.config.LocalAddr, localPort, node.Address, node.Role))
		addedCount++
	}

	return addedCount, nil
}

// authenticatePasswordOnConn performs password authentication on a connection
func (m *Manager) authenticatePasswordOnConn(conn net.Conn, password string) error {
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}

	respStr := string(response[:n])
	if len(respStr) >= 5 && respStr[:5] == "+OK\r\n" {
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
		return nil
	}

	return fmt.Errorf("authentication failed: %s", respStr)
}

// authenticateIAMOnConn performs IAM authentication on a connection
func (m *Manager) authenticateIAMOnConn(ctx context.Context, conn net.Conn) error {
	token, err := m.tokenSource.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get IAM token: %w", err)
	}

	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(token), token)

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}

	respStr := string(response[:n])
	if len(respStr) >= 5 && respStr[:5] == "+OK\r\n" {
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
		return nil
	}

	return fmt.Errorf("authentication failed: %s", respStr)
}

// extractHost extracts the host part from "host:port" address
func extractHost(address string) string {
	if idx := strings.LastIndex(address, ":"); idx != -1 {
		return address[:idx]
	}
	return address
}

// Start starts the proxy server
func (p *Proxy) Start() error {
	listener, err := net.Listen("tcp", p.localAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.localAddr, err)
	}
	p.listener = listener

	go p.acceptConnections()
	return nil
}

// Shutdown gracefully shuts down the proxy
func (p *Proxy) Shutdown() {
	p.shutdownOnce.Do(func() {
		close(p.shutdown)
		if p.listener != nil {
			p.listener.Close()
		}
		// Wait for all connections to finish (with timeout)
		done := make(chan struct{})
		go func() {
			p.connections.Wait()
			close(done)
		}()
		select {
		case <-done:
			logger.Debug(fmt.Sprintf("All connections closed for %s", p.localAddr))
		case <-time.After(5 * time.Second):
			logger.Error(fmt.Sprintf("Timeout waiting for connections to close for %s", p.localAddr))
		}
	})
}

// acceptConnections accepts and handles incoming connections
func (p *Proxy) acceptConnections() {
	for {
		select {
		case <-p.shutdown:
			return
		default:
		}

		// Set a deadline for Accept to allow checking shutdown channel
		p.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

		clientConn, err := p.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-p.shutdown:
				return
			default:
				logger.Error(fmt.Sprintf("Failed to accept connection: %v", err))
				continue
			}
		}

		p.connections.Add(1)
		go p.handleConnection(clientConn)
	}
}

// handleConnection handles a single client connection
func (p *Proxy) handleConnection(clientConn net.Conn) {
	defer p.connections.Done()
	defer clientConn.Close()

	logger.Debug(fmt.Sprintf("New connection from %s to %s", clientConn.RemoteAddr(), p.remoteAddr))

	// Connect to remote Valkey instance
	var remoteConn net.Conn
	var err error

	if p.tlsConfig != nil {
		// Establish TLS connection
		logger.Debug(fmt.Sprintf("Establishing TLS connection to %s", p.remoteAddr))
		dialer := &net.Dialer{
			Timeout: 5 * time.Second,
		}
		remoteConn, err = tls.DialWithDialer(dialer, "tcp", p.remoteAddr, p.tlsConfig)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to establish TLS connection to remote: %v", err))
			return
		}
		logger.Debug("TLS handshake completed successfully")
	} else {
		// Plain TCP connection
		remoteConn, err = net.DialTimeout("tcp", p.remoteAddr, 5*time.Second)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to connect to remote: %v", err))
			return
		}
	}
	defer remoteConn.Close()

	// Enable TCP keepalive for client connection
	if tcpConn, ok := clientConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		// Disable Nagle's algorithm for lower latency
		tcpConn.SetNoDelay(true)
	}

	// Enable TCP keepalive for remote connection (if it's a TCP connection under TLS)
	if tlsConn, ok := remoteConn.(*tls.Conn); ok {
		if tcpConn, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(30 * time.Second)
			tcpConn.SetNoDelay(true)
		}
	} else if tcpConn, ok := remoteConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
	}

	// Perform authentication based on configuration
	// Password auth takes precedence over IAM auth
	if p.authPassword != "" {
		// Password authentication (for Redis instances)
		if err := p.authenticatePassword(remoteConn, p.authPassword); err != nil {
			logger.Error(fmt.Sprintf("Password authentication failed: %v", err))
			return
		}
		logger.Debug("Password authentication successful")
	} else if p.tokenSource != nil {
		// IAM authentication (for Valkey with IAM_AUTH authorization mode)
		if err := p.authenticateIAM(remoteConn); err != nil {
			logger.Error(fmt.Sprintf("IAM authentication failed: %v", err))
			return
		}
		logger.Debug("IAM authentication successful")
	}

	// Choose connection handling strategy based on cluster mode
	if p.isClusterMode {
		// Cluster mode: intercept server responses and rewrite MOVED/ASK redirects
		p.handleClusterConnection(clientConn, remoteConn)
	} else {
		// Non-cluster mode: simple bidirectional copy (current behavior)
		p.handleSimpleConnection(clientConn, remoteConn)
	}

	logger.Debug(fmt.Sprintf("Connection closed: %s", clientConn.RemoteAddr()))
}

// handleSimpleConnection handles bidirectional traffic without protocol inspection
// This is used for non-cluster instances or when IAM auth is not enabled
func (p *Proxy) handleSimpleConnection(clientConn, remoteConn net.Conn) {
	errChan := make(chan error, 2)

	// Client -> Server
	go func() {
		_, err := io.Copy(remoteConn, clientConn)
		errChan <- err
	}()

	// Server -> Client
	go func() {
		_, err := io.Copy(clientConn, remoteConn)
		errChan <- err
	}()

	// Wait for either direction to complete
	<-errChan
}

// handleClusterConnection handles bidirectional traffic with RESP protocol inspection
// Intercepts and rewrites MOVED/ASK responses to use local proxy addresses
func (p *Proxy) handleClusterConnection(clientConn, remoteConn net.Conn) {
	errChan := make(chan error, 2)

	// Client -> Server: simple copy (no interception needed)
	go func() {
		_, err := io.Copy(remoteConn, clientConn)
		if err != nil {
			logger.Debug(fmt.Sprintf("Client->Server copy error: %v", err))
		}
		errChan <- err
	}()

	// Server -> Client: parse RESP and rewrite redirects
	go func() {
		err := p.proxyServerResponses(remoteConn, clientConn)
		if err != nil && err != io.EOF {
			logger.Debug(fmt.Sprintf("Server->Client proxy error: %v", err))
		}
		errChan <- err
	}()

	// Wait for either direction to complete
	<-errChan
}

// proxyServerResponses reads RESP responses from server and rewrites MOVED/ASK redirects
func (p *Proxy) proxyServerResponses(serverConn, clientConn net.Conn) error {
	respReader := NewRESPReader(serverConn)

	for {
		// Read a RESP value from the server
		value, err := respReader.ReadValue()
		if err != nil {
			if err == io.EOF {
				return err
			}
			// If not EOF, it might be a parse error or connection issue
			return fmt.Errorf("failed to read RESP value: %w", err)
		}

		// Check if this is a redirect error and rewrite if needed
		if value.IsRedirectError() {
			if value.RewriteRedirectError(p.nodeMap) {
				logger.Debug(fmt.Sprintf("Rewrote redirect: %s", value.Str))
			} else {
				logger.Debug(fmt.Sprintf("Redirect not rewritten (node not in map): %s", value.Str))
			}
		}

		// Serialize and send to client
		data := value.Serialize()
		if _, err := clientConn.Write(data); err != nil {
			return fmt.Errorf("failed to write to client: %w", err)
		}
	}
}

// authenticateIAM performs IAM authentication with Valkey
func (p *Proxy) authenticateIAM(conn net.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get IAM token
	token, err := p.tokenSource.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get IAM token: %w", err)
	}

	// Send AUTH command using RESP protocol
	// Format: *2\r\n$4\r\nAUTH\r\n$<length>\r\n<token>\r\n
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(token), token)

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}

	// Check for success response (+OK\r\n)
	respStr := string(response[:n])
	if len(respStr) >= 5 && respStr[:5] == "+OK\r\n" {
		// Clear deadlines after successful auth
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
		return nil
	}

	return fmt.Errorf("authentication failed: %s", respStr)
}
