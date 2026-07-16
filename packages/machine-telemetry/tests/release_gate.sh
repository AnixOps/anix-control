#!/usr/bin/env bash
# Execute the official Machine Telemetry package release contract without
# persisting or uploading a signing private key.

set -Eeuo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
readonly PACKAGE_ROOT="${ROOT_DIR}/packages/machine-telemetry"
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
Usage: packages/machine-telemetry/tests/release_gate.sh [--help]

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
  agent="${work}/machine-telemetry-agent"
  first="${work}/first"
  second="${work}/second"
  private_key="${work}/signing-private.pem"
  public_key="${work}/signing-public.pem"
  signature="${work}/manifest.sig.bin"
  signature_b64="${work}/manifest.sig"
  raw_key="${work}/signing-public.b64"

  printf '#!/bin/sh\nprintf "machine-telemetry-ci-fixture\\n"\n' >"${agent}"
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

  for name in machine-telemetry-1.0.0.tar manifest.json build-report.json SHA256SUMS.txt; do
    cmp -s "${first}/${name}" "${second}/${name}" || fail "reproducible build mismatch: ${name}"
  done
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${first}"
  "${PYTHON_BIN}" "${BUILDER}" verify --output-dir "${second}"

  "${OPENSSL_BIN}" genpkey -algorithm ED25519 -out "${private_key}" >/dev/null 2>&1
  chmod 0600 "${private_key}"
  "${OPENSSL_BIN}" pkey -in "${private_key}" -pubout -out "${public_key}" >/dev/null 2>&1
  "${SIGNING_SCRIPT}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --private-key "${private_key}" \
    --signature "${signature_b64}" \
    --public-key "${public_key}"
  cp "${signature_b64}" "${signature}"

  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --signature "${signature}" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  "${OPENSSL_BIN}" pkey -pubin -in "${public_key}" -outform DER 2>/dev/null \
    | tail -c 32 | base64 | tr -d '\n' >"${raw_key}"
  "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --signature "${signature}" \
    --public-key "${raw_key}" \
    --openssl "${OPENSSL_BIN}"

  cp "${first}/manifest.json" "${work}/manifest-tampered.json"
  printf ' ' >>"${work}/manifest-tampered.json"
  expect_failure "tampered manifest signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${work}/manifest-tampered.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --signature "${signature_b64}" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  expect_failure "missing signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --signature "${work}/missing.sig" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  cp "${signature}" "${work}/signature-tampered.sig"
  printf 'A' >>"${work}/signature-tampered.sig"
  expect_failure "tampered signature" \
    "${PYTHON_BIN}" "${VERIFIER}" \
    --manifest "${first}/manifest.json" \
    --artifact "${first}/machine-telemetry-1.0.0.tar" \
    --signature "${work}/signature-tampered.sig" \
    --public-key "${public_key}" \
    --openssl "${OPENSSL_BIN}"
  cp "${first}/machine-telemetry-1.0.0.tar" "${work}/artifact-tampered.tar"
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
    printf 'plugin_id=machine-telemetry\n'
    printf 'version=1.0.0\n'
    printf 'agent_input=temporary-ci-fixture\n'
    printf 'unsigned_reproducible=true\n'
    printf 'public_key_formats=pem,raw-base64\n'
    printf 'tamper_rejection=true\n'
  }
}

main "$@"
