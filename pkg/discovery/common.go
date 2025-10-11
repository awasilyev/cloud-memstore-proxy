package discovery

import (
	"context"
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
type GCPDiscoverer struct{}

// NewGCPDiscoverer creates a new GCP discoverer
func NewGCPDiscoverer() *GCPDiscoverer {
	return &GCPDiscoverer{}
}
