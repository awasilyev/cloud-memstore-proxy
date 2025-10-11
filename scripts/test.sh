#!/bin/bash
# Run tests with coverage

set -e

echo "Running tests..."

# Run tests with race detection and coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Display coverage
go tool cover -func=coverage.out | tail -1

echo ""
echo "✓ All tests passed!"
echo ""
echo "To view detailed coverage:"
echo "  go tool cover -html=coverage.out"

