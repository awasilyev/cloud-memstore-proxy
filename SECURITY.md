# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Currently supported versions:

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability within Cloud Valkey Proxy, please send an email to the maintainer. All security vulnerabilities will be promptly addressed.

**Please do not open a public issue for security vulnerabilities.**

### What to include in your report:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if available)

### Response Timeline:

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Fix Release**: Depends on severity (critical issues within 7 days)

## Security Best Practices

When using Cloud Valkey Proxy:

1. **Use IAM Authentication**: Always enable IAM authentication for production environments
2. **TLS Encryption**: Ensure transit encryption is enabled on your Valkey instances
3. **Workload Identity**: Use Workload Identity on GKE instead of service account keys
4. **Network Security**: Deploy the proxy in the same VPC as your Valkey instance
5. **Least Privilege**: Grant only necessary IAM permissions
6. **Keep Updated**: Regularly update to the latest version
7. **Monitor Logs**: Enable verbose logging for security auditing when needed

## Required IAM Permissions

Minimum required permissions:
```
memorystore.instances.get
memorystore.instances.getCertificateAuthority
```

## Security Features

- ✅ TLS/SSL encryption in transit
- ✅ Certificate validation
- ✅ IAM-based authentication
- ✅ No credential storage
- ✅ Minimal container image (scratch-based)
- ✅ Read-only filesystem support
- ✅ Non-root execution

## Known Limitations

- The proxy must be able to access the GCP metadata service for short instance names
- IAM tokens are cached briefly for performance (standard OAuth2 token caching)
- Local connections to the proxy are unencrypted (use localhost only)

## Security Audit

This project has not undergone a formal security audit. Contributions for security improvements are welcome.

