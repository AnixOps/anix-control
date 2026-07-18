#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_PATH="github.com/AnixOps/anix-control/v4"
PROTO_FILES=(
  "api/pluginhost/v1/control_host.proto"
)

cd "${REPO_ROOT}"

read_tool_pin() {
  local name="$1"
  local value

  value="$(sed -n -E "s/^[[:space:]]*${name}[[:space:]]*=[[:space:]]*\"([^\"]+)\".*/\1/p" tools.go)"
  if [[ -z "${value}" ]]; then
    echo "error: ${name} is not pinned in tools.go" >&2
    exit 1
  fi
  printf '%s' "${value}"
}

for command_name in protoc go; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "error: ${command_name} not found" >&2
    exit 1
  fi
done

PROTOC_VERSION="$(read_tool_pin ProtocVersion)"
PROTOC_GEN_GO_VERSION="$(read_tool_pin ProtocGenGoVersion)"
PROTOC_GEN_GO_GRPC_VERSION="$(read_tool_pin ProtocGenGoGRPCVersion)"
PROTOC_ACTUAL_VERSION="$(protoc --version)"
PROTOC_EXPECTED_VERSION="libprotoc ${PROTOC_VERSION}"

if [[ "${PROTOC_ACTUAL_VERSION}" != "${PROTOC_EXPECTED_VERSION}" ]]; then
  echo "error: expected ${PROTOC_EXPECTED_VERSION}, got ${PROTOC_ACTUAL_VERSION}" >&2
  exit 1
fi

TOOL_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TOOL_DIR}"
}
trap cleanup EXIT

GOBIN="${TOOL_DIR}" GOWORK=off go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
GOBIN="${TOOL_DIR}" GOWORK=off go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${PROTOC_GEN_GO_GRPC_VERSION}"

if [[ "$("${TOOL_DIR}/protoc-gen-go" --version)" != "protoc-gen-go ${PROTOC_GEN_GO_VERSION}" ]]; then
  echo "error: protoc-gen-go version does not match tools.go" >&2
  exit 1
fi
if [[ "$("${TOOL_DIR}/protoc-gen-go-grpc" --version)" != "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}" ]]; then
  echo "error: protoc-gen-go-grpc version does not match tools.go" >&2
  exit 1
fi

protoc \
  --plugin="protoc-gen-go=${TOOL_DIR}/protoc-gen-go" \
  --plugin="protoc-gen-go-grpc=${TOOL_DIR}/protoc-gen-go-grpc" \
  --go_out=. \
  --go_opt="module=${MODULE_PATH}" \
  --go-grpc_out=. \
  --go-grpc_opt="module=${MODULE_PATH}" \
  "${PROTO_FILES[@]}"

echo "Generated Control Plugin Host gRPC bindings."
