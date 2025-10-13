package discovery

import (
	"context"
	"net/http"
	"time"
)

// Endpoint represents a Memorystore endpoint
type Endpoint struct {
	Host string
	Port int
	Type string // "primary", "read-replica", "endpoint-N"
}

// InstanceInfo contains instance metadata including TLS configuration
type InstanceInfo struct {
	Endpoints             []Endpoint
	TransitEncryptionMode string
	AuthorizationMode     string
	RequiresTLS           bool
	CACertificate         string
	AuthPassword          string // For Redis instances with password auth
}

// Discoverer interface for discovering Memorystore endpoints
type Discoverer interface {
	DiscoverInstance(ctx context.Context, instanceName string) (*InstanceInfo, error)      // For Valkey
	DiscoverRedisInstance(ctx context.Context, instanceName string) (*InstanceInfo, error) // For Redis
}

// GCPDiscoverer implements Discoverer for GCP Memorystore
type GCPDiscoverer struct {
	httpClient *http.Client
}

// NewGCPDiscoverer creates a new GCP discoverer with configured timeout
func NewGCPDiscoverer(timeoutSeconds int) *GCPDiscoverer {
	return &GCPDiscoverer{
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
			},
		},
	}
}

// NewGCPDiscovererWithDefaults creates a new GCP discoverer with default 30s timeout
func NewGCPDiscovererWithDefaults() *GCPDiscoverer {
	return NewGCPDiscoverer(30)
}
