#!/bin/bash
# Install development tools for the project

set -e

echo "Installing development tools..."

# golangci-lint
if ! command -v golangci-lint &> /dev/null; then
    echo "Installing golangci-lint..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    echo "✓ golangci-lint installed"
else
    echo "✓ golangci-lint already installed"
fi

# goimports (better than gofmt)
if ! command -v goimports &> /dev/null; then
    echo "Installing goimports..."
    go install golang.org/x/tools/cmd/goimports@latest
    echo "✓ goimports installed"
else
    echo "✓ goimports already installed"
fi

# pre-commit (optional, Python-based)
if command -v python3 &> /dev/null || command -v python &> /dev/null; then
    if ! command -v pre-commit &> /dev/null; then
        echo ""
        echo "Optional: Install pre-commit framework"
        echo "  pip install pre-commit"
        echo "  Then run: pre-commit install"
    else
        echo "✓ pre-commit already installed"
    fi
fi

echo ""
echo "Development tools installed!"
echo ""
echo "Next steps:"
echo "  1. Run: ./scripts/setup-hooks.sh"
echo "  2. (Optional) Run: pre-commit install  # If you have pre-commit framework"

