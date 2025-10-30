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

const (
	authResponseBufferSize = 1024 // Buffer size for reading AUTH command responses
)

// isConnectionClosedError checks if an error is a connection closure error
// This includes EOF and "use of closed network connection" errors, which are expected
// when connections are closed normally
func isConnectionClosedError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	errStr := err.Error()
	// Check for various connection closure error messages
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe")
}

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
	tracing       bool              // Enable tracing mode
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

// isPortInUse checks if a port is already in use
func (m *Manager) isPortInUse(port int) bool {
	addr := net.JoinHostPort(m.config.LocalAddr, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return false // Port is not in use
	}
	conn.Close()
	return true // Port is in use
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
		tracing:       m.config.Tracing,
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

	// Enable cluster mode before filtering
	m.mu.Lock()
	m.isClusterMode = true

	// Update all existing proxy instances to enable cluster mode
	// This is critical because primary endpoint proxies were created before cluster discovery
	for _, proxy := range m.proxies {
		proxy.isClusterMode = true
	}
	logger.Debug(fmt.Sprintf("Enabled cluster mode on %d existing proxy instances", len(m.proxies)))
	m.mu.Unlock()

	// Strategy:
	// 1. Primary endpoint is already mapped to localhost:6379 (or startPort)
	// 2. For each node discovered via CLUSTER NODES, create a proxy if it doesn't exist
	// 3. Use sequential local ports (6380, 6381, etc.) for each additional node
	// 4. Map each node's cluster bus port to its corresponding local proxy address

	logger.Debug(fmt.Sprintf("Processing %d discovered nodes for proxy creation and cluster bus port mapping", len(nodes)))

	// First, find which nodes need proxies created (skip primary endpoint)
	primaryRemoteAddr := fmt.Sprintf("%s:%d", primaryEndpoint.Host, primaryEndpoint.Port)
	primaryLocalAddr := fmt.Sprintf("%s:%d", m.config.LocalAddr, startPort)

	// Verify primary endpoint proxy exists
	m.mu.Lock()
	if existingLocalAddr, exists := m.nodeMap[primaryRemoteAddr]; exists {
		primaryLocalAddr = existingLocalAddr
		logger.Debug(fmt.Sprintf("Primary endpoint proxy exists: %s -> %s", primaryRemoteAddr, primaryLocalAddr))
	} else {
		logger.Debug(fmt.Sprintf("Primary endpoint proxy not found in nodeMap, will create at %s", primaryLocalAddr))
	}
	m.mu.Unlock()

	// Create proxies for each discovered node and map their cluster bus ports
	addedCount := 0
	currentPort := startPort + 1 // Start from next port after primary (6380, 6381, etc.)
	nodeIndexToLocalAddr := make(map[int]string)

	// Map primary node first (if we can identify it)
	for i, node := range nodes {
		nodeIP := extractHost(node.Address)
		primaryIP := primaryEndpoint.Host
		if nodeIP == primaryIP {
			nodeIndexToLocalAddr[i] = primaryLocalAddr
			logger.Debug(fmt.Sprintf("Mapped primary node %s to primary proxy: %s", node.ID[:8], primaryLocalAddr))
			break
		}
	}

	// Process all nodes to create proxies and map cluster bus ports
	m.mu.Lock()
	logger.Debug(fmt.Sprintf("Current nodeMap has %d entries before processing", len(m.nodeMap)))
	m.mu.Unlock()

	mappedCount := 0
	for i, node := range nodes {
		nodeIP := extractHost(node.Address)
		clientRemoteAddr := fmt.Sprintf("%s:%d", nodeIP, 6379) // Use standard client port 6379

		logger.Debug(fmt.Sprintf("Processing node %d/%d: %s (IP: %s, cluster bus port: %d)",
			i+1, len(nodes), node.ID[:8], nodeIP, node.ClusterBusPort))

		// Get or create local proxy address for this node
		// IMPORTANT: Create a separate proxy for EACH node, even if they share the same IP,
		// because each node has a unique cluster bus port that CLUSTER SLOTS will return
		var localAddr string

		// Check if this is the primary endpoint node
		if nodeIP == primaryEndpoint.Host {
			// Use primary proxy for primary endpoint node
			localAddr = primaryLocalAddr
			m.mu.Lock()
			m.nodeMap[clientRemoteAddr] = localAddr
			nodeIndexToLocalAddr[i] = localAddr
			m.mu.Unlock()
			logger.Debug(fmt.Sprintf("Using primary proxy for node %s: %s -> %s", node.ID[:8], clientRemoteAddr, localAddr))
		} else {
			// For non-primary nodes, check if we already created a proxy for this specific node
			// Use cluster bus port as part of the key to ensure each node gets its own proxy
			clusterBusKey := fmt.Sprintf("%s:bus:%d", nodeIP, node.ClusterBusPort)

			m.mu.Lock()
			if existingLocalAddr, exists := m.nodeMap[clusterBusKey]; exists {
				// Proxy already exists for this specific node (identified by cluster bus port)
				localAddr = existingLocalAddr
				nodeIndexToLocalAddr[i] = localAddr
				m.mu.Unlock()
				logger.Debug(fmt.Sprintf("Node %s proxy already exists (via cluster bus key): %s -> %s",
					node.ID[:8], clusterBusKey, localAddr))
			} else {
				// Create a new proxy for this specific node
				localPort := currentPort
				currentPort++
				m.mu.Unlock()

				// Check if local port is already in use
				if m.isPortInUse(localPort) {
					logger.Error(fmt.Sprintf("Port %d is already in use, skipping proxy creation for node %s", localPort, node.ID[:8]))
					continue
				}

				localAddr = fmt.Sprintf("%s:%d", m.config.LocalAddr, localPort)

				// For cluster nodes, connect via their cluster bus port
				// In GCP Memorystore clusters, each node is accessible via its unique cluster bus port
				remotePort := node.ClusterBusPort
				if remotePort == 0 {
					// Fallback to client port if cluster bus port is not available
					remotePort = node.Port
					if remotePort == 0 {
						remotePort = 6379 // Final fallback
					}
					logger.Debug(fmt.Sprintf("Node %s has no cluster bus port, using client port %d", node.ID[:8], remotePort))
				}

				endpoint := discovery.Endpoint{
					Host: nodeIP,
					Port: remotePort,
					Type: fmt.Sprintf("cluster-%s", node.Role),
				}

				actualRemoteAddr := fmt.Sprintf("%s:%d", nodeIP, remotePort)
				logger.Debug(fmt.Sprintf("Creating proxy for node %s: %s -> %s (cluster bus port: %d)",
					node.ID[:8], actualRemoteAddr, localAddr, node.ClusterBusPort))

				// Create the proxy
				proxyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := m.AddProxy(proxyCtx, endpoint, localPort)
				cancel()

				if err != nil {
					logger.Error(fmt.Sprintf("Failed to create proxy for node %s (%s:%d): %v", node.ID[:8], nodeIP, remotePort, err))
					continue
				}

				m.mu.Lock()
				// Map both client address and cluster bus key to this proxy
				m.nodeMap[clientRemoteAddr] = localAddr
				m.nodeMap[clusterBusKey] = localAddr
				nodeIndexToLocalAddr[i] = localAddr
				m.mu.Unlock()

				logger.Info(fmt.Sprintf("Created proxy for node %s: %s -> %s (%s)",
					node.ID[:8], clientRemoteAddr, localAddr, node.Role))
				addedCount++
			}
		}

		// Map cluster bus port to this node's local proxy address
		if localAddr != "" && node.ClusterBusPort > 0 {
			m.mu.Lock()
			clusterBusAddr := net.JoinHostPort(nodeIP, fmt.Sprintf("%d", node.ClusterBusPort))
			m.nodeMap[clusterBusAddr] = localAddr
			m.mu.Unlock()
			mappedCount++
			logger.Debug(fmt.Sprintf("✓ Mapped cluster bus address %s -> %s (node: %s)",
				clusterBusAddr, localAddr, node.ID[:8]))
		} else {
			if node.ClusterBusPort == 0 {
				logger.Debug(fmt.Sprintf("Node %s has no cluster bus port, skipping mapping", node.ID[:8]))
			}
		}
	}

	m.mu.Lock()
	logger.Info(fmt.Sprintf("Created %d additional proxies, mapped %d cluster bus port addresses", addedCount, mappedCount))
	logger.Debug(fmt.Sprintf("Final nodeMap has %d entries: %v", len(m.nodeMap), getKeys(m.nodeMap)))
	m.mu.Unlock()

	return addedCount, nil
}

