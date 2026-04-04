#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build"

echo "Building Frontend..."
cd "${SCRIPT_DIR}/web"
npm install
npm run build

echo "Building Backend..."
mkdir -p "${BUILD_DIR}"
cd "${SCRIPT_DIR}"
GOWORK=off go build -o "${BUILD_DIR}/v2board" ./cmd/server

echo "Build Complete!"
