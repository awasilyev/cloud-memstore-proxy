package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/logger"
)

// Server represents the health check HTTP server
type Server struct {
	port       int
	server     *http.Server
	ready      bool
	proxyCount int
	startTime  time.Time
	mu         sync.RWMutex
}

// Status represents the health check response
type Status struct {
	Status       string `json:"status"`
	Ready        bool   `json:"ready"`
	Uptime       string `json:"uptime"`
	ProxyCount   int    `json:"proxy_count"`
	Version      string `json:"version,omitempty"`
	InstanceType string `json:"instance_type,omitempty"`
}

// NewServer creates a new health check server
func NewServer(port int) *Server {
	return &Server{
		port:      port,
		ready:     false,
		startTime: time.Now(),
	}
}

// Start starts the health check HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Liveness endpoint - always returns 200 if server is running
	mux.HandleFunc("/livez", s.handleLiveness)
	mux.HandleFunc("/healthz", s.handleLiveness) // Alias for compatibility

	// Ready endpoint - returns 200 only when proxies are configured
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/ready", s.handleReady) // Alias for compatibility

	// Status endpoint - detailed status information
	mux.HandleFunc("/status", s.handleStatus)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		logger.Info(fmt.Sprintf("Health check server listening on :%d", s.port))
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("Health server error: %v", err))
		}
	}()

	return nil
}

// Stop stops the health check server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// SetReady marks the server as ready (proxies configured)
func (s *Server) SetReady(proxyCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.proxyCount = proxyCount
}

// handleLiveness handles /livez and /healthz endpoints (liveness probe)
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}

// handleReady handles /ready and /readyz endpoints
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if ready {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ready",
		})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
		})
	}
}

// handleStatus handles /status endpoint
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ready := s.ready
	proxyCount := s.proxyCount
	s.mu.RUnlock()

	uptime := time.Since(s.startTime).Round(time.Second)

	status := Status{
		Status:     "healthy",
		Ready:      ready,
		Uptime:     uptime.String(),
		ProxyCount: proxyCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
