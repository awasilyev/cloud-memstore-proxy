#!/bin/bash
# Format Go code

set -e

echo "Formatting Go code..."

# Format all Go files
go fmt ./...

# If goimports is available, use it (it's better than gofmt)
if command -v goimports &> /dev/null; then
    echo "Running goimports..."
    find . -name '*.go' -not -path './vendor/*' -exec goimports -w {} \;
fi

echo "✓ Code formatted!"