// buildAuthCommand constructs a RESP AUTH command for the given credential
func buildAuthCommand(credential string) string {
	return fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(credential), credential)
}

// sendAuthCommand sends an AUTH command and validates the response
func sendAuthCommand(conn net.Conn, authCmd string) error {
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	response := make([]byte, authResponseBufferSize)
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

// authenticatePasswordOnConn performs password authentication on a connection
func (m *Manager) authenticatePasswordOnConn(conn net.Conn, password string) error {
	authCmd := buildAuthCommand(password)
	return sendAuthCommand(conn, authCmd)
}

// authenticateIAMOnConn performs IAM authentication on a connection
func (m *Manager) authenticateIAMOnConn(ctx context.Context, conn net.Conn) error {
	token, err := m.tokenSource.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get IAM token: %w", err)
	}

	authCmd := buildAuthCommand(token)
	return sendAuthCommand(conn, authCmd)
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
		logger.Debug(fmt.Sprintf("TLS handshake completed successfully, local=%s remote=%s", remoteConn.LocalAddr(), remoteConn.RemoteAddr()))
	} else {
		// Plain TCP connection
		remoteConn, err = net.DialTimeout("tcp", p.remoteAddr, 5*time.Second)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to connect to remote: %v", err))
			return
		}
		logger.Debug(fmt.Sprintf("Connected to remote, local=%s remote=%s", remoteConn.LocalAddr(), remoteConn.RemoteAddr()))
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

