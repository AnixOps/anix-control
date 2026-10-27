#!/usr/bin/env bash
# Fake-root tests of the Control installer's Agent release sync
# (scripts/install.sh): agent_install.artifact_dir is set when empty and the
# Agent release Control names is downloaded into it, checked against the
# official key. Nothing outside a temporary directory is touched.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/install.sh
source "${REPO_ROOT}/scripts/install.sh"
set +E
trap - ERR

temporary="$(mktemp -d)"
cleanup() { find "${temporary}" -depth -delete; }
trap cleanup EXIT

chown() { :; }
install() {
  local arguments=()
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      -o|-g) shift 2 ;;
      *) arguments+=("$1"); shift ;;
    esac
  done
  command install "${arguments[@]}"
}

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }
check() {
  local name="$1"
  shift
  if "$@"; then pass "${name}"; else fail "${name}"; fi
}

APP_USER="$(id -un)"
TAG="v5.0.0-rc.8"
AGENT_RELEASE_BASE="https://example.invalid/releases"

# A test release key stands in for the official one.
openssl genpkey -algorithm ed25519 -out "${temporary}/release.key" 2>/dev/null
OFFICIAL_PUBLIC_KEY="$(openssl pkey -in "${temporary}/release.key" -pubout -outform DER 2>/dev/null | tail -c 32 | base64 -w0)"

# make_release <dir> writes an Agent release signed by the test key.
make_release() {
  local dir="$1" zip digest
  mkdir -p "${dir}"
  : >"${dir}/SHA256SUMS"
  for zip in anix-agent-linux-64.zip anix-agent-linux-arm64-v8a.zip; do
    printf 'agent %s\n' "${zip}" >"${dir}/${zip}"
    digest="$(sha256sum "${dir}/${zip}" | awk '{print $1}')"
    printf 'SHA2-256(%s)= %s\n' "${zip}" "${digest}" >"${dir}/${zip}.dgst"
    printf 'c2ln\n' >"${dir}/${zip}.sig"
    printf '%s  %s\n' "${digest}" "${zip}" >>"${dir}/SHA256SUMS"
  done
  openssl pkeyutl -sign -inkey "${temporary}/release.key" -rawin -in "${dir}/SHA256SUMS" | base64 -w0 >"${dir}/SHA256SUMS.sig"
}
RELEASE="${temporary}/release"
make_release "${RELEASE}"

# curl serves Control's agent.env and the release from RELEASE; it logs the
# release downloads.
CURL_LOG="${temporary}/curl.log"
curl() {
  local out="" url="" argument
  while [[ "$#" -gt 0 ]]; do
    argument="$1"
    case "${argument}" in
      -o) out="$2"; shift 2 ;;
      --retry|--retry-delay|--connect-timeout|--max-time) shift 2 ;;
      -*) shift ;;
      *) url="${argument}"; shift ;;
    esac
  done
  case "${url}" in
    */install/agent.env) printf 'format 1\nagent_version %s\n' "${CONTROL_AGENT:-${TAG}}" ;;
    "${AGENT_RELEASE_BASE}/${TAG}/"*)
      printf '%s\n' "${url}" >>"${CURL_LOG}"
      [[ -f "${RELEASE}/${url##*/}" ]] || return 22
      cp "${RELEASE}/${url##*/}" "${out}"
      ;;
    *) return 22 ;;
  esac
}

new_install() {
  INSTALL_DIR="${temporary}/$1"
  set_install_paths
  mkdir -p "${INSTALL_DIR}/config"
  cp "${REPO_ROOT}/config/config.yaml.example" "${CONFIG_FILE}"
  : >"${CURL_LOG}"
}

# 1. An empty artifact_dir is set and the release is downloaded into it.
new_install fresh
configure_agent_artifact_dir >/dev/null
check "artifact_dir is set to <data_dir>/agent-releases" \
  test "$(config_section_value agent_install artifact_dir)" = "${DATA_DIR}/agent-releases"
sync_agent_release >"${temporary}/out" 2>&1
check "the release is in <dir>/<tag>" test -s "${DATA_DIR}/agent-releases/${TAG}/anix-agent-linux-64.zip.dgst"
check "all eight files are there" test "$(find "${DATA_DIR}/agent-releases/${TAG}" -type f | wc -l)" = 8
check "no stage directory is left" test -z "$(find "${DATA_DIR}/agent-releases" -mindepth 1 -maxdepth 1 -name '.*')"

# 2. A second run downloads nothing.
: >"${CURL_LOG}"
sync_agent_release >"${temporary}/out" 2>&1
check "a present release is not downloaded again" test ! -s "${CURL_LOG}"
check "and says so" grep -qF "Agent ${TAG} is already in" "${temporary}/out"

# 3. A configured artifact_dir is kept.
new_install custom
set_config_section_values agent_install 'artifact_dir="/srv/agent"'
configure_agent_artifact_dir >/dev/null
check "a configured artifact_dir is kept" test "${AGENT_ARTIFACT_DIR}" = "/srv/agent"

# 4. A bad signature or a zip that does not match installs nothing.
new_install badsig
configure_agent_artifact_dir >/dev/null
printf 'bm90IGEgc2lnbmF0dXJl\n' >"${RELEASE}/SHA256SUMS.sig"
sync_agent_release >"${temporary}/out" 2>&1
check "a bad signature is a warning" grep -qF "does not verify against the official key" "${temporary}/out"
check "a bad signature installs nothing" test ! -e "${DATA_DIR}/agent-releases/${TAG}"
make_release "${RELEASE}"
printf 'tampered\n' >"${RELEASE}/anix-agent-linux-64.zip"
sync_agent_release >"${temporary}/out" 2>&1
check "a tampered zip is a warning" grep -qF "does not match SHA256SUMS" "${temporary}/out"
check "a tampered zip installs nothing" test ! -e "${DATA_DIR}/agent-releases/${TAG}"
make_release "${RELEASE}"

# 5. A download failure or no Agent release from Control is a warning.
rm "${RELEASE}/SHA256SUMS.sig"
sync_agent_release >"${temporary}/out" 2>&1
check "a failed download is a warning" grep -qF "nodes download it from GitHub" "${temporary}/out"
make_release "${RELEASE}"
CONTROL_AGENT="nonsense" sync_agent_release >"${temporary}/out" 2>&1
check "no Agent release from Control is a warning" grep -qF "names no Agent release" "${temporary}/out"

if ((failures > 0)); then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'agent release sync: all checks passed\n'
