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
	"github.com/awasilyev/cloud-memstore-proxy/pkg/health"
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
	flag.IntVar(&cfg.HealthPort, "health-port", getEnvOrDefaultInt("HEALTH_PORT", 8080), "Health check HTTP server port")
	flag.BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", getEnvOrDefaultBool("TLS_SKIP_VERIFY", true), "Skip TLS certificate verification (needed for GCP Memorystore self-signed certs)")
	flag.BoolVar(&cfg.Verbose, "verbose", getEnvOrDefaultBool("VERBOSE", false), "Enable verbose logging")
	flag.Parse()

	// Set instance type
	cfg.InstanceType = config.InstanceType(strings.ToLower(instanceType))

	// Validate configuration
	if cfg.InstanceName == "" {
		logger.Fatal("Instance name is required. Set via -instance flag or VALKEY_INSTANCE_NAME env variable")
	}

	logger.Init(cfg.Verbose)

	// Always log startup information for debugging
	fmt.Printf("=== Cloud Memstore Proxy Startup ===\n")
	fmt.Printf("Type: %s\n", cfg.InstanceType)
	fmt.Printf("Instance: %s\n", cfg.InstanceName)
	fmt.Printf("Local Addr: %s\n", cfg.LocalAddr)
	fmt.Printf("Start Port: %d\n", cfg.StartPort)
	fmt.Printf("Health Port: %d\n", cfg.HealthPort)
	fmt.Printf("TLS Skip Verify: %v\n", cfg.TLSSkipVerify)
	fmt.Printf("Verbose: %v\n", cfg.Verbose)
	fmt.Printf("===================================\n\n")

	logger.Info(fmt.Sprintf("Starting Cloud Memstore Proxy for %s...", cfg.InstanceType))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health check server
	fmt.Printf("Starting health check server on port %d...\n", cfg.HealthPort)
	healthServer := health.NewServer(cfg.HealthPort)
	if err := healthServer.Start(); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to start health server: %v", err))
	}
	defer healthServer.Stop()
	fmt.Printf("Health check server started\n")

	// Resolve instance name (convert short name to full path if needed)
	fmt.Printf("Resolving instance name: %s\n", cfg.InstanceName)
	resolvedInstanceName, err := resolveInstanceName(ctx, cfg.InstanceName)
	if err != nil {
		fmt.Printf("ERROR: Failed to resolve instance name: %v\n", err)
		logger.Fatal(fmt.Sprintf("Failed to resolve instance name: %v", err))
	}

	if resolvedInstanceName != cfg.InstanceName {
		fmt.Printf("Instance name resolved: %s -> %s\n", cfg.InstanceName, resolvedInstanceName)
		logger.Info(fmt.Sprintf("Resolved instance: %s -> %s", cfg.InstanceName, resolvedInstanceName))
	}

	logger.Info(fmt.Sprintf("Instance: %s", resolvedInstanceName))
	logger.Info(fmt.Sprintf("Local address: %s", cfg.LocalAddr))

	fmt.Printf("Configuration validated successfully\n")

	// Discover instance endpoints and configuration based on type
	fmt.Printf("Starting discovery for %s instance...\n", cfg.InstanceType)
	logger.Info(fmt.Sprintf("Discovering %s instance configuration...", cfg.InstanceType))
	discoverer := discovery.NewGCPDiscoverer()

	var instanceInfo *discovery.InstanceInfo

	switch cfg.InstanceType {
	case config.InstanceTypeRedis:
		fmt.Printf("Using Redis discovery API\n")
		instanceInfo, err = discoverer.DiscoverRedisInstance(ctx, resolvedInstanceName)
	case config.InstanceTypeValkey:
		fmt.Printf("Using Valkey discovery API\n")
		instanceInfo, err = discoverer.DiscoverInstance(ctx, resolvedInstanceName)
	default:
		logger.Fatal(fmt.Sprintf("Unknown instance type: %s (must be 'valkey' or 'redis')", cfg.InstanceType))
	}

	if err != nil {
		fmt.Printf("ERROR: Discovery failed: %v\n", err)
		logger.Fatal(fmt.Sprintf("Failed to discover instance: %v", err))
	}

	fmt.Printf("Discovery completed successfully\n")

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
	fmt.Printf("Creating proxy manager...\n")
	proxyManager := proxy.NewManager(cfg)

	// Configure TLS if required
	if instanceInfo.RequiresTLS {
		fmt.Printf("TLS is required, configuring...\n")
		logger.Info("Configuring TLS...")
		if err := proxyManager.SetTLSConfig(instanceInfo.CACertificate, cfg.TLSSkipVerify); err != nil {
			fmt.Printf("ERROR: TLS configuration failed: %v\n", err)
			logger.Fatal(fmt.Sprintf("Failed to configure TLS: %v", err))
		}
		logger.Info("TLS configuration complete")
		fmt.Printf("TLS configured successfully\n")
	} else {
		fmt.Printf("TLS not required for this instance\n")
	}

	// Configure password auth for Redis instances
	if instanceInfo.AuthPassword != "" {
		fmt.Printf("Configuring password authentication...\n")
		proxyManager.SetAuthPassword(instanceInfo.AuthPassword)
		fmt.Printf("Password auth configured\n")
	}

	fmt.Printf("Starting %d proxy server(s)...\n", len(instanceInfo.Endpoints))
	for i, endpoint := range instanceInfo.Endpoints {
		localPort := cfg.StartPort + i
		fmt.Printf("Starting proxy %d/%d: %s:%d -> %s:%d\n", i+1, len(instanceInfo.Endpoints), cfg.LocalAddr, localPort, endpoint.Host, endpoint.Port)
		if err := proxyManager.AddProxy(ctx, endpoint, localPort); err != nil {
			fmt.Printf("ERROR: Failed to start proxy: %v\n", err)
			logger.Fatal(fmt.Sprintf("Failed to start proxy for %s:%d: %v", endpoint.Host, endpoint.Port, err))
		}
		tlsStatus := "plaintext"
		if instanceInfo.RequiresTLS {
			tlsStatus = "TLS"
		}
		logger.Info(fmt.Sprintf("Proxy listening on %s:%d -> %s:%d (%s, %s)", cfg.LocalAddr, localPort, endpoint.Host, endpoint.Port, endpoint.Type, tlsStatus))
		fmt.Printf("✅ Proxy %d started successfully\n", i+1)
	}

	// Mark health server as ready
	fmt.Printf("Marking health server as ready with %d proxies\n", len(instanceInfo.Endpoints))
	healthServer.SetReady(len(instanceInfo.Endpoints))
	logger.Info(fmt.Sprintf("All proxies ready. Health endpoints: http://localhost:%d/livez, /readyz, /status", cfg.HealthPort))

	fmt.Printf("\n=== READY ===\n")
	fmt.Printf("Proxies: %d\n", len(instanceInfo.Endpoints))
	fmt.Printf("Health: http://localhost:%d/livez\n", cfg.HealthPort)
	fmt.Printf("=============\n\n")

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
