# Multi-stage build for distninja

# Build stage
FROM ubuntu:24.04 AS builder

# Build arguments for version info
ARG BUILD_TIME
ARG COMMIT_ID

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

# Build the application with buildTime and commitID
RUN buildTime=${BUILD_TIME:-$(date +%FT%T%z)} && \
    commitID=${COMMIT_ID:-"unknown"} && \
    ldflags="-s -w -X github.com/distninja/distninja/cmd.BuildTime=$buildTime -X github.com/distninja/distninja/cmd.CommitID=$commitID" && \
    go env -w GOPROXY=https://goproxy.cn,direct && \
    CGO_ENABLED=0 GOARCH=$(go env GOARCH) GOOS=$(go env GOOS) go build -ldflags "$ldflags" -o bin/distninja .

# Runtime stage
FROM ubuntu:24.04

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/bin/distninja /usr/local/bin/distninja

# Expose ports
EXPOSE 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD distninja --help || exit 1

# Create data directory
RUN mkdir -p /data

# Create entrypoint script to handle database initialization
RUN cat > /entrypoint.sh << 'EOF'
#!/bin/bash
set -e

is_valid_db() {
    local file="$1"
    if [ ! -f "$file" ]; then
        return 1
    fi
    if [ ! -s "$file" ]; then
        return 1
    fi
    # Check for magic bytes at the beginning of the file
    local magic=$(head -c 4 "$file" 2>/dev/null | xxd -p 2>/dev/null || echo "")
    if [ "$magic" != "ed0cdaed" ]; then
        return 1
    fi
    return 0
}

store_path="/data/ninja.db"
for i in "${@}"; do
    if [[ "$prev_arg" == "--store" || "$prev_arg" == "-s" ]]; then
        store_path="$i"
        break
    fi
    prev_arg="$i"
done

mkdir -p "$(dirname "$store_path")"

if [ -f "$store_path" ] && ! is_valid_db "$store_path"; then
    echo "Warning: Database file $store_path exists but appears to be invalid or empty. Removing it to allow reinitialization."
    rm -f "$store_path"
fi

exec "$@"
EOF

RUN chmod +x /entrypoint.sh

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]

# Default command - use /data directory for database
CMD ["distninja", "serve", "--http", ":9090", "--store", "/data/ninja.db"]
