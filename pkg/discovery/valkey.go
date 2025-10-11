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

// ValKeyInstance represents the Memorystore for Valkey instance from REST API
type ValKeyInstance struct {
	Name                  string              `json:"name"`
	Host                  string              `json:"host,omitempty"`
	Port                  int                 `json:"port,omitempty"`
	ReadEndpoint          string              `json:"readEndpoint,omitempty"`
	ReadEndpointPort      int                 `json:"readEndpointPort,omitempty"`
	AuthorizationMode     string              `json:"authorizationMode"`
	TransitEncryptionMode string              `json:"transitEncryptionMode"`
	DiscoveryEndpoints    []DiscoveryEndpoint `json:"discoveryEndpoints,omitempty"`
	Endpoints             []InstanceEndpoint  `json:"endpoints,omitempty"`
	ServerCaCerts         []CertInfo          `json:"serverCaCerts,omitempty"`
}

// InstanceEndpoint represents an endpoint with connections
type InstanceEndpoint struct {
	Connections []ConnectionDetail `json:"connections"`
}

// ConnectionDetail represents a connection detail
type ConnectionDetail struct {
	PscAutoConnection PscAutoConnection `json:"pscAutoConnection"`
}

// PscAutoConnection represents PSC auto connection details
type PscAutoConnection struct {
	PscConnectionID   string `json:"pscConnectionId"`
	IPAddress         string `json:"ipAddress"`
	Port              int    `json:"port"`
	ConnectionType    string `json:"connectionType"`
	ServiceAttachment string `json:"serviceAttachment"`
}

// DiscoveryEndpoint represents a discovery endpoint from the API
type DiscoveryEndpoint struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// CertInfo represents certificate information
type CertInfo struct {
	Cert string `json:"cert"`
}

// DiscoverInstance discovers endpoints and configuration for a GCP Memorystore Valkey instance
func (d *GCPDiscoverer) DiscoverInstance(ctx context.Context, instanceName string) (*InstanceInfo, error) {
	// Parse instance name to extract project, location, and instance ID
	// Expected format: projects/PROJECT_ID/locations/LOCATION/instances/INSTANCE_ID
	parts := strings.Split(instanceName, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "instances" {
		return nil, fmt.Errorf("invalid instance name format: %s (expected: projects/PROJECT_ID/locations/LOCATION/instances/INSTANCE_ID)", instanceName)
	}

	// Get instance details via REST API
	instance, err := d.getInstance(ctx, instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	info := &InstanceInfo{
		Endpoints:             make([]Endpoint, 0),
		TransitEncryptionMode: instance.TransitEncryptionMode,
		AuthorizationMode:     instance.AuthorizationMode,
	}

	// Determine if TLS is required based on transit encryption mode
	info.RequiresTLS = instance.TransitEncryptionMode == "SERVER_AUTHENTICATION"

	// Parse endpoints from the new structure
	if len(instance.Endpoints) > 0 && len(instance.Endpoints[0].Connections) > 0 {
		for i, conn := range instance.Endpoints[0].Connections {
			psc := conn.PscAutoConnection
			if psc.IPAddress != "" {
				epType := "primary"
				// CONNECTION_TYPE_DISCOVERY is for read-write
				if psc.ConnectionType == "CONNECTION_TYPE_DISCOVERY" {
					epType = "primary"
				} else if i > 0 {
					epType = fmt.Sprintf("endpoint-%d", i)
				}

				info.Endpoints = append(info.Endpoints, Endpoint{
					Host: psc.IPAddress,
					Port: psc.Port,
					Type: epType,
				})
			}
		}
	} else if len(instance.DiscoveryEndpoints) > 0 {
		// Fallback to discoveryEndpoints if available
		for i, ep := range instance.DiscoveryEndpoints {
			epType := "primary"
			if i > 0 {
				epType = fmt.Sprintf("endpoint-%d", i)
			}
			info.Endpoints = append(info.Endpoints, Endpoint{
				Host: ep.Address,
				Port: ep.Port,
				Type: epType,
			})
		}
	} else if instance.Host != "" {
		// Fallback to host/port if nothing else available
		info.Endpoints = append(info.Endpoints, Endpoint{
			Host: instance.Host,
			Port: instance.Port,
			Type: "primary",
		})

		// Add read endpoint if available (for read replicas)
		if instance.ReadEndpoint != "" && instance.ReadEndpointPort > 0 {
			info.Endpoints = append(info.Endpoints, Endpoint{
				Host: instance.ReadEndpoint,
				Port: instance.ReadEndpointPort,
				Type: "read-replica",
			})
		}
	}

	// If TLS is required, get CA certificate
	if info.RequiresTLS {
		if len(instance.ServerCaCerts) > 0 {
			info.CACertificate = instance.ServerCaCerts[0].Cert
		} else {
			// Try to fetch from getCertificateAuthority endpoint (may not be available for Valkey)
			caCert, err := d.getCACertificate(ctx, instanceName)
			if err != nil {
				// getCertificateAuthority may not be available for Valkey instances
				// In this case, TLS will use system CA certificates
				if os.Getenv("DEBUG_DISCOVERY") == "true" {
					fmt.Fprintf(os.Stderr, "Warning: Could not retrieve CA certificate: %v\n", err)
					fmt.Fprintf(os.Stderr, "TLS will use system CA certificates\n")
				}
			} else {
				info.CACertificate = caCert
			}
		}
	}

	return info, nil
}

// getInstance fetches instance details from Memorystore REST API
func (d *GCPDiscoverer) getInstance(ctx context.Context, instanceName string) (*ValKeyInstance, error) {
	// Get OAuth2 token
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Make REST API call
	url := fmt.Sprintf("https://memorystore.googleapis.com/v1/%s", instanceName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: print raw response if verbose env is set
	if os.Getenv("DEBUG_DISCOVERY") == "true" {
		fmt.Fprintf(os.Stderr, "Raw API Response:\n%s\n\n", string(bodyBytes))
	}

	var instance ValKeyInstance
	if err := json.Unmarshal(bodyBytes, &instance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &instance, nil
}

// CertificateAuthority represents the response from getCertificateAuthority API
type CertificateAuthority struct {
	ManagedServerCa struct {
		CaCerts []CertInfo `json:"caCerts"`
	} `json:"managedServerCa"`
}

// getCACertificate retrieves the CA certificate for TLS connections via REST API
func (d *GCPDiscoverer) getCACertificate(ctx context.Context, instanceName string) (string, error) {
	// Get OAuth2 token
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	// Make REST API call to getCertificateAuthority
	// According to GCP docs, this is a POST method with empty body
	url := fmt.Sprintf("https://memorystore.googleapis.com/v1/%s:getCertificateAuthority", instanceName)

	// Debug output
	if os.Getenv("DEBUG_DISCOVERY") == "true" {
		fmt.Fprintf(os.Stderr, "getCertificateAuthority URL: %s\n", url)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var certAuth CertificateAuthority
	if err := json.NewDecoder(resp.Body).Decode(&certAuth); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(certAuth.ManagedServerCa.CaCerts) == 0 {
		return "", fmt.Errorf("no CA certificates found")
	}

	return certAuth.ManagedServerCa.CaCerts[0].Cert, nil
}
