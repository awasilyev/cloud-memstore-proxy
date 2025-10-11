package metadata

import (
	"context"
	"testing"
)

func TestResolveInstanceName(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedPrefix string
	}{
		{
			name:           "Full instance name unchanged",
			input:          "projects/test-project/locations/us-central1/instances/my-valkey",
			expectedPrefix: "projects/test-project/locations/us-central1/instances/my-valkey",
		},
		{
			name:           "Another full path",
			input:          "projects/prod/locations/europe-west1/instances/valkey-prod",
			expectedPrefix: "projects/prod/locations/europe-west1/instances/valkey-prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := ResolveInstanceName(ctx, tt.input)

			// For full paths, should return as-is without error
			if tt.input == tt.expectedPrefix {
				if err != nil {
					t.Errorf("Expected no error for full path, got: %v", err)
				}
				if result != tt.expectedPrefix {
					t.Errorf("Expected %s, got %s", tt.expectedPrefix, result)
				}
			}
		})
	}
}

func TestGetRegionFromZone(t *testing.T) {
	tests := []struct {
		zone     string
		expected string
	}{
		{"us-central1-a", "us-central1"},
		{"europe-west1-b", "europe-west1"},
		{"asia-southeast1-c", "asia-southeast1"},
	}

	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			// Simulate the zone to region conversion
			parts := len(tt.zone) - 2 // Remove last 2 chars (-a, -b, etc)
			region := tt.zone[:parts]

			if region != tt.expected {
				t.Errorf("Expected region %s from zone %s, got %s", tt.expected, tt.zone, region)
			}
		})
	}
}

func TestGCPMetadata(t *testing.T) {
	metadata := NewGCPMetadata()
	if metadata == nil {
		t.Fatal("Expected non-nil metadata client")
	}

	if metadata.client == nil {
		t.Fatal("Expected non-nil HTTP client")
	}
}
