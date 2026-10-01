#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REPO_ROOT="$(cd "${SDK_ROOT}/.." && pwd)"
TOOLS_FILE="${REPO_ROOT}/tools.go"
MODULE_PATH="github.com/AnixOps/anix-control/sdk"
PROTO_FILE="api/kernelnodeops/v1/kernel_node_ops.proto"

cd "${SDK_ROOT}"

read_tool_pin() {
  local name="$1"
  local value
  value="$(sed -n -E "s/^[[:space:]]*${name}[[:space:]]*=[[:space:]]*\"([^\"]+)\".*/\1/p" "${TOOLS_FILE}")"
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
if [[ "$(protoc --version)" != "libprotoc ${PROTOC_VERSION}" ]]; then
  echo "error: protoc version does not match tools.go" >&2
  exit 1
fi

TOOL_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TOOL_DIR}"; }
trap cleanup EXIT

GOBIN="${TOOL_DIR}" GOWORK=off go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
GOBIN="${TOOL_DIR}" GOWORK=off go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${PROTOC_GEN_GO_GRPC_VERSION}"

protoc \
  --plugin="protoc-gen-go=${TOOL_DIR}/protoc-gen-go" \
  --plugin="protoc-gen-go-grpc=${TOOL_DIR}/protoc-gen-go-grpc" \
  --go_out=. \
  --go_opt="module=${MODULE_PATH}" \
  --go-grpc_out=. \
  --go-grpc_opt="module=${MODULE_PATH}" \
  "${PROTO_FILE}"

echo "Generated kernelnodeops gRPC bindings."
