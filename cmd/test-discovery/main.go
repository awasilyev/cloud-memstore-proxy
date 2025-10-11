package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/discovery"
)

func main() {
	instanceName := flag.String("instance", "", "Instance name to discover")
	instanceType := flag.String("type", "valkey", "Instance type: 'valkey' or 'redis'")
	verbose := flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	if *instanceName == "" {
		fmt.Println("Usage: test-discovery -type <type> -instance <instance-name>")
		fmt.Println("\nExample:")
		fmt.Println("  test-discovery -type valkey -instance projects/my-project/locations/us-central1/instances/my-valkey")
		fmt.Println("  test-discovery -type redis -instance projects/my-project/locations/us-central1/instances/my-redis")
		os.Exit(1)
	}

	ctx := context.Background()
	discoverer := discovery.NewGCPDiscoverer()

	fmt.Printf("Discovering %s instance: %s\n\n", *instanceType, *instanceName)

	var info *discovery.InstanceInfo
	var err error

	switch strings.ToLower(*instanceType) {
	case "redis":
		info, err = discoverer.DiscoverRedisInstance(ctx, *instanceName)
	case "valkey":
		info, err = discoverer.DiscoverInstance(ctx, *instanceName)
	default:
		fmt.Printf("❌ Unknown instance type: %s (must be 'valkey' or 'redis')\n", *instanceType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println("✅ Discovery successful!")
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("INSTANCE INFORMATION")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\n📋 Configuration:\n")
	fmt.Printf("   Transit Encryption Mode: %s\n", info.TransitEncryptionMode)
	fmt.Printf("   Authorization Mode:      %s\n", info.AuthorizationMode)
	fmt.Printf("   TLS Required:            %v\n", info.RequiresTLS)

	fmt.Printf("\n🌐 Endpoints (%d):\n", len(info.Endpoints))
	for i, ep := range info.Endpoints {
		fmt.Printf("   %d. %s:%d (%s)\n", i+1, ep.Host, ep.Port, ep.Type)
	}

	if info.RequiresTLS && info.CACertificate != "" {
		fmt.Printf("\n🔒 CA Certificate:\n")
		certLines := strings.Split(info.CACertificate, "\n")
		for i, line := range certLines {
			if i < 3 || i >= len(certLines)-3 {
				fmt.Printf("   %s\n", line)
			} else if i == 3 {
				fmt.Printf("   ... (%d lines)\n", len(certLines)-6)
			}
		}
	}

	if *verbose {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("JSON OUTPUT")
		fmt.Println(strings.Repeat("=", 60))
		jsonData, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(jsonData))
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
}