// copyAndTrace copies data from src to dst while tracing if tracing is enabled
// inputLabel: label for data coming INTO the proxy from the source
// outputLabel: label for data going OUT of the proxy to the destination
func (p *Proxy) copyAndTrace(src, dst net.Conn, inputLabel, outputLabel string) error {
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			data := buf[:nr]

			// Trace incoming data if tracing is enabled
			if p.tracing {
				logger.Trace(inputLabel, data)
			}

			nw, ew := dst.Write(data)

			// Trace outgoing data if tracing is enabled
			if p.tracing && nw > 0 {
				logger.Trace(outputLabel, data[:nw])
			}

			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			if ew != nil {
				return ew
			}
			if nr != nw {
				return fmt.Errorf("short write: %d != %d", nr, nw)
			}
		}
		if err != nil {
			// Treat connection closure errors as EOF for consistency
			if isConnectionClosedError(err) {
				return io.EOF
			}
			return err
		}
	}
}

// handleSimpleConnection handles bidirectional traffic without protocol inspection
// This is used for non-cluster instances.
func (p *Proxy) handleSimpleConnection(clientConn, remoteConn net.Conn) {
	errChan := make(chan error, 2)

	// Client -> Proxy -> Server
	go func() {
		err := p.copyAndTrace(clientConn, remoteConn, "CLIENT->PROXY", "PROXY->SERVER")
		errChan <- err
	}()

	// Server -> Proxy -> Client
	go func() {
		err := p.copyAndTrace(remoteConn, clientConn, "SERVER->PROXY", "PROXY->CLIENT")
		errChan <- err
	}()

	// Wait for either direction to complete
	<-errChan
}

