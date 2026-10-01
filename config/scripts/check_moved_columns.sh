#!/usr/bin/env bash
# The static gate of the node credential split: no kernel read of a moved
# credential column outside internal/nodesecrets
# (docs/architecture/node-ops-service.md, section 4.3). The rules and the
# reasoned exceptions are in check_moved_columns.go.
set -euo pipefail

repository="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repository}"
GOWORK="${GOWORK:-off}" go run ./config/scripts/check_moved_columns.go "$@"
