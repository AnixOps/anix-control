#!/usr/bin/env bash
# Execute the official GOST Mesh package release contract with real Agent and
# pinned GOST executables. No signing private key is persisted or uploaded.

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
readonly ROOT_DIR
readonly PACKAGE_ROOT="${ROOT_DIR}/packages/gost-mesh"
readonly BUILDER="${PACKAGE_ROOT}/build.py"
readonly VERIFIER="${PACKAGE_ROOT}/verify_signature.py"
readonly SEMANTIC_RUNNER="${PACKAGE_ROOT}/tests/run_semantic_contract.py"
readonly SEMANTIC_CASES="${PACKAGE_ROOT}/tests/semantic_contract_cases.json"
readonly SIGNING_SCRIPT="${ROOT_DIR}/scripts/sign_plugin_release.sh"
readonly GOST_VERSION="3.2.6"
readonly GOST_AMD64_SHA256="a2aea24efb4597b5f57b35b8e1bbcc59f439b80723854d4371f6828b46682ffb"
PYTHON_BIN="${PYTHON_BIN:-python3}"
OPENSSL_BIN="${OPENSSL_BIN:-openssl}"
WORK_DIR=""

cleanup() {
  if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
    rm -rf -- "${WORK_DIR}"
  fi
}

fail() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: packages/gost-mesh/tests/release_gate.sh \
  --agent-binary PATH --gost PATH [--help]

Builds the package twice from a real gost-mesh Agent executable and the pinned
GOST v3.2.6 Linux/amd64 runtime, verifies byte reproducibility and the runtime
contract, signs one exact manifest with a temporary Ed25519 key, and proves
tampered or unsigned inputs are rejected. The temporary private key is deleted
before exit and is never an output artifact.
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

expect_failure() {
  local description="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "${description} unexpectedly succeeded"
  fi
}

