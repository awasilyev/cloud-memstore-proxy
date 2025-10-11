# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Multi-instance support**: Works with both Valkey and Redis instances
- **Memorystore for Valkey** support:
  - REST API-based discovery (no SDK dependency)
  - IAM token-based authentication
  - discoveryEndpoints and pscAutoConnections parsing
  - Authorization mode detection (IAM_AUTH, AUTH_DISABLED)
- **Memorystore for Redis** support:
  - REST API-based discovery
  - Automatic password retrieval via authString API
  - Password-based authentication
  - Read replica endpoint support
- **Short instance name support**: Use just instance ID, automatically resolves project/region from GCP metadata
- **Automatic TLS/SSL support** with skip-verify for GCP self-signed certificates
- Transit encryption mode detection and configuration
- Multi-endpoint support via discoveryEndpoints API
- Automatic local proxy creation for each endpoint
- TCP performance optimizations (TCP_NODELAY, keepalive)
- TLS 1.2+ support
- Docker container support with minimal image size
- Kubernetes deployment examples
- Pre-commit hooks for linting and testing
- Comprehensive documentation and examples
- Command-line flags and environment variable configuration
- Verbose logging mode for debugging
- Graceful shutdown handling
- Zero-copy I/O for efficient data transfer
- GCP metadata integration for seamless operation on GKE/GCE

### Performance Features
- Zero-copy I/O using `io.Copy`
- Nagle's algorithm disabled (TCP_NODELAY)
- TCP keepalive enabled
- Minimal memory footprint
- Built from scratch Docker image (~10MB)

### Security Features
- GCP IAM authentication
- Application Default Credentials support
- Workload Identity support for GKE
- Secure token handling

## [0.1.0] - 2025-10-11

### Added
- Initial project structure
- Core proxy functionality
- GCP integration
- Docker support
- Documentation

