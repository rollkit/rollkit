# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev linux-headers

# Set working directory
WORKDIR /rollkit

# Copy go mod files
COPY go.mod go.sum ./
COPY apps/grpc/single/go.mod apps/grpc/single/go.sum ./apps/grpc/single/
COPY core/go.mod core/go.sum ./core/
COPY execution/grpc/go.mod execution/grpc/go.sum ./execution/grpc/
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY sequencers/single/go.mod sequencers/single/go.sum ./sequencers/single/

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
WORKDIR /rollkit/apps/grpc/single
RUN go build -o grpc-single .

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 rollkit && \
    adduser -u 1000 -G rollkit -s /bin/sh -D rollkit

# Set working directory
WORKDIR /home/rollkit

# Copy binary from builder
COPY --from=builder /rollkit/apps/grpc/single/grpc-single /usr/local/bin/

# Create necessary directories
RUN mkdir -p /home/rollkit/.grpc-single && \
    chown -R rollkit:rollkit /home/rollkit

# Switch to non-root user
USER rollkit

# Expose ports
# P2P port
EXPOSE 26656
# RPC port
EXPOSE 26657
# Prometheus metrics
EXPOSE 26660

# Set entrypoint
ENTRYPOINT ["grpc-single"]

# Default command
CMD ["start"]