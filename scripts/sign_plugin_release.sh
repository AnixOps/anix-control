#!/usr/bin/env bash
# Sign one canonical official plugin manifest without ever writing a private
# key into the package or release directory.

set -Eeuo pipefail

OPENSSL_BIN="${OPENSSL_BIN:-openssl}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MANIFEST=""
ARTIFACT=""
PRIVATE_KEY=""
SIGNATURE=""
PUBLIC_KEY=""

usage() {
  cat <<'EOF'
Usage: scripts/sign_plugin_release.sh \
  --manifest PATH --artifact PATH --private-key PATH \
  --signature PATH --public-key PATH

The private key is read only from the supplied temporary file. The output
signature is Base64 encoded and the public key is emitted as PEM. The exact
manifest bytes and artifact digest are verified before the script succeeds.
EOF
}

die() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

require_file() {
  local path="$1"
  [[ -f "${path}" && ! -L "${path}" ]] || die "regular file required: ${path}"
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --manifest) MANIFEST="${2:-}"; shift 2 ;;
      --artifact) ARTIFACT="${2:-}"; shift 2 ;;
      --private-key) PRIVATE_KEY="${2:-}"; shift 2 ;;
      --signature) SIGNATURE="${2:-}"; shift 2 ;;
      --public-key) PUBLIC_KEY="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
  done
  [[ -n "${MANIFEST}" && -n "${ARTIFACT}" && -n "${PRIVATE_KEY}" && -n "${SIGNATURE}" && -n "${PUBLIC_KEY}" ]] || {
    usage >&2
    exit 2
  }
}

validate_artifact_binding() {
  "${PYTHON_BIN}" - "${MANIFEST}" "${ARTIFACT}" <<'PY'
import hashlib
import json
import pathlib
import sys

manifest_path = pathlib.Path(sys.argv[1])
artifact_path = pathlib.Path(sys.argv[2])
manifest = json.loads(manifest_path.read_bytes())
expected = manifest.get("artifact_sha256")
if not isinstance(expected, str) or len(expected) != 64 or any(char not in "0123456789abcdef" for char in expected):
    raise SystemExit("manifest artifact_sha256 must be a lowercase SHA-256 digest")
actual = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
if actual != expected:
    raise SystemExit(f"artifact digest mismatch: manifest={expected} actual={actual}")
PY
}

main() {
  parse_args "$@"
  command -v "${OPENSSL_BIN}" >/dev/null 2>&1 || die "OpenSSL executable not found: ${OPENSSL_BIN}"
  command -v "${PYTHON_BIN}" >/dev/null 2>&1 || die "Python executable not found: ${PYTHON_BIN}"
  require_file "${MANIFEST}"
  require_file "${ARTIFACT}"
  require_file "${PRIVATE_KEY}"
  [[ "$(stat -c '%a' "${PRIVATE_KEY}" 2>/dev/null || printf '600')" == "600" ]] || die "private key must have mode 0600"
  validate_artifact_binding

  local work
  work="$(mktemp -d)"
  trap "rm -rf '${work}'" EXIT
  local raw_signature="${work}/signature.bin"
  local derived_public="${work}/public.pem"
  mkdir -p "$(dirname "${SIGNATURE}")" "$(dirname "${PUBLIC_KEY}")"
  umask 077

  "${OPENSSL_BIN}" pkey -in "${PRIVATE_KEY}" -pubout -out "${derived_public}" >/dev/null 2>&1 || die "failed to derive the signing public key"
  "${OPENSSL_BIN}" pkeyutl -sign -inkey "${PRIVATE_KEY}" -rawin \
    -in "${MANIFEST}" -out "${raw_signature}" >/dev/null 2>&1 || die "failed to sign the manifest"
  base64 -w0 "${raw_signature}" >"${SIGNATURE}"
  printf '\n' >>"${SIGNATURE}"
  cp "${derived_public}" "${PUBLIC_KEY}"
  chmod 0644 "${SIGNATURE}" "${PUBLIC_KEY}"

  "${PYTHON_BIN}" "${ROOT_DIR}/packages/machine-telemetry/verify_signature.py" \
    --manifest "${MANIFEST}" --artifact "${ARTIFACT}" \
    --signature "${SIGNATURE}" --public-key "${PUBLIC_KEY}" \
    --openssl "${OPENSSL_BIN}" >/dev/null
  printf '[PASS] signed plugin manifest: %s\n' "${MANIFEST}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
