# Valkey Auth Proxy - Key Features

## Core Functionality

### 1. Instance Discovery via REST API
- Uses [GCP Memorystore for Valkey REST API](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances) directly (no SDK dependency)
- Automatically discovers all `discoveryEndpoints` from instance configuration
- Retrieves actual `authorizationMode` (`IAM_AUTH`, `DISABLED`)
- Retrieves `transitEncryptionMode` (`SERVER_AUTHENTICATION`, `DISABLED`)

### 2. Automatic TLS/SSL Support
- Detects `transitEncryptionMode` from instance metadata
- Automatically retrieves CA certificate via [`getCertificateAuthority`](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances/getCertificateAuthority) API
- Configures TLS 1.2+ with proper certificate validation
- No manual certificate management required

### 3. Short Instance Name Support ⭐ NEW
- Use just the instance ID: `my-valkey` instead of full path
- Automatically resolves project and region from GCP metadata service
- Perfect for GKE deployments where proxy runs in same project/region
- Falls back to full path format when not on GCP

#### Example Usage:
```bash
# Short format (on GCP)
./valkey-auth-proxy -instance my-valkey

# Full format (works anywhere)
./valkey-auth-proxy -instance "projects/my-project/locations/us-central1/instances/my-valkey"
```

### 4. IAM Authentication
- Automatic authentication using GCP IAM tokens
- Uses Application Default Credentials
- Supports Workload Identity on GKE
- Token refresh handled automatically

### 5. High-Performance Proxying
- **Zero-copy I/O**: Direct data transfer between connections
- **TCP_NODELAY**: Disabled Nagle's algorithm for lower latency
- **TCP Keepalive**: Maintains stable connections
- **TLS Optimization**: Session resumption support
- **Minimal overhead**: Designed for production workloads

### 6. Multi-Endpoint Support
- Automatically creates local listeners for each discovered endpoint
- Port mapping: 6379 (primary), 6380+ (additional endpoints)
- Handles cluster and standalone configurations

## Architecture

```
┌─────────────┐
│   Client    │
│ Application │
└──────┬──────┘
       │ localhost:6379
       │ (plaintext)
       ▼
┌─────────────────────────────────┐
│   Valkey Auth Proxy             │
│                                 │
│  1. Query GCP Metadata          │◄──── GCP Metadata Service
│     (if short name)             │
│                                 │
│  2. Query Memorystore API       │◄──── Memorystore API
│     - Get endpoints             │
│     - Get TLS config            │
│     - Get CA cert               │
│                                 │
│  3. For each connection:        │
│     - Establish TLS (if req.)   │
│     - Authenticate with IAM     │
│     - Proxy traffic             │
└──────┬──────────────────────────┘
       │ TLS + IAM Auth
       ▼
┌─────────────────┐
│ Valkey Instance │
│  (Memorystore)  │
└─────────────────┘
```

## Deployment Patterns

### 1. Kubernetes Sidecar (Recommended)
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-app
      - name: valkey-proxy
        image: valkey-auth-proxy
        env:
        - name: VALKEY_INSTANCE_NAME
          value: "my-valkey"  # Short name!
```

### 2. Standalone Proxy Service
Deploy as a separate service in Kubernetes:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: valkey-proxy
spec:
  ports:
  - port: 6379
  selector:
    app: valkey-proxy
```

### 3. Local Development
```bash
# On GCE instance in same project/region
./valkey-auth-proxy -instance my-valkey -verbose

# From anywhere with credentials
./valkey-auth-proxy \
  -instance "projects/my-project/locations/us-central1/instances/my-valkey"
```

## Security Features

- ✅ TLS/SSL encryption in transit
- ✅ Automatic CA certificate validation
- ✅ IAM-based authentication
- ✅ No password storage required
- ✅ Workload Identity support
- ✅ Minimal attack surface (scratch-based Docker image)

## Performance Characteristics

| Metric | Value |
|--------|-------|
| Latency overhead | < 1ms (typical) |
| Memory footprint | ~10-20MB |
| Docker image size | ~10MB |
| Concurrent connections | Limited by system resources |
| TLS handshake | Hardware-accelerated (AES-NI) |

## Comparison with Cloud SQL Proxy

| Feature | Valkey Auth Proxy | Cloud SQL Proxy |
|---------|-------------------|-----------------|
| Short instance names | ✅ Yes | ✅ Yes |
| Auto TLS setup | ✅ Yes | ✅ Yes |
| IAM authentication | ✅ Yes | ✅ Yes |
| Metadata integration | ✅ Yes | ✅ Yes |
| Multi-endpoint support | ✅ Yes | ⚠️ Limited |
| Language | Go | Go |
| Docker image size | ~10MB | ~20MB |

## Use Cases

### 1. GKE Applications
Perfect for applications running in GKE that need to connect to Memorystore Valkey:
- Automatic project/region resolution
- Workload Identity support
- Sidecar or service deployment

### 2. Multi-Region Applications
- Deploy proxy in each region
- Connects to regional Valkey instances
- Minimal cross-region latency

### 3. Development Environments
- Developers can easily connect to remote instances
- No need to remember full resource paths
- Secure access via IAM

### 4. Migration from Redis
- Drop-in replacement for redis clients
- Applications connect to localhost:6379
- Transparent IAM auth and TLS

## Configuration Examples

### Minimal Configuration (GKE)
```yaml
env:
- name: VALKEY_INSTANCE_NAME
  value: "my-valkey"
```

### Full Configuration
```yaml
env:
- name: VALKEY_INSTANCE_NAME
  value: "projects/my-project/locations/us-central1/instances/my-valkey"
- name: LOCAL_ADDR
  value: "0.0.0.0"
- name: ENABLE_IAM_AUTH
  value: "true"
- name: VERBOSE
  value: "false"
```

## Monitoring and Observability

The proxy logs important events:
- Instance name resolution (short → full path)
- TLS configuration status
- Endpoint discovery results
- Connection establishment
- Authentication success/failure
- Graceful shutdown

Enable verbose logging for debugging:
```bash
./valkey-auth-proxy -instance my-valkey -verbose=true
```

## Future Enhancements

Potential features for future releases:
- [ ] Prometheus metrics endpoint
- [ ] Connection pooling statistics
- [ ] Health check endpoint
- [ ] Dynamic instance discovery (watch for changes)
- [ ] Support for Redis clusters (not just Valkey)
- [ ] Unix socket support
- [ ] mTLS client authentication

