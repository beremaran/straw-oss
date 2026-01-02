#!/bin/bash
set -e

# Default values
TARGET=${1:-server}
TAG_VERSION=${2:-latest}

# Determine binary name based on target
if [ "$TARGET" == "server" ]; then
    BINARY_NAME="relay-server"
    IMAGE_NAME="kwilabs/relay-server"
elif [ "$TARGET" == "endpoint" ]; then
    BINARY_NAME="endpoint"
    IMAGE_NAME="kwilabs/endpoint"
else
    echo "Usage: $0 [server|endpoint] [tag]"
    exit 1
fi

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "Building Docker image for $BINARY_NAME..."
echo "  Version:    $VERSION"
echo "  Commit:     $COMMIT"
echo "  Build Time: $BUILD_TIME"
echo "  Image:      $IMAGE_NAME:$TAG_VERSION"

# Build Docker image
docker build \
    --build-arg TARGET_BINARY=$BINARY_NAME \
    --build-arg VERSION=$VERSION \
    --build-arg COMMIT=$COMMIT \
    --build-arg BUILD_TIME=$BUILD_TIME \
    -t $IMAGE_NAME:$TAG_VERSION \
    -f Dockerfile .

echo "Build complete: $IMAGE_NAME:$TAG_VERSION"
