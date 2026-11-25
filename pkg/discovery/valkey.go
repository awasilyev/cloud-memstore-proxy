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
	ServerCaMode          string              `json:"serverCaMode,omitempty"`
}

// perInstanceCAMode is the Memorystore server CA mode where each instance has its own CA
const perInstanceCAMode = "GOOGLE_MANAGED_PER_INSTANCE_CA"

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
	// IMPORTANT: Only return the primary endpoint initially.
	// Additional nodes will be discovered via CLUSTER NODES command if in cluster mode.
	if len(instance.Endpoints) > 0 && len(instance.Endpoints[0].Connections) > 0 {
		// Find the primary endpoint (prefer CONNECTION_TYPE_DISCOVERY, otherwise use first)
		var primaryConn *PscAutoConnection
		for _, conn := range instance.Endpoints[0].Connections {
			psc := conn.PscAutoConnection
			if psc.IPAddress != "" {
				if psc.ConnectionType == "CONNECTION_TYPE_DISCOVERY" {
					primaryConn = &psc
					break
				} else if primaryConn == nil {
					primaryConn = &psc
				}
			}
		}

		if primaryConn != nil {
			info.Endpoints = append(info.Endpoints, Endpoint{
				Host: primaryConn.IPAddress,
				Port: primaryConn.Port,
				Type: "primary",
			})
		}
	} else if len(instance.DiscoveryEndpoints) > 0 {
		// Fallback to discoveryEndpoints if available - only return first (primary)
		if len(instance.DiscoveryEndpoints) > 0 {
			ep := instance.DiscoveryEndpoints[0]
			info.Endpoints = append(info.Endpoints, Endpoint{
				Host: ep.Address,
				Port: ep.Port,
				Type: "primary",
			})
		}
	} else if instance.Host != "" {
		// Fallback to host/port if nothing else available
		info.Endpoints = append(info.Endpoints, Endpoint{
			Host: instance.Host,
			Port: instance.Port,
			Type: "primary",
		})
	}

	// If TLS is required, get the server CA certificates
	if info.RequiresTLS {
		info.PerInstanceCA = instance.ServerCaMode == perInstanceCAMode
		if len(instance.ServerCaCerts) > 0 {
			info.CACertificate = joinCerts(instance.ServerCaCerts)
		} else {
			caCert, err := d.getCACertificate(ctx, instanceName)
			if err != nil {
				// Not fatal here: the proxy refuses to start without a CA unless TLS verification is disabled
				fmt.Fprintf(os.Stderr, "Warning: could not retrieve server CA certificate: %v\n", err)
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

	resp, err := d.httpClient.Do(req)
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

// CertificateAuthority represents the response from the certificateAuthority API
// https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances/getCertificateAuthority
type CertificateAuthority struct {
	ManagedServerCa struct {
		CaCerts []struct {
			Certificates []string `json:"certificates"`
		} `json:"caCerts"`
	} `json:"managedServerCa"`
}

// getCACertificate retrieves every server CA certificate for TLS connections via REST API
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

	url := fmt.Sprintf("https://memorystore.googleapis.com/v1/%s/certificateAuthority", instanceName)

	// Debug output
	if os.Getenv("DEBUG_DISCOVERY") == "true" {
		fmt.Fprintf(os.Stderr, "certificateAuthority URL: %s\n", url)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := d.httpClient.Do(req)
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

	return certAuth.bundle()
}

// bundle returns every CA certificate (intermediates and roots of all chains) as one PEM bundle
func (ca CertificateAuthority) bundle() (string, error) {
	var certs []CertInfo
	for _, chain := range ca.ManagedServerCa.CaCerts {
		for _, cert := range chain.Certificates {
			certs = append(certs, CertInfo{Cert: cert})
		}
	}
	if len(certs) == 0 {
		return "", fmt.Errorf("no CA certificates found")
	}
	return joinCerts(certs), nil
}

// joinCerts joins PEM certificates into one bundle
func joinCerts(certs []CertInfo) string {
	var b strings.Builder
	for _, c := range certs {
		b.WriteString(strings.TrimSpace(c.Cert))
		b.WriteString("\n")
	}
	return b.String()
}
