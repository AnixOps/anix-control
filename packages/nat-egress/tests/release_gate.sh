#!/usr/bin/env bash
# Execute the official NAT Egress package release contract without
# persisting or uploading a signing private key.

set -Eeuo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
readonly PACKAGE_ROOT="${ROOT_DIR}/packages/nat-egress"
readonly BUILDER="${PACKAGE_ROOT}/build.py"
readonly VERIFIER="${PACKAGE_ROOT}/verify_signature.py"
readonly SIGNING_SCRIPT="${ROOT_DIR}/scripts/sign_plugin_release.sh"
PYTHON_BIN="${PYTHON_BIN:-python3}"
OPENSSL_BIN="${OPENSSL_BIN:-openssl}"

fail() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: packages/nat-egress/tests/release_gate.sh [--help]

Builds the package twice, verifies both unsigned outputs, signs one exact
manifest with a temporary Ed25519 key, verifies it with the public-key-only
verifier, and proves tampered/unsigned signatures are rejected. The temporary
private key is deleted before exit and is never an output artifact.
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
  case "${1:-}" in
    "") ;;
    -h|--help)
      usage
      return 0
      ;;
    *)
      usage >&2
      return 2
      ;;
  esac

  require_command "${PYTHON_BIN}"
  require_command "${OPENSSL_BIN}"
  local command
  for command in cmp mktemp tail base64 tr grep find; do
    require_command "${command}"
  done
  [[ -f "${BUILDER}" ]] || fail "package builder is missing"
  [[ -f "${VERIFIER}" ]] || fail "public-key verifier is missing"
  [[ -x "${SIGNING_SCRIPT}" ]] || fail "plugin signing script is missing or not executable"

  local work agent first second private_key public_key signature signature_b64 raw_key
  work="$(mktemp -d)"
  trap "rm -rf '${work}'" EXIT
  agent="${work}/nat-egress-agent"
  first="${work}/first"
  second="${work}/second"
  private_key="${work}/signing-private.pem"
  public_key="${work}/signing-public.pem"
  signature="${work}/manifest.sig.bin"
  signature_b64="${work}/manifest.sig"
  raw_key="${work}/signing-public.b64"

  printf '#!/bin/sh\nprintf "nat-egress-ci-fixture\\n"\n' >"${agent}"
  chmod 0755 "${agent}"

  "${PYTHON_BIN}" "${BUILDER}" build \
    --agent-binary "${agent}" \
    --goos linux \
    --goarch amd64 \
    --output-dir "${first}"
  "${PYTHON_BIN}" "${BUILDER}" build \
    --agent-binary "${agent}" \
    --goos linux \
    --goarch amd64 \
    --output-dir "${second}"

  for name in nat-egress-1.0.0.tar manifest.json build-report.json SHA256SUMS.txt; do
    cmp -s "${first}/${name}" "${second}/${name}" || fail "reproducible build mismatch: ${name}"
  done
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${first}"
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${second}"
  "${PYTHON_BIN}" - "${first}/manifest.json" "${first}/nat-egress-1.0.0.tar" <<'PY'
import json
import sys
import tarfile

manifest_path, artifact_path = sys.argv[1:]
expected_defaults = {
    "apply": False,
    "rollback_on_exit": True,
    "table_name": "anixops_nat_egress",
    "chain_name": "postrouting",
    "egress_interface": "eth0",
    "default_mark": 100,
    "policy_table": 100,
    "rule_priority": 10100,
    "ipv4_masquerade": True,
    "ipv6_masquerade": False,
    "health_check_enabled": True,
    "health_check_interval_seconds": 15,
    "health_check_timeout_seconds": 3,
    "health_check_target": "1.1.1.1:443",
}
expected_fields = list(expected_defaults)
with open(manifest_path, "rb") as handle:
    manifest = json.load(handle)
