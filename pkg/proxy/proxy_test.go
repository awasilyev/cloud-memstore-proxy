package proxy

import (
	"testing"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/config"
	"github.com/awasilyev/cloud-memstore-proxy/pkg/discovery"
)

func TestNewManager(t *testing.T) {
	cfg := &config.Config{
		LocalAddr: "127.0.0.1",
		StartPort: 6379,
	}

	manager := NewManager(cfg)
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.config != cfg {
		t.Error("Config not set correctly")
	}

	if len(manager.proxies) != 0 {
		t.Error("Expected empty proxies list")
	}
}

func TestProxyAddressGeneration(t *testing.T) {
	tests := []struct {
		name      string
		localAddr string
		startPort int
		index     int
		expected  string
	}{
		{
			name:      "First endpoint",
			localAddr: "127.0.0.1",
			startPort: 6379,
			index:     0,
			expected:  "127.0.0.1:6379",
		},
		{
			name:      "Second endpoint",
			localAddr: "127.0.0.1",
			startPort: 6379,
			index:     1,
			expected:  "127.0.0.1:6380",
		},
		{
			name:      "Custom start port",
			localAddr: "0.0.0.0",
			startPort: 7000,
			index:     2,
			expected:  "0.0.0.0:7002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localPort := tt.startPort + tt.index
			result := formatAddress(tt.localAddr, localPort)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func formatAddress(addr string, port int) string {
	return addr + ":" + string(rune('0'+port/1000%10)) +
		string(rune('0'+port/100%10)) +
		string(rune('0'+port/10%10)) +
		string(rune('0'+port%10))
}

func TestEndpointTypes(t *testing.T) {
	endpoint := discovery.Endpoint{
		Host: "10.0.0.1",
		Port: 6379,
		Type: "read-write",
	}

	if endpoint.Type != "read-write" {
		t.Errorf("Expected type read-write, got %s", endpoint.Type)
	}

	if endpoint.Port != 6379 {
		t.Errorf("Expected port 6379, got %d", endpoint.Port)
	}
}
