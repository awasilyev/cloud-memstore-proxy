package config

import "testing"

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	// Test default values
	if cfg.LocalAddr != "127.0.0.1" {
		t.Errorf("Expected LocalAddr to be 127.0.0.1, got %s", cfg.LocalAddr)
	}

	if cfg.StartPort != 6379 {
		t.Errorf("Expected StartPort to be 6379, got %d", cfg.StartPort)
	}

	if cfg.Verbose != false {
		t.Error("Expected Verbose to be false")
	}
}

func TestConfigModification(t *testing.T) {
	cfg := NewConfig()

	cfg.LocalAddr = "0.0.0.0"
	cfg.StartPort = 7000
	cfg.Verbose = true

	if cfg.LocalAddr != "0.0.0.0" {
		t.Error("LocalAddr not modified correctly")
	}

	if cfg.StartPort != 7000 {
		t.Error("StartPort not modified correctly")
	}

	if cfg.Verbose != true {
		t.Error("Verbose not modified correctly")
	}
}
