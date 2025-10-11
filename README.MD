# Cloud Memstore Proxy

A lightweight, high-performance proxy service for **Google Cloud Memorystore** instances (Redis and Valkey) with automatic authentication and TLS support. Similar to Cloud SQL Auth Proxy, this tool provides secure, authenticated access to your Memorystore instances.

[![Release](https://img.shields.io/github/v/release/awasilyev/cloud-memstore-proxy)](https://github.com/awasilyev/cloud-memstore-proxy/releases)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://github.com/awasilyev/cloud-memstore-proxy/pkgs/container/cloud-memstore-proxy)
[![License](https://img.shields.io/github/license/awasilyev/cloud-memstore-proxy)](LICENSE)

## Features

- 🎯 **Multi-Instance Support**: Works with both **Valkey** and **Redis** instances
- 🔐 **Automatic Authentication**: 
  - IAM authentication for Valkey (IAM_AUTH mode)
  - Password authentication for Redis (auto-retrieved from API)
- 🔒 **TLS Support**: Automatic TLS/SSL encryption
- 🔍 **Auto-discovery**: Automatically discovers all endpoints using GCP APIs
- 🚀 **High Performance**: Minimal latency overhead with TCP optimizations
- 🐳 **Docker Ready**: Available as a lightweight Docker image (built from scratch)
- ⚙️ **Flexible Configuration**: Configure via command-line flags or environment variables
- 🔄 **Multi-endpoint Support**: Handles primary and read replica endpoints

## Quick Start

### Valkey Instance (with IAM auth)

```bash
# Short name (on GCP)
docker run -d -p 6379:6379 \
  -e INSTANCE_NAME="my-valkey" \
  -e INSTANCE_TYPE="valkey" \
  ghcr.io/awasilyev/cloud-memstore-proxy:latest

# Full name
./cloud-memstore-proxy \
  -type valkey \
  -instance "projects/my-project/locations/us-central1/instances/my-valkey"
```

### Redis Instance (with password auth)

```bash
# Short name (on GCP)
docker run -d -p 6379:6379 \
  -e INSTANCE_NAME="my-redis" \
  -e INSTANCE_TYPE="redis" \
  ghcr.io/awasilyev/cloud-memstore-proxy:latest

# Full name
./cloud-memstore-proxy \
  -type redis \
  -instance "projects/my-project/locations/us-central1/instances/my-redis"
```

**Note**: The proxy automatically:
- Detects authentication mode (IAM or password)
- Retrieves password for Redis instances
- Configures TLS if enabled
- Discovers all endpoints

## Configuration

### Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-type` | Instance type: `valkey` or `redis` | `valkey` |
| `-instance` | Instance name - short (`my-instance`) or full (`projects/.../instances/...`) format (required) | - |
| `-local-addr` | Local address to bind to | `127.0.0.1` |
| `-start-port` | Starting port for first endpoint | `6379` |
| `-enable-iam-auth` | Enable IAM authentication (Valkey only) | `true` |
| `-tls-skip-verify` | Skip TLS certificate verification | `true` |
| `-verbose` | Enable verbose logging | `false` |

### Environment Variables

| Variable | Description | Equivalent Flag |
|----------|-------------|-----------------|
| `INSTANCE_NAME` | Instance name (short or full format) | `-instance` |
| `INSTANCE_TYPE` | Instance type (`valkey` or `redis`) | `-type` |
| `LOCAL_ADDR` | Local address to bind to | `-local-addr` |
| `ENABLE_IAM_AUTH` | Enable IAM authentication (Valkey only) | `-enable-iam-auth` |
| `TLS_SKIP_VERIFY` | Skip TLS certificate verification | `-tls-skip-verify` |
| `VERBOSE` | Enable verbose logging | `-verbose` |

### Instance Name Format

The proxy supports both short and full instance names:

**Short format** (when running on GCP):
```bash
# Valkey instance
./cloud-memstore-proxy -type valkey -instance my-valkey

# Redis instance
./cloud-memstore-proxy -type redis -instance my-redis
```
The proxy will automatically detect the project and region from GCP metadata.

**Full format** (works everywhere):
```
projects/PROJECT_ID/locations/LOCATION/instances/INSTANCE_ID
```

Example:
```bash
./cloud-memstore-proxy -type valkey -instance "projects/my-project/locations/us-central1/instances/my-valkey"
./cloud-memstore-proxy -type redis -instance "projects/my-project/locations/us-central1/instances/my-redis"
```

**Note**: Short format only works when running on GCP (GKE, GCE, Cloud Run, etc.) where metadata is available.

### How Short Names Work

When you provide a short instance name (e.g., `my-valkey`), the proxy:

1. Detects it's not a full resource path
2. Queries the GCP metadata service at `http://metadata.google.internal` to get:
   - Current project ID
   - Current zone (and derives region from it)
3. Constructs the full instance path: `projects/{project}/locations/{region}/instances/{name}`

This is particularly convenient when running on GKE where the proxy automatically uses the cluster's project and region.

## How It Works

### For Valkey Instances

1. **Discovery**: Uses [Memorystore for Valkey REST API](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances/get) to discover:
   - Discovery endpoints and PSC connections
   - Transit encryption mode (`SERVER_AUTHENTICATION`, `DISABLED`)
   - Authorization mode (`IAM_AUTH`, `AUTH_DISABLED`)
2. **TLS Setup**: If enabled, configures TLS with skip-verify for GCP self-signed certificates
3. **IAM Authentication**: For `IAM_AUTH` mode, uses GCP IAM tokens for each connection
4. **Proxying**: Transparent TCP proxy with zero-copy I/O

### For Redis Instances

1. **Discovery**: Uses [Memorystore for Redis REST API](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances/get) to discover:
   - Host, port, and read replica endpoints
   - Transit encryption mode
   - Authentication status
2. **Password Retrieval**: If auth is enabled, retrieves password via `authString` API
3. **TLS Setup**: Configures TLS if transit encryption is enabled
4. **Password Authentication**: Automatically sends `AUTH` command with retrieved password
5. **Proxying**: Transparent TCP proxy

### Local Listeners

Creates local TCP listeners for each endpoint:
- Port 6379: Primary endpoint
- Port 6380+: Read replicas/additional endpoints (if available)

## Performance Optimizations

The proxy is designed for minimal latency:

- **TCP_NODELAY**: Disables Nagle's algorithm for lower latency
- **TLS Session Resumption**: Efficient TLS handshakes
- **Zero-copy I/O**: Uses `io.Copy` for efficient data transfer
- **Keep-alive**: TCP keep-alive enabled for stable connections
- **Minimal Dependencies**: Built from scratch Docker image (~10MB)
- **Native TLS**: Uses Go's optimized crypto/tls package

## TLS/SSL Support

The proxy automatically handles TLS encryption based on your instance configuration:

### Automatic TLS Detection

When you create a Valkey instance with transit encryption enabled, the proxy:
1. Detects the `transitEncryptionMode` from the instance metadata
2. Automatically retrieves the CA certificate
3. Establishes secure TLS connections with proper certificate validation

### Manual TLS Configuration

TLS is automatically configured based on the instance settings. No manual configuration is required.

### TLS Performance

- **TLS 1.2+**: Minimum TLS version enforced
- **Session Reuse**: TLS session tickets supported
- **Hardware Acceleration**: Leverages CPU AES-NI instructions when available

## Building

### Build Binary

```bash
make build
```

### Build Docker Image

```bash
make docker-build
```

### Run Tests

```bash
make test
```

### Development Tools

Set up pre-commit hooks for automatic linting and testing:

```bash
# Install development tools (golangci-lint, goimports, etc.)
make install-tools

# Setup git hooks
make setup-hooks

# Run all checks manually
make check
```

The pre-commit hook will automatically run before each commit:
- Code formatting check
- Go vet
- Go mod tidy
- Unit tests
- Linting (if golangci-lint is installed)

## Authentication

### Valkey IAM Authentication

For Valkey instances with `IAM_AUTH` mode:

1. Obtains a GCP IAM access token using Application Default Credentials
2. Sends an AUTH command to Valkey with the token as password
3. Validates the authentication response
4. Proxies all subsequent traffic transparently

### Redis Password Authentication

For Redis instances with auth enabled:

1. Retrieves the instance auth string (password) via the API
2. Sends an AUTH command to Redis with the password
3. Validates the authentication response  
4. Proxies all subsequent traffic transparently

**Note**: Passwords are retrieved securely from the API and never stored persistently

### Setting up GCP Credentials

The proxy uses [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials):

#### Local Development
```bash
gcloud auth application-default login
```

#### In GKE/GCE
Credentials are automatically available via Workload Identity or Compute Engine service account

#### Using Service Account Key
```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

## Docker Deployment

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: valkey-auth-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: valkey-auth-proxy
  template:
    metadata:
      labels:
        app: valkey-auth-proxy
    spec:
      containers:
      - name: proxy
        image: valkey-auth-proxy:latest
        env:
        - name: VALKEY_INSTANCE_NAME
          value: "projects/my-project/locations/us-central1/instances/my-valkey"
        - name: ENABLE_IAM_AUTH
          value: "true"
        ports:
        - containerPort: 6379
          name: redis
        - containerPort: 6380
          name: redis-replica
```

### Docker Compose

```yaml
version: '3.8'
services:
  valkey-proxy:
    image: valkey-auth-proxy:latest
    environment:
      VALKEY_INSTANCE_NAME: "projects/my-project/locations/us-central1/instances/my-valkey"
      ENABLE_IAM_AUTH: "true"
    ports:
      - "6379:6379"
      - "6380:6380"
    volumes:
      - ~/.config/gcloud/application_default_credentials.json:/credentials/key.json:ro
    environment:
      GOOGLE_APPLICATION_CREDENTIALS: /credentials/key.json
```

## Connecting Clients

Once the proxy is running, connect your Redis/Valkey client to localhost:

### Python
```python
import redis

# Connect to primary (read-write)
client = redis.Redis(host='localhost', port=6379, decode_responses=True)
client.set('key', 'value')

# Connect to replica (read-only)
replica = redis.Redis(host='localhost', port=6380, decode_responses=True)
value = replica.get('key')
```

### Go
```go
import "github.com/redis/go-redis/v9"

client := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
```

### Node.js
```javascript
const redis = require('redis');
const client = redis.createClient({
    host: 'localhost',
    port: 6379
});
```

## Troubleshooting

### Connection Refused
- Ensure the proxy is running: `docker ps` or check process
- Verify the instance name is correct
- Check GCP credentials are properly configured

### Authentication Failed
- Ensure IAM authentication is enabled on your Valkey instance
- Verify your GCP credentials have the necessary permissions
- Check the service account has `memorystore.instances.get` permission

### TLS Handshake Failed
- Ensure the instance has transit encryption properly configured
- Verify the CA certificate was retrieved successfully (enable verbose logging)
- Check for clock skew between your system and GCP

### High Latency
- Ensure the proxy is running in the same region as your Valkey instance
- Check network connectivity between proxy and Valkey
- Enable verbose logging to diagnose bottlenecks

### Enable Debug Logging
```bash
./cloud-valkey-proxy -instance "..." -verbose=true
```

## Requirements

- Go 1.25 or later (for building)
- Docker (for containerization)
- GCP IAM credentials with appropriate permissions:
  - `memorystore.instances.get`
  - `memorystore.instances.getCertificateAuthority` (for TLS)
- Access to GCP Memorystore for Valkey

## API References

### Memorystore for Valkey
- [Instance Resource](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances#Instance)
- [Get Instance Method](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances/get)

### Memorystore for Redis
- [Instance Resource](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances#Instance)
- [Get Instance Method](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances/get)
- [Get Auth String](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances/getAuthString)

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

