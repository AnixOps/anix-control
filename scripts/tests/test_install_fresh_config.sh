#!/usr/bin/env bash
# The configuration a fresh systemd install writes is a production
# configuration, and an existing one is never rewritten.
#
# config/config.yaml.example is the template the installer downloads for the
# tag, and it ships `env: "development"` for local use. With that value
# Control runs a full AutoMigrate on every start and skips the production
# guards (the template JWT secret is accepted). The installer therefore sets
# `env: "production"` itself, next to the secrets it generates. Nothing outside
# a temporary directory is touched and no service is started.
#
# internal/config/installer_config_test.go runs this script with
# ANIX_INSTALL_FRESH_CONFIG_OUT=<file>, loads the generated file with the real
# configuration loader and applies the server's production checks to it.

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
  ADMIN_EMAIL="" ADMIN_PASSWORD="" FRESH_CONFIG=0
}

# --- 1. A fresh install writes a production configuration.
new_install fresh
write_fresh_config >/dev/null 2>&1
fresh_config="${temporary}/fresh-config.yaml"
cp "${CONFIG_FILE}" "${fresh_config}"
check "fresh: template fetched at the installed tag" \
  grep -qxF "${RAW_BASE}/${VERSION}/config/config.yaml.example" "${CURL_URLS}"
check "fresh: env is production" test "$(top_level env "${CONFIG_FILE}")" = production
check "fresh: one env key" test "$(grep -c '^env:' "${CONFIG_FILE}")" -eq 1
check "fresh: no development env left" test "$(grep -c '^env:[[:space:]]*"\?development' "${CONFIG_FILE}")" -eq 0
check "fresh: marked as a fresh config" test "${FRESH_CONFIG}" -eq 1
jwt_secret="$(config_section_value jwt secret)"
check "fresh: jwt secret is 64 hex characters" \
  test "$(printf '%s' "${jwt_secret}" | grep -Ec '^[0-9a-f]{64}$')" -eq 1
check "fresh: jwt secret is not a template value" \
  test "${jwt_secret}" != "your-jwt-secret-key-change-in-production" -a \
       "${jwt_secret}" != "CHANGE-THIS-TO-A-VERY-LONG-RANDOM-STRING-IN-PRODUCTION"
check "fresh: api token generated" \
  test "$(config_section_value app api_token | grep -Ec '^[0-9a-f]{64}$')" -eq 1
check "fresh: bootstrap password file written" test -s "${INSTALL_DIR}/.bootstrap-admin-password"
check "fresh: admin password generated into the config" \
  test "$(config_section_value admin password)" = "$(cat "${INSTALL_DIR}/.bootstrap-admin-password")"
# Only the env line and the four generated values differ from the template.
check "fresh: five lines changed from the template" \
  test "$(diff "${TEMPLATE}" "${CONFIG_FILE}" | grep -c '^>')" -eq 5
check "fresh: no line added or lost" \
  test "$(wc -l <"${TEMPLATE}")" -eq "$(wc -l <"${CONFIG_FILE}")"

# --- 2. A template without an env key still ends up in production.
new_install noenv
TEMPLATE="${temporary}/template-without-env.yaml"
grep -v '^env:' "${REPO_ROOT}/config/config.yaml.example" >"${TEMPLATE}"
check "noenv: the fixture has no env key" test "$(grep -c '^env:' "${TEMPLATE}")" -eq 0
write_fresh_config >/dev/null 2>&1
check "noenv: env is production" test "$(top_level env "${CONFIG_FILE}")" = production
check "noenv: one env key" test "$(grep -c '^env:' "${CONFIG_FILE}")" -eq 1
TEMPLATE="${REPO_ROOT}/config/config.yaml.example"

# --- 3. A template that spells the key differently is still replaced.
new_install spelling
TEMPLATE="${temporary}/template-spelling.yaml"
sed 's/^env: "development"$/env: development   # local use/' "${REPO_ROOT}/config/config.yaml.example" >"${TEMPLATE}"
check "spelling: the fixture spells the key differently" grep -qxF 'env: development   # local use' "${TEMPLATE}"
write_fresh_config >/dev/null 2>&1
check "spelling: env is production" test "$(top_level env "${CONFIG_FILE}")" = production
check "spelling: one env key" test "$(grep -c '^env:' "${CONFIG_FILE}")" -eq 1
TEMPLATE="${REPO_ROOT}/config/config.yaml.example"

# --- 4. ensure_layout_and_config (install and update) creates the file only
#        when it is missing and never rewrites an existing one.
new_install ensure-fresh
ensure_layout_and_config >"${temporary}/ensure-fresh.log" 2>&1
check "ensure: fresh install writes the config" test -s "${CONFIG_FILE}"
check "ensure: fresh install is production" test "$(top_level env "${CONFIG_FILE}")" = production
check "ensure: fresh install is marked fresh" test "${FRESH_CONFIG}" -eq 1
new_install ensure-existing
printf '%s\n' '# the operator edited this file' 'env: "development"' 'jwt:' '  secret: "operator-chosen-secret"' >"${CONFIG_FILE}"
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
