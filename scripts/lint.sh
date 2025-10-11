#!/bin/bash
# Run linting checks

set -e

echo "Running linting checks..."

# Check if golangci-lint is installed
if ! command -v golangci-lint &> /dev/null; then
    echo "Error: golangci-lint is not installed"
    echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
    exit 1
fi

# Run golangci-lint
golangci-lint run --timeout=5m ./...

echo "✓ Linting passed!"

