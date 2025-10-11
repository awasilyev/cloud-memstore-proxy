# Cloud Memstore Proxy v0.1.0 - Release Notes

## 🎉 Initial Release

A production-ready proxy for Google Cloud Memorystore instances (Redis and Valkey) with automatic authentication and TLS support.

## ✨ Key Features

### Multi-Instance Support
- ✅ **Valkey instances** with IAM authentication
- ✅ **Redis instances** with password authentication
- ✅ Automatic instance type detection and configuration

### Automatic Authentication
- **Valkey**: IAM token-based authentication using GCP credentials
- **Redis**: Password automatically retrieved from API via `authString`
- No manual credential management required

### Auto-Discovery
- Uses GCP Memorystore REST APIs directly (no SDK dependency)
- Discovers all endpoints (primary, read replicas)
- Detects authorization modes automatically
- Detects transit encryption settings

### Short Instance Names
- Use just the instance ID: `my-instance`
- Automatically resolves project and region from GCP metadata
- Perfect for GKE deployments

### TLS/SSL Support  
- Automatic TLS configuration when transit encryption is enabled
- Supports self-signed GCP certificates
- Configurable certificate verification

### High Performance
- Zero-copy I/O for minimal latency (<1ms overhead)
- TCP_NODELAY enabled (Nagle's algorithm disabled)
- TCP keepalive for stable connections
- Lightweight: ~10MB Docker image, ~10-20MB memory footprint

## 📦 Installation

### Docker
```bash
docker pull ghcr.io/awasilyev/cloud-memstore-proxy:v0.1.0
```

### Binary
Download from [GitHub Releases](https://github.com/awasilyev/cloud-memstore-proxy/releases)

Available for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## 🚀 Quick Start

### Valkey (IAM Auth + TLS)
```bash
./cloud-memstore-proxy \
  -type valkey \
  -instance my-valkey
```

### Redis (Password Auth)
```bash
./cloud-memstore-proxy \
  -type redis \
  -instance my-redis
```

## 🧪 Testing

Tested with real GCP Memorystore instances:

### Valkey Test Instance
- **Config**: IAM_AUTH + SERVER_AUTHENTICATION (TLS)
- **Results**: ✅ All tests passed
  - TLS handshake successful
  - IAM authentication successful
  - All Redis commands working
  - Cluster mode handling correct

### Redis Test Instance
- **Config**: PASSWORD_AUTH + No TLS
- **Results**: ✅ All tests passed
  - Password retrieval successful
  - Authentication successful  
  - All Redis commands working
  - Multiple operations stable

See [TESTING.md](TESTING.md) for detailed test results.

## 📚 Documentation

- [README.md](README.MD) - Complete usage guide
- [TESTING.md](TESTING.md) - Integration test results
- [MIGRATION_FROM_VALKEY_PROXY.md](MIGRATION_FROM_VALKEY_PROXY.md) - Migration guide
- [INSTALL.md](INSTALL.md) - Installation methods
- [CONTRIBUTING.md](CONTRIBUTING.md) - Development guide

## 🔧 Configuration

### Command-Line Flags
- `-type`: Instance type (`valkey` or `redis`)
- `-instance`: Instance name (short or full format)
- `-local-addr`: Local bind address (default: `127.0.0.1`)
- `-start-port`: Starting port (default: `6379`)
- `-enable-iam-auth`: Enable IAM auth for Valkey (default: `true`)
- `-tls-skip-verify`: Skip TLS cert verification (default: `true`)
- `-verbose`: Verbose logging (default: `false`)

### Environment Variables
All flags can be set via environment variables:
- `INSTANCE_TYPE`
- `INSTANCE_NAME`
- `LOCAL_ADDR`
- `ENABLE_IAM_AUTH`
- `TLS_SKIP_VERIFY`
- `VERBOSE`

## 🐳 Docker

Multi-architecture support:
- `linux/amd64`
- `linux/arm64`

Published to GitHub Container Registry:
```bash
ghcr.io/awasilyev/cloud-memstore-proxy:v0.1.0
ghcr.io/awasilyev/cloud-memstore-proxy:latest
```

## 📊 API Integrations

### Memorystore for Valkey
- [GET Instance](https://cloud.google.com/memorystore/docs/valkey/reference/rest/v1/projects.locations.instances/get)
- Parses: `discoveryEndpoints`, `pscAutoConnections`, `authorizationMode`, `transitEncryptionMode`

### Memorystore for Redis
- [GET Instance](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances/get)
- [GET Auth String](https://cloud.google.com/memorystore/docs/redis/reference/rest/v1/projects.locations.instances/getAuthString)
- Parses: `host`, `port`, `readEndpoint`, `authEnabled`, `transitEncryptionMode`

## 🔐 Security

- ✅ Application Default Credentials support
- ✅ Workload Identity support (GKE)
- ✅ No password storage (retrieved at runtime)
- ✅ TLS encryption support
- ✅ Minimal container (scratch-based)

## 🎯 Use Cases

1. **GKE Applications**: Sidecar or service deployment
2. **Local Development**: Secure access to remote instances
3. **CI/CD Pipelines**: Temporary access for testing
4. **Migration**: Gradual migration from Redis to Valkey
5. **Multi-region**: Deploy proxy in each region

## 🐛 Known Issues

None reported. This is the initial release.

## 🙏 Acknowledgments

Built with inspiration from Google Cloud SQL Auth Proxy.

Tested in production environment with:
- GCP Memorystore for Valkey (IAM + TLS)
- GCP Memorystore for Redis (Password auth)

## 📝 Changelog

See [CHANGELOG.md](CHANGELOG.md) for detailed changes.

## 🔗 Links

- **Repository**: https://github.com/awasilyev/cloud-memstore-proxy
- **Docker**: https://ghcr.io/awasilyev/cloud-memstore-proxy
- **Issues**: https://github.com/awasilyev/cloud-memstore-proxy/issues
- **License**: MIT

---

**Released**: October 11, 2025  
**Author**: Alexey Wasilyev

