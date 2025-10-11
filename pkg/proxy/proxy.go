package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/auth"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/config"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/discovery"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/logger"
)

// Manager manages multiple proxy instances
type Manager struct {
	config       *config.Config
	proxies      []*Proxy
	tokenSource  *auth.IAMTokenProvider
	authPassword string // For Redis password auth
	tlsConfig    *tls.Config
	mu           sync.Mutex
}

// Proxy represents a single proxy instance
type Proxy struct {
	localAddr    string
	remoteAddr   string
	endpoint     discovery.Endpoint
	listener     net.Listener
	config       *config.Config
	tokenSource  *auth.IAMTokenProvider
	authPassword string // For Redis password auth
	tlsConfig    *tls.Config
	connections  sync.WaitGroup
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// NewManager creates a new proxy manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config:  cfg,
		proxies: make([]*Proxy, 0),
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

// AddProxy adds and starts a new proxy
func (m *Manager) AddProxy(ctx context.Context, endpoint discovery.Endpoint, localPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize token source if IAM auth is enabled AND no password is set (shared across all proxies)
	// Password auth takes precedence over IAM auth
	if m.config.EnableIAMAuth && m.authPassword == "" && m.tokenSource == nil {
		tokenSource, err := auth.NewIAMTokenProvider(ctx)
		if err != nil {
			return fmt.Errorf("failed to create IAM token provider: %w", err)
		}
		m.tokenSource = tokenSource
	}

	localAddr := fmt.Sprintf("%s:%d", m.config.LocalAddr, localPort)
	remoteAddr := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	proxy := &Proxy{
		localAddr:    localAddr,
		remoteAddr:   remoteAddr,
		endpoint:     endpoint,
		config:       m.config,
		tokenSource:  m.tokenSource,
		authPassword: m.authPassword,
		tlsConfig:    m.tlsConfig,
		shutdown:     make(chan struct{}),
	}

	if err := proxy.Start(); err != nil {
		return err
	}

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
	} else if p.config.EnableIAMAuth && p.tokenSource != nil {
		// IAM authentication (for Valkey with IAM_AUTH)
		if err := p.authenticateIAM(remoteConn); err != nil {
			logger.Error(fmt.Sprintf("IAM authentication failed: %v", err))
			return
		}
		logger.Debug("IAM authentication successful")
	}

	// Start bidirectional copy
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(remoteConn, clientConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, remoteConn)
		errChan <- err
	}()

	// Wait for either direction to complete
	<-errChan

	logger.Debug(fmt.Sprintf("Connection closed: %s", clientConn.RemoteAddr()))
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
