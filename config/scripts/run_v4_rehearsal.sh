#!/usr/bin/env bash
# Build or verify the formal v4 package set, then write complete release evidence.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TAG="v4.0.0"
PACKAGES_DIR="${REPO_ROOT}/artifacts/v4-packages"
OUTPUT="${REPO_ROOT}/artifacts/v4-rehearsal-evidence.json"
SIGNING_KEY=""
OFFICIAL_PUBLIC_KEY=""
POSTGRES_DSN=""
CANARY_EVIDENCE=""
SUPPORT_EVIDENCE=""
AGENT_BINARY_ARGS=()
RUNTIME_BINARY_ARGS=()

usage() {
  cat <<'EOF'
Usage: config/scripts/run_v4_rehearsal.sh [options]

Runs the formal v4 package-only rehearsal. PostgreSQL, canary, and support
evidence are mandatory and are never replaced with synthetic passing records.

Options:
  --tag TAG                    Release tag (must be v4.0.0)
  --packages-dir PATH          Existing formal package directory
  --signing-key PATH           Official Ed25519 signing key (required; signs evidence)
  --official-public-key PATH   Base64 raw Ed25519 official root file (required)
  --postgres-dsn DSN           Disposable PostgreSQL test DSN (required)
  --canary-evidence PATH       Approved canary evidence file (required)
  --support-evidence PATH      Approved support evidence file (required)
  --agent-binary SPEC          Real Agent binary PACKAGE_ID@GOOS/GOARCH=PATH; repeat for every Agent package/platform
  --runtime-binary SPEC        Pinned runtime binary PACKAGE_ID:RUNTIME@GOOS/GOARCH=PATH; repeat for every runtime/platform
  --output PATH                Evidence JSON output path
  --self-test                  Validate script dependencies without a rehearsal
  --help                       Show this help
EOF
}

fail() {
  printf 'v4 rehearsal: %s\n' "$*" >&2
  exit 1
}

while (($#)); do
  case "$1" in
    --tag) TAG="${2:-}"; shift 2 ;;
    --packages-dir) PACKAGES_DIR="${2:-}"; shift 2 ;;
    --signing-key) SIGNING_KEY="${2:-}"; shift 2 ;;
    --official-public-key) OFFICIAL_PUBLIC_KEY="${2:-}"; shift 2 ;;
    --postgres-dsn) POSTGRES_DSN="${2:-}"; shift 2 ;;
    --canary-evidence) CANARY_EVIDENCE="${2:-}"; shift 2 ;;
    --support-evidence) SUPPORT_EVIDENCE="${2:-}"; shift 2 ;;
    --agent-binary) AGENT_BINARY_ARGS+=(--agent-binary "${2:-}"); shift 2 ;;
    --runtime-binary) RUNTIME_BINARY_ARGS+=(--runtime-binary "${2:-}"); shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --self-test)
      python3 "${REPO_ROOT}/config/scripts/render_v4_release_evidence.py" --self-test
      python3 "${REPO_ROOT}/config/scripts/verify_v4_evidence.py" --self-test
      exit 0
      ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "${TAG}" == "v4.0.0" ]] || fail "tag must be v4.0.0"
[[ -n "${OFFICIAL_PUBLIC_KEY}" ]] || fail "--official-public-key is required"
[[ -f "${OFFICIAL_PUBLIC_KEY}" && ! -L "${OFFICIAL_PUBLIC_KEY}" ]] || fail "official public key must be a regular file"
[[ -n "${SIGNING_KEY}" ]] || fail "--signing-key is required"
[[ -f "${SIGNING_KEY}" && ! -L "${SIGNING_KEY}" ]] || fail "signing key must be a regular file"
[[ -n "${POSTGRES_DSN}" ]] || fail "--postgres-dsn is required"
[[ -n "${CANARY_EVIDENCE}" && -f "${CANARY_EVIDENCE}" && ! -L "${CANARY_EVIDENCE}" ]] || fail "--canary-evidence must be a regular file"
[[ -n "${SUPPORT_EVIDENCE}" && -f "${SUPPORT_EVIDENCE}" && ! -L "${SUPPORT_EVIDENCE}" ]] || fail "--support-evidence must be a regular file"
(( ${#AGENT_BINARY_ARGS[@]} > 0 )) || fail "--agent-binary is required for the formal Agent package set"
(( ${#RUNTIME_BINARY_ARGS[@]} > 0 )) || fail "--runtime-binary is required for the formal runtime package set"

mkdir -p "${PACKAGES_DIR}" "$(dirname "${OUTPUT}")"
python3 "${REPO_ROOT}/packages/shared/build_package.py" \
  --all \
  --version "${TAG#v}" \
  --out "${PACKAGES_DIR}" \
  --formal-release \
  --signing-key "${SIGNING_KEY}" \
  --official-public-key "${OFFICIAL_PUBLIC_KEY}" \
  --platform linux/amd64 \
  --platform linux/arm64 \
  "${AGENT_BINARY_ARGS[@]}" \
  "${RUNTIME_BINARY_ARGS[@]}"

ANIX_TEST_POSTGRES_DSN="${POSTGRES_DSN}" \
python3 "${REPO_ROOT}/config/scripts/render_v4_release_evidence.py" \
  --tag "${TAG}" \
  --packages-dir "${PACKAGES_DIR}" \
  --official-public-key "${OFFICIAL_PUBLIC_KEY}" \
  --signing-key "${SIGNING_KEY}" \
  --canary-evidence "${CANARY_EVIDENCE}" \
  --support-evidence "${SUPPORT_EVIDENCE}" \
  --output "${OUTPUT}" \
  --postgres-dsn "${POSTGRES_DSN}" \
  --repo-root "${REPO_ROOT}"

python3 "${REPO_ROOT}/config/scripts/verify_v4_evidence.py" \
  --input "${OUTPUT}" \
  --trusted-official-public-key "${OFFICIAL_PUBLIC_KEY}"
printf 'v4 rehearsal evidence verified: %s\n' "${OUTPUT}"