// handleClusterConnection handles bidirectional traffic with RESP protocol inspection
// Intercepts and rewrites MOVED/ASK responses to use local proxy addresses
func (p *Proxy) handleClusterConnection(clientConn, remoteConn net.Conn) {
	errChan := make(chan error, 2)

	// Client -> Proxy -> Server: trace and copy
	go func() {
		err := p.copyAndTrace(clientConn, remoteConn, "CLIENT->PROXY", "PROXY->SERVER")
		// Only log non-connection-closure errors (parse errors, etc.)
		if err != nil && !isConnectionClosedError(err) {
			logger.Debug(fmt.Sprintf("Client->Server copy error: %v", err))
		}
		errChan <- err
	}()

	// Server -> Proxy -> Client: parse RESP and rewrite redirects, also trace
	go func() {
		err := p.proxyServerResponses(remoteConn, clientConn)
		// Only log non-connection-closure errors (parse errors, etc.)
		if err != nil && !isConnectionClosedError(err) {
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
			// Treat connection closure errors (EOF, closed connection, etc.) as expected
			if isConnectionClosedError(err) {
				return io.EOF
			}
			// If not a connection closure error, it might be a parse error
			return fmt.Errorf("failed to read RESP value: %w", err)
		}

		// Trace the raw RESP value received from server (before rewriting)
		if p.tracing {
			rawData := value.Serialize()
			logger.Trace("SERVER->PROXY", rawData)
		}

		// Log RESP value type for debugging
		logger.Debug(fmt.Sprintf("Processing RESP value: Type=%c, ArrayLen=%d, Null=%v", byte(value.Type), len(value.Array), value.Null))

		// Check if this is a redirect error (MOVED or ASK) and rewrite if needed
		if value.IsRedirectError() {
			if value.RewriteRedirectError(p.nodeMap) {
				redirectType := "MOVED"
				if strings.HasPrefix(value.Str, "ASK ") {
					redirectType = "ASK"
				}
				logger.Debug(fmt.Sprintf("Rewrote %s redirect: %s", redirectType, value.Str))
			} else {
				logger.Debug(fmt.Sprintf("Redirect not rewritten (node not in map): %s", value.Str))
			}
		}

		// Check if this is a CLUSTER SLOTS response and rewrite addresses if needed
		// CLUSTER SLOTS returns an array of arrays where each sub-array contains [start, end, master_info, ...replicas]
		if value.Type == Array && len(value.Array) > 0 {
			logger.Debug(fmt.Sprintf("Received array response with %d elements, type check: %v", len(value.Array), value.Type == Array))

			// Try to detect if this looks like CLUSTER SLOTS: first element should be an array with at least slot info
			isClusterSlots := false
			if len(value.Array) > 0 && value.Array[0].Type == Array && len(value.Array[0].Array) >= 2 {
				// First element should be [start_slot (integer), end_slot (integer), ...]
				firstElem := value.Array[0]
				logger.Debug(fmt.Sprintf("First element is array with %d sub-elements, checking types...", len(firstElem.Array)))
				if len(firstElem.Array) >= 2 && firstElem.Array[0].Type == Integer && firstElem.Array[1].Type == Integer {
					isClusterSlots = true
					logger.Debug(fmt.Sprintf("Detected CLUSTER SLOTS format (start=%d, end=%d)", firstElem.Array[0].Int, firstElem.Array[1].Int))
				}
			}

			if isClusterSlots {
				logger.Debug(fmt.Sprintf("CLUSTER SLOTS detected (array with %d slot ranges), nodeMap has %d entries:", len(value.Array), len(p.nodeMap)))
				for remote, local := range p.nodeMap {
					logger.Debug(fmt.Sprintf("  nodeMap[%s] = %s", remote, local))
				}
			}

			// Always try to rewrite if it's an array (RewriteClusterSlots will check internally)
			changed := value.RewriteClusterSlots(p.nodeMap)
			if isClusterSlots {
				if changed {
					logger.Debug(fmt.Sprintf("✓ Rewrote CLUSTER SLOTS response addresses (nodeMap has %d entries)", len(p.nodeMap)))
				} else {
					logger.Debug(fmt.Sprintf("✗ CLUSTER SLOTS response not rewritten (nodeMap has %d entries)", len(p.nodeMap)))
				}
			}
		}

		// Serialize and send to client
		data := value.Serialize()

		// Trace the response being sent to client (after any rewriting)
		if p.tracing {
			logger.Trace("PROXY->CLIENT", data)
		}

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

	authCmd := buildAuthCommand(token)
	return sendAuthCommand(conn, authCmd)
}
