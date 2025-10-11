package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	metadataServerURL = "http://metadata.google.internal/computeMetadata/v1"
	metadataTimeout   = 2 * time.Second
)

// GCPMetadata provides access to GCP metadata
type GCPMetadata struct {
	client *http.Client
}

// NewGCPMetadata creates a new GCP metadata client
func NewGCPMetadata() *GCPMetadata {
	return &GCPMetadata{
		client: &http.Client{
			Timeout: metadataTimeout,
		},
	}
}

// GetProjectID retrieves the current GCP project ID
func (m *GCPMetadata) GetProjectID(ctx context.Context) (string, error) {
	return m.get(ctx, "/project/project-id")
}

// GetZone retrieves the current GCP zone (e.g., "us-central1-a")
func (m *GCPMetadata) GetZone(ctx context.Context) (string, error) {
	zone, err := m.get(ctx, "/instance/zone")
	if err != nil {
		return "", err
	}
	// Zone comes back as "projects/PROJECT_NUMBER/zones/ZONE"
	// Extract just the zone name
	parts := strings.Split(zone, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1], nil
	}
	return zone, nil
}

// GetRegion retrieves the current GCP region from the zone
func (m *GCPMetadata) GetRegion(ctx context.Context) (string, error) {
	zone, err := m.GetZone(ctx)
	if err != nil {
		return "", err
	}
	// Convert zone to region by removing the last part (e.g., "us-central1-a" -> "us-central1")
	parts := strings.Split(zone, "-")
	if len(parts) >= 2 {
		return strings.Join(parts[:len(parts)-1], "-"), nil
	}
	return "", fmt.Errorf("unable to parse region from zone: %s", zone)
}

// get performs a metadata server request
func (m *GCPMetadata) get(ctx context.Context, path string) (string, error) {
	url := metadataServerURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Metadata server requires this header
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query metadata server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// IsRunningOnGCP checks if we're running on GCP by attempting to contact the metadata server
func (m *GCPMetadata) IsRunningOnGCP(ctx context.Context) bool {
	_, err := m.GetProjectID(ctx)
	return err == nil
}

// ResolveInstanceName converts a short instance name to full resource path
// If the instance name is already in full format, returns it as-is
// If it's a short name, resolves project and region from metadata
func ResolveInstanceName(ctx context.Context, instanceName string) (string, error) {
	// Check if already in full format
	if strings.HasPrefix(instanceName, "projects/") {
		return instanceName, nil
	}

	// Short name provided, need to resolve project and region
	metadata := NewGCPMetadata()

	projectID, err := metadata.GetProjectID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get project ID from metadata (are you running on GCP?): %w", err)
	}

	region, err := metadata.GetRegion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get region from metadata: %w", err)
	}

	// Construct full instance name
	fullName := fmt.Sprintf("projects/%s/locations/%s/instances/%s", projectID, region, instanceName)
	return fullName, nil
}
