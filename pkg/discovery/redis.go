package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

// RedisInstance represents a Memorystore for Redis instance from REST API
type RedisInstance struct {
	Name                  string `json:"name"`
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	ReadReplicasMode      string `json:"readReplicasMode,omitempty"`
	ReadEndpoint          string `json:"readEndpoint,omitempty"`
	ReadEndpointPort      int    `json:"readEndpointPort,omitempty"`
	AuthEnabled           bool   `json:"authEnabled"`
	TransitEncryptionMode string `json:"transitEncryptionMode"`
	ServerCaCerts         []struct {
		Cert string `json:"cert"`
	} `json:"serverCaCerts,omitempty"`
	CurrentLocationID string `json:"currentLocationId,omitempty"`
}

// DiscoverRedisInstance discovers a Memorystore for Redis instance
func (d *GCPDiscoverer) DiscoverRedisInstance(ctx context.Context, instanceName string) (*InstanceInfo, error) {
	// Parse instance name
	parts := strings.Split(instanceName, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "instances" {
		return nil, fmt.Errorf("invalid instance name format: %s", instanceName)
	}

	// Get instance via REST API
	instance, err := d.getRedisInstance(ctx, instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis instance: %w", err)
	}

	info := &InstanceInfo{
		Endpoints:             make([]Endpoint, 0),
		TransitEncryptionMode: instance.TransitEncryptionMode,
		AuthorizationMode:     "PASSWORD_AUTH",
	}

	// Check if auth is enabled
	if instance.AuthEnabled {
		info.AuthorizationMode = "PASSWORD_AUTH"
	} else {
		info.AuthorizationMode = "AUTH_DISABLED"
	}

	// Determine if TLS is required
	info.RequiresTLS = instance.TransitEncryptionMode == "SERVER_AUTHENTICATION"

	// Add primary endpoint
	if instance.Host != "" {
		info.Endpoints = append(info.Endpoints, Endpoint{
			Host: instance.Host,
			Port: instance.Port,
			Type: "primary",
		})
	}

	// Add read endpoint if available
	if instance.ReadEndpoint != "" && instance.ReadEndpointPort > 0 {
		info.Endpoints = append(info.Endpoints, Endpoint{
			Host: instance.ReadEndpoint,
			Port: instance.ReadEndpointPort,
			Type: "read-replica",
		})
	}

	// Get CA certificate if TLS is enabled
	if info.RequiresTLS {
		if len(instance.ServerCaCerts) > 0 {
			info.CACertificate = instance.ServerCaCerts[0].Cert
		}
	}

	// Get auth password if auth is enabled
	if instance.AuthEnabled {
		password, err := d.getRedisAuthString(ctx, instanceName)
		if err != nil {
			// Auth string retrieval failed, but we can continue
			// The proxy will fail to authenticate, but discovery succeeds
			if os.Getenv("DEBUG_DISCOVERY") == "true" {
				fmt.Fprintf(os.Stderr, "Warning: Could not retrieve auth string: %v\n", err)
			}
		} else {
			info.AuthPassword = password
		}
	}

	return info, nil
}

// getRedisInstance fetches Redis instance details from REST API
func (d *GCPDiscoverer) getRedisInstance(ctx context.Context, instanceName string) (*RedisInstance, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Use Redis API endpoint
	url := fmt.Sprintf("https://redis.googleapis.com/v1/%s", instanceName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if os.Getenv("DEBUG_DISCOVERY") == "true" {
		fmt.Fprintf(os.Stderr, "Redis Instance API Response:\n%s\n\n", string(bodyBytes))
	}

	var instance RedisInstance
	if err := json.Unmarshal(bodyBytes, &instance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &instance, nil
}

// getRedisAuthString retrieves the auth string (password) for a Redis instance
func (d *GCPDiscoverer) getRedisAuthString(ctx context.Context, instanceName string) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	// Call getAuthString method
	url := fmt.Sprintf("https://redis.googleapis.com/v1/%s/authString", instanceName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp struct {
		AuthString string `json:"authString"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return authResp.AuthString, nil
}
