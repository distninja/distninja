#!/bin/bash

# Script to build Docker image with proper build arguments
set -e

# Default values
IMAGE_TAG="${1:-distninja:latest}"
PUSH="${2:-false}"

# Get build time and commit ID
buildTime=$(date +%FT%T%z)
commitID=$(git rev-parse --short=7 HEAD 2>/dev/null || echo "unknown")

echo "Building Docker image with:"
echo "  Build Time: $buildTime"
echo "  Commit ID: $commitID"
echo "  Image Tag: $IMAGE_TAG"

# Build Docker image with build arguments
docker build \
    --build-arg BUILD_TIME="$buildTime" \
    --build-arg COMMIT_ID="$commitID" \
    -t "$IMAGE_TAG" \
    .

echo "Docker image built successfully!"

# Push if requested
if [ "$PUSH" = "true" ]; then
    echo "Pushing image to registry..."
    docker push "$IMAGE_TAG"
    echo "Image pushed successfully!"
fi
