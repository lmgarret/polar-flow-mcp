# syntax=docker/dockerfile:1

# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./
RUN CGO_ENABLED=0 go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s" \
    -o polar-flow-mcp \
    ./cmd/polar-flow-mcp

# Stage 2: Minimal final image
FROM scratch

# TLS certificates required for outbound HTTPS to Polar API
# Without this, all HTTPS calls fail with x509: certificate signed by unknown authority
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Binary only — no shell, no package manager, no OS
COPY --from=builder /build/polar-flow-mcp /polar-flow-mcp

EXPOSE 8080

ENTRYPOINT ["/polar-flow-mcp"]
