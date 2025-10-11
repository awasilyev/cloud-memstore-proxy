# Installation Guide

This guide covers different ways to install and run Cloud Valkey Proxy.

## Quick Start

### Docker (Recommended)

Pull the latest image:
```bash
docker pull ghcr.io/awasilyev/cloud-valkey-proxy:latest
```

Run with short instance name (on GCP):
```bash
docker run -d \
  --name cloud-valkey-proxy \
  -p 6379:6379 \
  -e VALKEY_INSTANCE_NAME="my-valkey" \
  ghcr.io/awasilyev/cloud-valkey-proxy:latest
```

Run with full instance name:
```bash
docker run -d \
  --name cloud-valkey-proxy \
  -p 6379:6379 \
  -e VALKEY_INSTANCE_NAME="projects/my-project/locations/us-central1/instances/my-valkey" \
  -e GOOGLE_APPLICATION_CREDENTIALS=/creds/key.json \
  -v ~/.config/gcloud:/creds:ro \
  ghcr.io/awasilyev/cloud-valkey-proxy:latest
```

### Pre-built Binary

Download from [GitHub Releases](https://github.com/awasilyev/cloud-valkey-proxy/releases):

**Linux:**
```bash
curl -L https://github.com/awasilyev/cloud-valkey-proxy/releases/latest/download/cloud-valkey-proxy-linux-amd64.tar.gz | tar xz
chmod +x cloud-valkey-proxy-linux-amd64
sudo mv cloud-valkey-proxy-linux-amd64 /usr/local/bin/cloud-valkey-proxy
```

**macOS:**
```bash
curl -L https://github.com/awasilyev/cloud-valkey-proxy/releases/latest/download/cloud-valkey-proxy-darwin-amd64.tar.gz | tar xz
chmod +x cloud-valkey-proxy-darwin-amd64
sudo mv cloud-valkey-proxy-darwin-amd64 /usr/local/bin/cloud-valkey-proxy
```

**Windows:**
Download the `.zip` file from the releases page and extract it to a location in your PATH.

### Build from Source

Requirements:
- Go 1.25 or later
- Git

```bash
# Clone the repository
git clone https://github.com/awasilyev/cloud-valkey-proxy.git
cd cloud-valkey-proxy

# Build
./build.sh

# Or use go directly
go build -o cloud-valkey-proxy main.go

# Install
sudo mv cloud-valkey-proxy /usr/local/bin/
```

## Installation Methods

### 1. Kubernetes (Sidecar Pattern)

Add as a sidecar container:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-app:latest
        env:
        - name: VALKEY_HOST
          value: "localhost"
        - name: VALKEY_PORT
          value: "6379"
      
      - name: cloud-valkey-proxy
        image: ghcr.io/awasilyev/cloud-valkey-proxy:latest
        env:
        - name: VALKEY_INSTANCE_NAME
          value: "my-valkey"
        ports:
        - containerPort: 6379
          name: valkey
```

### 2. Kubernetes (Separate Deployment)

Deploy as a standalone service:

```bash
kubectl apply -f examples/kubernetes-deployment.yaml
```

Then connect your applications to the service:
```yaml
env:
- name: VALKEY_HOST
  value: "valkey-auth-proxy.default.svc.cluster.local"
- name: VALKEY_PORT
  value: "6379"
```

### 3. Docker Compose

```yaml
version: '3.8'
services:
  valkey-proxy:
    image: ghcr.io/awasilyev/cloud-valkey-proxy:latest
    environment:
      VALKEY_INSTANCE_NAME: "my-valkey"
    ports:
      - "6379:6379"
  
  app:
    image: my-app
    environment:
      VALKEY_HOST: valkey-proxy
      VALKEY_PORT: 6379
    depends_on:
      - valkey-proxy
```

### 4. Systemd Service

Create `/etc/systemd/system/cloud-valkey-proxy.service`:

```ini
[Unit]
Description=Cloud Valkey Proxy
After=network.target

[Service]
Type=simple
User=valkey-proxy
ExecStart=/usr/local/bin/cloud-valkey-proxy \
  -instance=my-valkey \
  -local-addr=0.0.0.0 \
  -verbose=false
Restart=on-failure
RestartSec=5s

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/run/cloud-valkey-proxy

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable cloud-valkey-proxy
sudo systemctl start cloud-valkey-proxy
```

### 5. GCE VM Startup Script

Add to your GCE instance metadata or startup script:

```bash
#!/bin/bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Run proxy
docker run -d \
  --name cloud-valkey-proxy \
  --restart always \
  -p 6379:6379 \
  -e VALKEY_INSTANCE_NAME="my-valkey" \
  ghcr.io/awasilyev/cloud-valkey-proxy:latest
```

### 6. Cloud Run (If Needed)

```bash
gcloud run deploy cloud-valkey-proxy \
  --image=ghcr.io/awasilyev/cloud-valkey-proxy:latest \
  --set-env-vars="VALKEY_INSTANCE_NAME=my-valkey" \
  --port=6379 \
  --region=us-central1
```

## Configuration

### Environment Variables

All command-line flags can be set via environment variables:

| Environment Variable | Flag | Description |
|---------------------|------|-------------|
| `VALKEY_INSTANCE_NAME` | `-instance` | Instance name (required) |
| `LOCAL_ADDR` | `-local-addr` | Local bind address |
| `ENABLE_IAM_AUTH` | `-enable-iam-auth` | Enable IAM auth |
| `VERBOSE` | `-verbose` | Verbose logging |

### GCP Authentication

The proxy uses Application Default Credentials. Setup options:

**On GKE (Recommended):**
- Use Workload Identity
- No configuration needed

**On GCE:**
- Uses instance service account
- No configuration needed

**Local Development:**
```bash
gcloud auth application-default login
```

**With Service Account Key:**
```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

## Verification

Test the proxy is working:

```bash
# Using redis-cli
redis-cli -h localhost -p 6379 PING

# Using Python
python3 << EOF
import redis
r = redis.Redis(host='localhost', port=6379)
print(r.ping())
EOF
```

## Upgrading

### Docker

```bash
docker pull ghcr.io/awasilyev/cloud-valkey-proxy:latest
docker stop cloud-valkey-proxy
docker rm cloud-valkey-proxy
# Run with new image
```

### Binary

```bash
# Download new version
curl -L https://github.com/awasilyev/cloud-valkey-proxy/releases/latest/download/cloud-valkey-proxy-linux-amd64.tar.gz | tar xz

# Replace binary
sudo systemctl stop cloud-valkey-proxy
sudo mv cloud-valkey-proxy-linux-amd64 /usr/local/bin/cloud-valkey-proxy
sudo systemctl start cloud-valkey-proxy
```

### Kubernetes

```bash
kubectl set image deployment/valkey-auth-proxy \
  proxy=ghcr.io/awasilyev/cloud-valkey-proxy:v0.2.0
```

## Uninstall

### Docker
```bash
docker stop cloud-valkey-proxy
docker rm cloud-valkey-proxy
docker rmi ghcr.io/awasilyev/cloud-valkey-proxy
```

### Binary
```bash
sudo rm /usr/local/bin/cloud-valkey-proxy
```

### Systemd
```bash
sudo systemctl stop cloud-valkey-proxy
sudo systemctl disable cloud-valkey-proxy
sudo rm /etc/systemd/system/cloud-valkey-proxy.service
sudo systemctl daemon-reload
```

### Kubernetes
```bash
kubectl delete -f examples/kubernetes-deployment.yaml
```

## Troubleshooting

See [README.md#troubleshooting](README.MD#troubleshooting) for common issues and solutions.

## Next Steps

- Read the [README](README.MD) for usage details
- Check [examples/](examples/) for more deployment patterns
- See [CONTRIBUTING.md](CONTRIBUTING.md) to contribute


