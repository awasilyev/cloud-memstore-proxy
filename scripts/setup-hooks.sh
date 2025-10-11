#!/bin/bash
# Setup git hooks for the repository

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.githooks"
GIT_HOOKS_DIR="$REPO_ROOT/.git/hooks"

echo "Setting up git hooks..."

# Create git hooks directory if it doesn't exist
mkdir -p "$GIT_HOOKS_DIR"

# Copy or link the pre-commit hook
if [ -f "$HOOKS_DIR/pre-commit" ]; then
    echo "Installing pre-commit hook..."
    cp "$HOOKS_DIR/pre-commit" "$GIT_HOOKS_DIR/pre-commit"
    chmod +x "$GIT_HOOKS_DIR/pre-commit"
    echo "✓ Pre-commit hook installed"
else
    echo "Warning: pre-commit hook not found in $HOOKS_DIR"
fi

# Set git to use the hooks directory
git config core.hooksPath .githooks

echo ""
echo "Git hooks setup complete!"
echo ""
echo "The following checks will run before each commit:"
echo "  1. Code formatting (gofmt)"
echo "  2. Go vet"
echo "  3. Go mod tidy"
echo "  4. Unit tests"
echo "  5. Linting (if golangci-lint is installed)"
echo ""
echo "To skip hooks for a specific commit, use: git commit --no-verify"
echo ""
echo "To install golangci-lint:"
echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"

