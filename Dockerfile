# Build stage is not needed - GoReleaser pre-builds the binary
# This Dockerfile is used by GoReleaser to package the binary into a container image

FROM alpine:3.20

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN adduser -D -u 1000 etcd-reader

# Copy the binary from GoReleaser
COPY etcd-secret-reader /usr/local/bin/etcd-secret-reader

# Switch to non-root user
USER etcd-reader

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/etcd-secret-reader"]

# Default command (show help)
CMD ["--help"]
