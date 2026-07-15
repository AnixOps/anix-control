#!/usr/bin/env bash
set -euo pipefail

# Compatibility entrypoint. The canonical generator covers both the legacy
# v2board wire package and the v3 AnixOps Agent control stream.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/../../api/grpc/gen.sh" "$@"