main() {
  local agent="" gost=""
  while (($#)); do
    case "$1" in
      --agent-binary)
        (($# >= 2)) || fail "--agent-binary requires a path"
        agent="$2"
        shift 2
        ;;
      --gost)
        (($# >= 2)) || fail "--gost requires a path"
        gost="$2"
        shift 2
        ;;
      -h|--help)
        usage
        return 0
        ;;
      *)
        usage >&2
        fail "unsupported argument: $1"
        ;;
    esac
  done

  [[ -n "${agent}" && -x "${agent}" && ! -L "${agent}" ]] || fail "a real executable --agent-binary is required"
  [[ -n "${gost}" && -x "${gost}" && ! -L "${gost}" ]] || fail "a real executable --gost runtime is required"

  require_command "${PYTHON_BIN}"
  require_command "${OPENSSL_BIN}"
  local command
  for command in awk cmp mktemp tail base64 tr grep find sha256sum; do
    require_command "${command}"
  done
  [[ -f "${BUILDER}" ]] || fail "package builder is missing"
  [[ -f "${VERIFIER}" ]] || fail "public-key verifier is missing"
  [[ -f "${SEMANTIC_RUNNER}" ]] || fail "semantic contract runner is missing"
  [[ -f "${SEMANTIC_CASES}" ]] || fail "semantic contract cases are missing"
  [[ -x "${SIGNING_SCRIPT}" ]] || fail "plugin signing script is missing or not executable"

  local agent_version gost_version gost_sha256
  agent_version="$("${agent}" --version 2>&1)" || fail "gost-mesh Agent version check failed"
  [[ "${agent_version}" =~ gost-mesh[[:space:]]+1\.0\.0 ]] || fail "Agent binary is not gost-mesh 1.0.0"
  gost_version="$("${gost}" -V 2>&1)" || fail "GOST version check failed"
  [[ "${gost_version}" =~ gost[[:space:]]+v${GOST_VERSION//./\.} ]] || fail "GOST runtime is not v${GOST_VERSION}"
  gost_sha256="$(sha256sum "${gost}" | awk '{print $1}')"
  [[ "${gost_sha256}" == "${GOST_AMD64_SHA256}" ]] || fail "GOST runtime SHA-256 does not match the pinned v3.2.6 binary"

  "${PYTHON_BIN}" "${SEMANTIC_RUNNER}" --agent "${agent}" --cases "${SEMANTIC_CASES}"

  local work first second private_key public_key signature signature_b64 raw_key
  work="$(mktemp -d)"
  WORK_DIR="${work}"
  trap cleanup EXIT
  first="${work}/first"
  second="${work}/second"
  private_key="${work}/signing-private.pem"
  public_key="${work}/signing-public.pem"
  signature="${work}/manifest.sig.bin"
  signature_b64="${work}/manifest.sig"
  raw_key="${work}/signing-public.b64"

  "${PYTHON_BIN}" "${BUILDER}" build \
    --agent-binary "${agent}" --gost "${gost}" \
    --goos linux --goarch amd64 --output-dir "${first}"
  "${PYTHON_BIN}" "${BUILDER}" build \
    --agent-binary "${agent}" --gost "${gost}" \
    --goos linux --goarch amd64 --output-dir "${second}"

  local name
  for name in gost-mesh-1.0.0.tar manifest.json build-report.json SHA256SUMS.txt; do
    cmp -s "${first}/${name}" "${second}/${name}" || fail "reproducible build mismatch: ${name}"
  done
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${first}"
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${second}"

  "${PYTHON_BIN}" - "${first}/manifest.json" "${first}/gost-mesh-1.0.0.tar" "${GOST_AMD64_SHA256}" <<'PY'
import json
import sys
import tarfile

manifest_path, artifact_path, gost_sha256 = sys.argv[1:]
expected_capabilities = [
    "forward.gost.mesh", "forward.tunnel.wss", "forward.tunnel.quic",
    "forward.tunnel.health", "forward.route.policy", "plugin.runtime-state", "plugin.cleanup",
]
expected_permissions = [
    "gost-mesh.view", "gost-mesh.api", "gost-mesh.network-admin", "gost-mesh.process-exec",
]
expected_defaults = {
    "api_version": "anixops.gost-mesh/v1", "apply": False,
    "rollback_on_exit": True, "tunnels": [],
}
expected_entrypoints = {
    "agent-linux-amd64": "agent/linux-amd64/plugin",
    "runtime-gost-linux-amd64": "runtime/linux-amd64/gost",
}
with open(manifest_path, "rb") as handle:
    manifest = json.load(handle)
with tarfile.open(artifact_path, mode="r:") as archive:
    defaults = json.load(archive.extractfile("config.defaults.json"))
    schema = json.load(archive.extractfile("config.schema.json"))
    package_index = json.load(archive.extractfile("package.json"))
    gost_member = archive.getmember("runtime/linux-amd64/gost")

if manifest.get("capabilities") != expected_capabilities:
    raise SystemExit("manifest capabilities do not match the executable runtime")
if manifest.get("permissions") != expected_permissions or manifest.get("webui", {}).get("permissions") != ["gost-mesh.view"]:
    raise SystemExit("manifest permissions are invalid")
if manifest.get("secret_fields") != []:
    raise SystemExit("unresolved secret fields are not allowed in v1")
if manifest.get("entrypoints") != expected_entrypoints:
    raise SystemExit("Agent and GOST runtime entrypoints are not version-bound")
if defaults != expected_defaults:
    raise SystemExit("runtime defaults do not match the Agent contract")
if schema.get("additionalProperties") is not False or schema.get("required") != list(expected_defaults):
    raise SystemExit("runtime schema is not strict")
definitions = schema.get("$defs", {})
if schema.get("properties", {}).get("tunnels", {}).get("maxItems") != 128 or schema.get("properties", {}).get("tunnels", {}).get("uniqueItems") is not True:
    raise SystemExit("runtime schema does not enforce the 128 unique tunnel limit")
if definitions.get("tun", {}).get("properties", {}).get("mtu", {}).get("maximum") != 9000:
    raise SystemExit("runtime schema MTU limit does not match the Agent")
if definitions.get("routing", {}).get("properties", {}).get("table", {}).get("maximum") != 252:
    raise SystemExit("runtime schema routing table limit does not match the Agent")
if definitions.get("routing", {}).get("required") != ["source_cidrs", "route_cidrs"]:
    raise SystemExit("runtime schema must leave exit table and priority optional")
role_condition = definitions.get("tunnel", {}).get("allOf", [{}])[0]
entry_routing = role_condition.get("then", {}).get("properties", {}).get("routing", {})
exit_routing = role_condition.get("else", {}).get("properties", {}).get("routing", {})
if entry_routing.get("required") != ["table", "priority"]:
    raise SystemExit("runtime schema must require entry table and priority")
if any(exit_routing.get("properties", {}).get(field, {}).get("const") != 0 for field in ("table", "priority")):
    raise SystemExit("runtime schema must restrict exit table and priority to omitted or 0")
if definitions.get("tun", {}).get("properties", {}).get("peer_address", {}).get("type") != "string":
    raise SystemExit("runtime schema tun.peer_address is not an IPv4 host field")
if definitions.get("remote", {}).get("required") != ["host", "port"]:
    raise SystemExit("runtime schema remote endpoint fields do not match the Agent")
if "tuic" in json.dumps(manifest).lower() or "tuic" in json.dumps(schema).lower():
    raise SystemExit("TUIC must not be advertised by the GOST v1 package")
if manifest.get("config_schema") != schema:
    raise SystemExit("manifest schema is not bound to the packaged schema")
runtime = package_index.get("runtime", {}).get("gost", {})
if runtime.get("version") != "3.2.6" or runtime.get("sha256") != gost_sha256:
    raise SystemExit("package index does not bind pinned GOST v3.2.6")
if gost_member.mode & 0o777 != 0o755:
    raise SystemExit("packaged GOST runtime is not executable")
PY

  "${OPENSSL_BIN}" genpkey -algorithm ED25519 -out "${private_key}" >/dev/null 2>&1
  chmod 0600 "${private_key}"
  "${OPENSSL_BIN}" pkey -in "${private_key}" -pubout -out "${public_key}" >/dev/null 2>&1
  "${SIGNING_SCRIPT}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/gost-mesh-1.0.0.tar" \
    --private-key "${private_key}" \
    --signature "${signature_b64}" \
    --public-key "${public_key}"
  cp "${signature_b64}" "${signature}"

  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" --artifact "${first}/gost-mesh-1.0.0.tar" \
    --signature "${signature}" --public-key "${public_key}" --openssl "${OPENSSL_BIN}"
  "${OPENSSL_BIN}" pkey -pubin -in "${public_key}" -outform DER 2>/dev/null \
    | tail -c 32 | base64 | tr -d '\n' >"${raw_key}"
  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" --artifact "${first}/gost-mesh-1.0.0.tar" \
    --signature "${signature}" --public-key "${raw_key}" --openssl "${OPENSSL_BIN}"

  cp "${first}/manifest.json" "${work}/manifest-tampered.json"
  printf ' ' >>"${work}/manifest-tampered.json"
  expect_failure "tampered manifest signature" "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${work}/manifest-tampered.json" --artifact "${first}/gost-mesh-1.0.0.tar" \
    --signature "${signature_b64}" --public-key "${public_key}" --openssl "${OPENSSL_BIN}"
  expect_failure "missing signature" "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" --artifact "${first}/gost-mesh-1.0.0.tar" \
    --signature "${work}/missing.sig" --public-key "${public_key}" --openssl "${OPENSSL_BIN}"
  cp "${signature}" "${work}/signature-tampered.sig"
  printf 'A' >>"${work}/signature-tampered.sig"
  expect_failure "tampered signature" "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" --artifact "${first}/gost-mesh-1.0.0.tar" \
    --signature "${work}/signature-tampered.sig" --public-key "${public_key}" --openssl "${OPENSSL_BIN}"
  cp "${first}/gost-mesh-1.0.0.tar" "${work}/artifact-tampered.tar"
  printf 'A' >>"${work}/artifact-tampered.tar"
  expect_failure "tampered artifact binding" "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" --artifact "${work}/artifact-tampered.tar" \
    --signature "${signature}" --public-key "${public_key}" --openssl "${OPENSSL_BIN}"

  if grep -RIlE --exclude='*.pyc' --exclude='*.tar' \
      -- '-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----' "${PACKAGE_ROOT}" | grep -q .; then
    fail "a private key appears in the package source tree"
  fi
  if find "${first}" "${second}" -type f \( -name '*.pem' -o -name '*.key' \) -print -quit | grep -q .; then
    fail "a private key was emitted into package outputs"
  fi

  printf 'status=PASS\n'
  printf 'plugin_id=gost-mesh\n'
  printf 'version=1.0.0\n'
  printf 'agent_input=real-binary\n'
  printf 'gost_version=%s\n' "${GOST_VERSION}"
  printf 'gost_sha256=%s\n' "${gost_sha256}"
  printf 'runtime_contract=true\n'
  printf 'semantic_contract=true\n'
  printf 'semantic_validation_network_started=false\n'
  printf 'unsigned_reproducible=true\n'
  printf 'public_key_formats=pem,raw-base64\n'
  printf 'tamper_rejection=true\n'
}

main "$@"
