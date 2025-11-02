# Dockerfile for GoReleaser
# GoReleaser provides the pre-built binary in the build context
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the pre-built binary from GoReleaser's build context
# With dockers_v2, the binary is in linux/amd64/foundry
COPY linux/amd64/foundry /usr/local/bin/foundry

# Run as non-root user
RUN addgroup -g 1000 foundry && \
    adduser -D -u 1000 -G foundry foundry

USER foundry

ENTRYPOINT ["foundry"]
CMD ["--help"]
