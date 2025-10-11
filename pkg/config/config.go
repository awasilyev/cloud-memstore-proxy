package config

// InstanceType represents the type of Memorystore instance
type InstanceType string

const (
	InstanceTypeValkey InstanceType = "valkey"
	InstanceTypeRedis  InstanceType = "redis"
)

// Config holds the configuration for the proxy
type Config struct {
	InstanceName  string
	InstanceType  InstanceType
	LocalAddr     string
	StartPort     int
	EnableIAMAuth bool
	Verbose       bool
	TLSSkipVerify bool
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		InstanceType:  InstanceTypeValkey, // Default to Valkey
		LocalAddr:     "127.0.0.1",
		StartPort:     6379,
		EnableIAMAuth: true,
		Verbose:       false,
		TLSSkipVerify: true, // Default to true for GCP Memorystore self-signed certs
	}
}