with tarfile.open(artifact_path, mode="r:") as archive:
    defaults_handle = archive.extractfile("config.defaults.json")
    schema_handle = archive.extractfile("config.schema.json")
    if defaults_handle is None or schema_handle is None:
        raise SystemExit("runtime configuration is missing from the package")
    defaults = json.load(defaults_handle)
    schema = json.load(schema_handle)

if defaults != expected_defaults:
    raise SystemExit("runtime defaults do not match the Agent contract")
if schema.get("additionalProperties") is not False:
    raise SystemExit("runtime schema permits unknown fields")
if schema.get("required") != expected_fields or set(schema.get("properties", {})) != set(expected_fields):
    raise SystemExit("runtime schema does not require the exact Agent field set")
if manifest.get("config_schema") != schema:
    raise SystemExit("manifest schema is not bound to the packaged schema")
if manifest.get("permissions") != ["nat-egress.view", "nat-egress.api", "nat-egress.network-admin"]:
    raise SystemExit("manifest network-admin permission is missing")
if manifest.get("capabilities") != [
    "forward.nat.egress",
    "forward.route.policy",
    "forward.egress.health",
    "forward.mark.consume",
    "plugin.runtime-state",
    "plugin.cleanup",
]:
    raise SystemExit("manifest lifecycle or mark-consumer capabilities are missing")
if manifest.get("webui", {}).get("permissions") != ["nat-egress.view"]:
    raise SystemExit("WebUI permissions are not view-only")
PY

  "${OPENSSL_BIN}" genpkey -algorithm ED25519 -out "${private_key}" >/dev/null 2>&1
  chmod 0600 "${private_key}"
  "${OPENSSL_BIN}" pkey -in "${private_key}" -pubout -out "${public_key}" >/dev/null 2>&1
  "${SIGNING_SCRIPT}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --private-key "${private_key}" \
    --signature "${signature_b64}" \
    --public-key "${public_key}"
  cp "${signature_b64}" "${signature}"

  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --signature "${signature}" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  "${OPENSSL_BIN}" pkey -pubin -in "${public_key}" -outform DER 2>/dev/null \
    | tail -c 32 | base64 | tr -d '\n' >"${raw_key}"
  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --signature "${signature}" \
    --public-key "${raw_key}" \
    --openssl "${OPENSSL_BIN}"

  cp "${first}/manifest.json" "${work}/manifest-tampered.json"
  printf ' ' >>"${work}/manifest-tampered.json"
  expect_failure "tampered manifest signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${work}/manifest-tampered.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --signature "${signature_b64}" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  expect_failure "missing signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --signature "${work}/missing.sig" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  cp "${signature}" "${work}/signature-tampered.sig"
  printf 'A' >>"${work}/signature-tampered.sig"
  expect_failure "tampered signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/nat-egress-1.0.0.tar" \
    --signature "${work}/signature-tampered.sig" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  cp "${first}/nat-egress-1.0.0.tar" "${work}/artifact-tampered.tar"
  printf 'A' >>"${work}/artifact-tampered.tar"
  expect_failure "tampered artifact binding" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${work}/artifact-tampered.tar" \
    --signature "${signature}" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"

  if grep -RIlE --exclude='*.pyc' --exclude='*.tar' \
      -- '-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----' "${PACKAGE_ROOT}" | grep -q .; then
    fail "a private key appears in the package source tree"
  fi
  if find "${first}" "${second}" -type f \( -name '*.pem' -o -name '*.key' \) -print -quit | grep -q .; then
    fail "a private key was emitted into package outputs"
  fi

  {
    printf 'status=PASS\n'
    printf 'plugin_id=nat-egress\n'
    printf 'version=1.0.0\n'
    printf 'agent_input=temporary-ci-fixture\n'
    printf 'runtime_contract=true\n'
    printf 'unsigned_reproducible=true\n'
    printf 'public_key_formats=pem,raw-base64\n'
    printf 'tamper_rejection=true\n'
  }
}

main "$@"
