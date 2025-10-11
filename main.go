package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/config"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/discovery"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/logger"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/metadata"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/proxy"
)

func main() {
	// Parse configuration from flags and environment variables
	cfg := config.NewConfig()

	var instanceType string
	flag.StringVar(&cfg.InstanceName, "instance", os.Getenv("INSTANCE_NAME"), "Instance name (format: projects/PROJECT_ID/locations/LOCATION/instances/INSTANCE_ID)")
	flag.StringVar(&instanceType, "type", getEnvOrDefault("INSTANCE_TYPE", "valkey"), "Instance type: 'valkey' or 'redis'")
	flag.StringVar(&cfg.LocalAddr, "local-addr", getEnvOrDefault("LOCAL_ADDR", "127.0.0.1"), "Local address to bind to")
	flag.IntVar(&cfg.StartPort, "start-port", getEnvOrDefaultInt("START_PORT", 6379), "Starting port number for the first endpoint")
	flag.BoolVar(&cfg.EnableIAMAuth, "enable-iam-auth", getEnvOrDefaultBool("ENABLE_IAM_AUTH", true), "Enable IAM authentication (for Valkey with IAM_AUTH mode)")
	flag.BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", getEnvOrDefaultBool("TLS_SKIP_VERIFY", true), "Skip TLS certificate verification (needed for GCP Memorystore self-signed certs)")
	flag.BoolVar(&cfg.Verbose, "verbose", getEnvOrDefaultBool("VERBOSE", false), "Enable verbose logging")
	flag.Parse()

	// Set instance type
	cfg.InstanceType = config.InstanceType(strings.ToLower(instanceType))

	// Validate configuration
	if cfg.InstanceName == "" {
		logger.Fatal("Instance name is required. Set via -instance flag or INSTANCE_NAME env variable")
	}

	logger.Init(cfg.Verbose)
	logger.Info(fmt.Sprintf("Starting Cloud Memstore Proxy for %s...", cfg.InstanceType))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Resolve instance name (convert short name to full path if needed)
	resolvedInstanceName, err := resolveInstanceName(ctx, cfg.InstanceName)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to resolve instance name: %v", err))
	}

	if resolvedInstanceName != cfg.InstanceName {
		logger.Info(fmt.Sprintf("Resolved instance: %s -> %s", cfg.InstanceName, resolvedInstanceName))
	}

	logger.Info(fmt.Sprintf("Instance: %s", resolvedInstanceName))
	logger.Info(fmt.Sprintf("Local address: %s", cfg.LocalAddr))
	logger.Info(fmt.Sprintf("IAM Auth: %v", cfg.EnableIAMAuth))

	// Discover instance endpoints and configuration based on type
	logger.Info(fmt.Sprintf("Discovering %s instance configuration...", cfg.InstanceType))
	discoverer := discovery.NewGCPDiscoverer()

	var instanceInfo *discovery.InstanceInfo

	switch cfg.InstanceType {
	case config.InstanceTypeRedis:
		instanceInfo, err = discoverer.DiscoverRedisInstance(ctx, resolvedInstanceName)
	case config.InstanceTypeValkey:
		instanceInfo, err = discoverer.DiscoverInstance(ctx, resolvedInstanceName)
	default:
		logger.Fatal(fmt.Sprintf("Unknown instance type: %s (must be 'valkey' or 'redis')", cfg.InstanceType))
	}

	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to discover instance: %v", err))
	}

	if len(instanceInfo.Endpoints) == 0 {
		logger.Fatal("No endpoints found for the instance")
	}

	logger.Info("Instance configuration:")
	logger.Info(fmt.Sprintf("  Transit Encryption: %s", instanceInfo.TransitEncryptionMode))
	logger.Info(fmt.Sprintf("  Authorization Mode: %s", instanceInfo.AuthorizationMode))
	logger.Info(fmt.Sprintf("  TLS Required: %v", instanceInfo.RequiresTLS))
	logger.Info(fmt.Sprintf("  Endpoints: %d", len(instanceInfo.Endpoints)))

	for i, ep := range instanceInfo.Endpoints {
		logger.Info(fmt.Sprintf("    %d. %s:%d (%s)", i+1, ep.Host, ep.Port, ep.Type))
	}

	// Start proxy servers for each endpoint
	proxyManager := proxy.NewManager(cfg)

	// Configure TLS if required
	if instanceInfo.RequiresTLS {
		logger.Info("Configuring TLS...")
		if err := proxyManager.SetTLSConfig(instanceInfo.CACertificate, cfg.TLSSkipVerify); err != nil {
			logger.Fatal(fmt.Sprintf("Failed to configure TLS: %v", err))
		}
		logger.Info("TLS configuration complete")
	}

	// Configure password auth for Redis instances
	if instanceInfo.AuthPassword != "" {
		proxyManager.SetAuthPassword(instanceInfo.AuthPassword)
	}

	for i, endpoint := range instanceInfo.Endpoints {
		localPort := cfg.StartPort + i
		if err := proxyManager.AddProxy(ctx, endpoint, localPort); err != nil {
			logger.Fatal(fmt.Sprintf("Failed to start proxy for %s:%d: %v", endpoint.Host, endpoint.Port, err))
		}
		tlsStatus := "plaintext"
		if instanceInfo.RequiresTLS {
			tlsStatus = "TLS"
		}
		logger.Info(fmt.Sprintf("Proxy listening on %s:%d -> %s:%d (%s, %s)", cfg.LocalAddr, localPort, endpoint.Host, endpoint.Port, endpoint.Type, tlsStatus))
	}

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	proxyManager.Shutdown()
	logger.Info("Shutdown complete")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var intValue int
	if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
		return intValue
	}
	return defaultValue
}

// resolveInstanceName converts a short instance name to full resource path if needed
func resolveInstanceName(ctx context.Context, instanceName string) (string, error) {
	// If already in full format, return as-is
	if strings.HasPrefix(instanceName, "projects/") {
		return instanceName, nil
	}

	// Short name provided - resolve using metadata
	logger.Debug("Short instance name detected, resolving from GCP metadata...")

	resolved, err := metadata.ResolveInstanceName(ctx, instanceName)
	if err != nil {
		// If we can't get metadata, provide helpful error message
		return "", fmt.Errorf("%w\n\nYou can also specify the full instance name in the format:\nprojects/PROJECT_ID/locations/LOCATION/instances/INSTANCE_ID", err)
	}

	return resolved, nil
}
