# Environment Variables Reference

All configuration parameters can be set via environment variables. Command-line flags take precedence.

## Quick Reference

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `INSTANCE_NAME` | string | *required* | Instance name (short or full format) |
| `INSTANCE_TYPE` | string | `valkey` | Instance type: `valkey` or `redis` |
| `LOCAL_ADDR` | string | `127.0.0.1` | Local address to bind to |
| `START_PORT` | int | `6379` | Starting port for first endpoint |
| `ENABLE_IAM_AUTH` | bool | `true` | Enable IAM authentication (Valkey only) |
| `TLS_SKIP_VERIFY` | bool | `true` | Skip TLS certificate verification |
| `VERBOSE` | bool | `false` | Enable verbose logging |

## Boolean Values

Boolean environment variables accept multiple formats:
- **True**: `true`, `1`, `yes`
- **False**: `false`, `0`, `no`, or empty

## Examples

### Minimal Configuration (Valkey)
```bash
export INSTANCE_NAME="my-valkey"
export INSTANCE_TYPE="valkey"
./cloud-memstore-proxy
```

### Full Configuration (Redis)
```bash
export INSTANCE_TYPE="redis"
export INSTANCE_NAME="projects/my-project/locations/us-central1/instances/my-redis"
export LOCAL_ADDR="0.0.0.0"
export START_PORT="6379"
export ENABLE_IAM_AUTH="false"
export TLS_SKIP_VERIFY="true"
export VERBOSE="true"
./cloud-memstore-proxy
```

### Docker with .env File

Create a `.env` file:
```env
INSTANCE_TYPE=valkey
INSTANCE_NAME=my-valkey
LOCAL_ADDR=0.0.0.0
VERBOSE=false
```

Run with docker:
```bash
docker run -d -p 6379:6379 --env-file .env \
  ghcr.io/awasilyev/cloud-memstore-proxy:latest
```

### Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: memstore-proxy-config
data:
  INSTANCE_TYPE: "valkey"
  INSTANCE_NAME: "my-valkey"
  LOCAL_ADDR: "0.0.0.0"
  START_PORT: "6379"
  ENABLE_IAM_AUTH: "true"
  TLS_SKIP_VERIFY: "true"
  VERBOSE: "false"
```

### Override with Flags

Environment variables can be overridden with command-line flags:

```bash
# Set via env
export INSTANCE_NAME="my-valkey"
export VERBOSE="false"

# Override verbose with flag
./cloud-memstore-proxy -verbose=true
# Result: verbose=true (flag takes precedence)
```

## Special Environment Variables

### GCP Authentication

```bash
# Use service account key file
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json

# Or use Application Default Credentials (recommended)
gcloud auth application-default login
```

### Debug Discovery

```bash
# Enable debug output for API calls
export DEBUG_DISCOVERY="true"
./cloud-memstore-proxy -instance my-valkey
```

## Validation

The proxy validates configuration at startup:

- ✅ `INSTANCE_NAME` is required
- ✅ `INSTANCE_TYPE` must be `valkey` or `redis`
- ✅ `START_PORT` must be a valid port number (1-65535)
- ✅ `LOCAL_ADDR` must be a valid IP address

Invalid values will cause the proxy to exit with an error message.

## Best Practices

1. **Use env vars in containers** - Easier to manage in Kubernetes/Docker
2. **Use flags for local development** - Easier to test different configs
3. **Use .env files** - Keep configuration organized
4. **Use ConfigMaps in K8s** - Central configuration management
5. **Don't commit credentials** - Use secrets/Workload Identity

## Priority Order

When the same parameter is set in multiple places:

1. **Command-line flag** (highest priority)
2. **Environment variable**
3. **Default value** (lowest priority)

Example:
```bash
# All three set
export START_PORT="7000"           # Priority 2
./cloud-memstore-proxy \
  -start-port 8000                 # Priority 1 (wins!)
# Default would be 6379            # Priority 3

# Result: Uses port 8000
```

## Troubleshooting

### Environment Variable Not Working

```bash
# Check if variable is set
echo $INSTANCE_NAME

# Check effective configuration (use verbose mode)
./cloud-memstore-proxy -verbose=true

# Check for typos
env | grep INSTANCE
```

### Boolean Not Parsing

```bash
# ✅ Correct
export VERBOSE="true"
export VERBOSE="1"
export VERBOSE="yes"

# ❌ Incorrect
export VERBOSE="True"  # Case sensitive!
export VERBOSE="TRUE"  # Case sensitive!
```

### Port Number Issues

```bash
# ✅ Correct
export START_PORT="6379"
export START_PORT="7000"

# ❌ Incorrect
export START_PORT="abc"    # Not a number
export START_PORT="99999"  # Out of range
```

## See Also

- [config.example](config.example) - Example configuration file
- [README.MD](README.MD) - Main documentation
- [examples/](examples/) - Kubernetes and Docker Compose examples

