#!/usr/bin/env bash
# The configuration a fresh systemd install writes, and an existing one is
# never rewritten.
#
# config/config.yaml.example is the template the installer downloads for the
# tag: the operator keys' one line, domain: panel.example.com, with comments.
# The installer does not ask for the domain yet, so it keeps that line a
# comment, and writes nothing else: no secret (the root key in the data
# directory derives the JWT secret) and no administrator (the first one
# claims the panel with the link the installer prints at the end, see
# test_install_fresh_main.sh). Nothing outside a temporary directory is
# touched and no service is started.
#
# internal/config/installer_config_test.go runs this script with
# ANIX_INSTALL_FRESH_CONFIG_OUT=<file>, loads the generated file with the real
# configuration loader and applies the server's checks to it.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/install.sh
source "${REPO_ROOT}/scripts/install.sh"
set +E
trap - ERR

temporary="$(mktemp -d)"
cleanup() { find "${temporary}" -depth -delete; }
trap cleanup EXIT

# The installer runs as root; keep the test unprivileged.
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

# curl serves the configuration template of the requested tag from this
# checkout (TEMPLATE) and records the URL it was asked for.
TEMPLATE="${REPO_ROOT}/config/config.yaml.example"
CURL_URLS="${temporary}/curl-urls"
: >"${CURL_URLS}"
curl() {
  local out="" url=""
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      -o) out="$2"; shift 2 ;;
      --retry|--connect-timeout) shift 2 ;;
      -*) shift ;;
      *) url="$1"; shift ;;
    esac
  done
  printf '%s\n' "${url}" >>"${CURL_URLS}"
  cp "${TEMPLATE}" "${out}"
}

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }
check() {
  local name="$1"
  shift
  if "$@"; then pass "${name}"; else fail "${name}"; fi
}

# top_level <key> <file> prints a top-level scalar, unquoted.
top_level() {
  awk -v key="$1" '
    index($0, key ":") == 1 {
      value = substr($0, length(key) + 2)
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+#.*$/, "", value)
      gsub(/"/, "", value)
      print value
      exit
    }' "$2"
}

APP_USER="$(id -un)"
VERSION="v4.2.0-rc.4"
COMMAND="install"

# new_install <name> prepares an installation root and a scratch directory.
new_install() {
  INSTALL_DIR="${temporary}/$1"
  set_install_paths
  TMP_DIR="${temporary}/$1-tmp"
  mkdir -p "${INSTALL_DIR}/config" "${TMP_DIR}"
  FRESH_CONFIG=0
}

# --- 1. A fresh install writes the template, the domain line kept as a
#        comment, and nothing else: no secret, no administrator.
new_install fresh
write_fresh_config >/dev/null 2>&1
fresh_config="${temporary}/fresh-config.yaml"
cp "${CONFIG_FILE}" "${fresh_config}"
check "fresh: template fetched at the installed tag" \
  grep -qxF "${RAW_BASE}/${VERSION}/config/config.yaml.example" "${CURL_URLS}"
check "fresh: the domain line stays a comment" grep -qxF '# domain: panel.example.com' "${CONFIG_FILE}"
check "fresh: no active domain line" test "$(grep -c '^domain:' "${CONFIG_FILE}")" -eq 0
check "fresh: no env key (Control always runs as production)" test "$(grep -c '^env:' "${CONFIG_FILE}")" -eq 0
check "fresh: no jwt secret (the root key derives it)" test "$(grep -c '^jwt:' "${CONFIG_FILE}")" -eq 0
check "fresh: no api token" test "$(grep -c 'api_token' "${CONFIG_FILE}")" -eq 0
check "fresh: no administrator (the claim link creates the first one)" test "$(grep -c '^admin:' "${CONFIG_FILE}")" -eq 0
check "fresh: no password anywhere in the file" test "$(grep -ci 'password' "${CONFIG_FILE}")" -eq 0
check "fresh: no bootstrap password file" test ! -e "${INSTALL_DIR}/.bootstrap-admin-password"
check "fresh: marked as a fresh config" test "${FRESH_CONFIG}" -eq 1
# Only the domain line changed from the template, and nothing is appended.
check "fresh: the template is kept, the domain line commented" \
  test "$(diff "${TEMPLATE}" "${CONFIG_FILE}" | grep -c '^<')" -eq 1
check "fresh: nothing appended" test "$(wc -l <"${CONFIG_FILE}")" -eq "$(wc -l <"${TEMPLATE}")"

# --- 2. ensure_layout_and_config (install and update) creates the file only
#        when it is missing and never rewrites an existing one.
new_install ensure-fresh
ensure_layout_and_config >"${temporary}/ensure-fresh.log" 2>&1
check "ensure: fresh install writes the config" test -s "${CONFIG_FILE}"
check "ensure: fresh install has no administrator" test "$(grep -c '^admin:' "${CONFIG_FILE}")" -eq 0
check "ensure: fresh install is marked fresh" test "${FRESH_CONFIG}" -eq 1
new_install ensure-existing
printf '%s\n' '# the operator edited this file' 'env: "development"' 'jwt:' '  secret: "operator-chosen-secret"' 'admin:' '  email: "owner@example.com"' >"${CONFIG_FILE}"
before="$(sha256sum "${CONFIG_FILE}")"
downloads_before="$(grep -c . "${CURL_URLS}")"
ensure_layout_and_config >"${temporary}/ensure-existing.log" 2>&1
check "ensure: existing config byte for byte unchanged" test "$(sha256sum "${CONFIG_FILE}")" = "${before}"
check "ensure: existing config keeps its env" test "$(top_level env "${CONFIG_FILE}")" = development
check "ensure: existing config reported as preserved" grep -qF "Existing configuration found; preserving" "${temporary}/ensure-existing.log"
check "ensure: existing config is not marked fresh" test "${FRESH_CONFIG}" -eq 0
check "ensure: nothing was downloaded for it" test "$(grep -c . "${CURL_URLS}")" -eq "${downloads_before}"

# The Go test loads this file with the real configuration loader.
if [[ -n "${ANIX_INSTALL_FRESH_CONFIG_OUT:-}" ]]; then
  cp "${fresh_config}" "${ANIX_INSTALL_FRESH_CONFIG_OUT}"
fi

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'install fresh config: all checks passed\n'
