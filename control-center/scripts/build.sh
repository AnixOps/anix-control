#!/bin/bash

# Build Script for All Platforms
# Usage: ./scripts/build.sh [version] [platform]

set -e

VERSION="${1:-dev}"
PLATFORM="${2:-all}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS="-s -w -X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'"

echo "Building AnixOps Control Center"
echo "Version: ${VERSION}"
echo "Commit: ${COMMIT}"
echo "Date: ${DATE}"
echo ""

# Clean
rm -rf dist/
mkdir -p dist

build_binary() {
    local GOOS=$1
    local GOARCH=$2
    local PLATFORM_NAME="${GOOS}-${GOARCH}"
    local OUTPUT_DIR="dist/${PLATFORM_NAME}"

    echo "Building for ${PLATFORM_NAME}..."

    mkdir -p "${OUTPUT_DIR}"

    # CLI binary
    BINARY_NAME="anixops"
    if [ "${GOOS}" = "windows" ]; then
        BINARY_NAME="anixops.exe"
    fi

    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags="${LDFLAGS}" \
        -o "${OUTPUT_DIR}/${BINARY_NAME}" \
        cmd/anixops/main.go

    # TUI binary
    TUI_NAME="anixops-tui"
    if [ "${GOOS}" = "windows" ]; then
        TUI_NAME="anixops-tui.exe"
    fi

    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags="${LDFLAGS}" \
        -o "${OUTPUT_DIR}/${TUI_NAME}" \
        cmd/anixops-tui/main.go

    # Create archive
    if [ "${GOOS}" = "windows" ]; then
        (cd dist && zip -r "anixops-${VERSION}-${PLATFORM_NAME}.zip" "${PLATFORM_NAME}")
    else
        (cd dist && tar -czvf "anixops-${VERSION}-${PLATFORM_NAME}.tar.gz" "${PLATFORM_NAME}")
    fi

    echo "✅ Built ${PLATFORM_NAME}"
}

# Build based on platform argument
case "${PLATFORM}" in
    "all")
        # Linux
        build_binary "linux" "amd64"
        build_binary "linux" "arm64"
        # Windows
        build_binary "windows" "amd64"
        build_binary "windows" "arm64"
        # macOS
        build_binary "darwin" "amd64"
        build_binary "darwin" "arm64"
        ;;
    "linux-amd64")
        build_binary "linux" "amd64"
        ;;
    "linux-arm64")
        build_binary "linux" "arm64"
        ;;
    "windows-amd64")
        build_binary "windows" "amd64"
        ;;
    "windows-arm64")
        build_binary "windows" "arm64"
        ;;
    "darwin-amd64")
        build_binary "darwin" "amd64"
        ;;
    "darwin-arm64")
        build_binary "darwin" "arm64"
        ;;
    *)
        echo "Unknown platform: ${PLATFORM}"
        echo "Available: all, linux-amd64, linux-arm64, windows-amd64, windows-arm64, darwin-amd64, darwin-arm64"
        exit 1
        ;;
esac

# Generate checksums
echo ""
echo "Generating checksums..."
cd dist
sha256sum *.tar.gz *.zip > checksums.sha256 2>/dev/null || true
sha512sum *.tar.gz *.zip > checksums.sha512 2>/dev/null || true
cd ..

echo ""
echo "Build complete! Artifacts in dist/"
ls -la dist/