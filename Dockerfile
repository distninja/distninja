# Multi-stage build for distninja

# Build stage
FROM ubuntu:24.04 AS builder

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV GO_VERSION=1.24.4
ENV GOPROXY=https://goproxy.cn,direct

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN curl -L https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV PATH=/usr/local/go/bin:$PATH

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN chmod +x script/build.sh && ./script/build.sh

# Runtime stage
FROM ubuntu:24.04

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Create non-root user
RUN groupadd -r distninja && useradd -r -g distninja -s /bin/bash distninja

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/bin/distninja /usr/local/bin/distninja

# Change ownership
RUN chown -R distninja:distninja /app

# Switch to non-root user
USER distninja

# Expose ports
EXPOSE 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD distninja --help || exit 1

# Default command
CMD ["distninja", "serve", "--http", ":9090", "--store", "/tmp/ninja.db"]
