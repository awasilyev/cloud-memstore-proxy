# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations for size and performance
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o cloud-memstore-proxy \
    main.go

# Final stage
FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /build/cloud-memstore-proxy /cloud-memstore-proxy

# Expose default ports (6379-6389 to support up to 10 endpoints)
EXPOSE 6379 6380 6381 6382 6383 6384 6385 6386 6387 6388 6389

# Run the proxy
ENTRYPOINT ["/cloud-memstore-proxy"]

