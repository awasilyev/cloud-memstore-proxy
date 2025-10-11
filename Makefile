.PHONY: build run test clean docker-build docker-run fmt lint setup-hooks

BINARY_NAME=cloud-memstore-proxy
DOCKER_IMAGE=ghcr.io/awasilyev/cloud-memstore-proxy
DOCKER_TAG=latest

# Build the binary
build:
	go build -o $(BINARY_NAME) main.go

# Run locally
run: build
	./$(BINARY_NAME)

# Run tests with coverage
test:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -1

# Run tests (short version for pre-commit)
test-short:
	go test -race -short ./...

# Format code
fmt:
	go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		find . -name '*.go' -not -path './vendor/*' -exec goimports -w {} \; ; \
	fi

# Run linter
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./... ; \
	else \
		echo "golangci-lint not installed. Install with:" ; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" ; \
		exit 1 ; \
	fi

# Go vet
vet:
	go vet ./...

# Setup git hooks
setup-hooks:
	@chmod +x scripts/setup-hooks.sh
	@./scripts/setup-hooks.sh

# Install development tools
install-tools:
	@chmod +x scripts/install-tools.sh
	@./scripts/install-tools.sh

# Run all checks (like CI)
check: fmt vet lint test
	@echo "All checks passed!"

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Run Docker container (requires environment variables)
docker-run:
	docker run --rm \
		-p 6379:6379 \
		-p 6380:6380 \
		-e VALKEY_INSTANCE_NAME=$(VALKEY_INSTANCE_NAME) \
		-e GOOGLE_APPLICATION_CREDENTIALS=/credentials/key.json \
		-v $(GOOGLE_APPLICATION_CREDENTIALS):/credentials/key.json:ro \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Download dependencies
deps:
	go mod download
	go mod tidy

