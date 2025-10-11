#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building Cloud Memstore Proxy...${NC}"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi

# Get version from git or default
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS="-w -s"
LDFLAGS="$LDFLAGS -X main.Version=$VERSION"
LDFLAGS="$LDFLAGS -X main.BuildTime=$BUILD_TIME"
LDFLAGS="$LDFLAGS -X main.GitCommit=$GIT_COMMIT"

# Tidy dependencies
echo -e "${YELLOW}Tidying dependencies...${NC}"
go mod tidy

# Run tests
echo -e "${YELLOW}Running tests...${NC}"
go test -v ./... || true

# Build binary
echo -e "${YELLOW}Building binary...${NC}"
CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o cloud-memstore-proxy main.go

echo -e "${GREEN}Build complete!${NC}"
echo -e "Binary: ${YELLOW}./cloud-memstore-proxy${NC}"
echo -e "Version: ${YELLOW}$VERSION${NC}"
echo -e "Commit: ${YELLOW}$GIT_COMMIT${NC}"
echo -e "Build Time: ${YELLOW}$BUILD_TIME${NC}"

# Make binary executable
chmod +x cloud-memstore-proxy

echo -e "\n${GREEN}To run:${NC}"
echo -e "./cloud-memstore-proxy -type valkey -instance \"projects/YOUR_PROJECT/locations/YOUR_LOCATION/instances/YOUR_INSTANCE\""
echo -e "./cloud-memstore-proxy -type redis -instance \"projects/YOUR_PROJECT/locations/YOUR_LOCATION/instances/YOUR_INSTANCE\""

