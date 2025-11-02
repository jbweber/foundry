# Multi-stage build for Foundry
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o foundry ./cmd/foundry

# Final minimal image
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/foundry /usr/local/bin/foundry

# Run as non-root user
RUN addgroup -g 1000 foundry && \
    adduser -D -u 1000 -G foundry foundry

USER foundry

ENTRYPOINT ["foundry"]
CMD ["--help"]
