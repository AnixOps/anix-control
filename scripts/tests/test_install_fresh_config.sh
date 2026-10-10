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
# configuration loader and applies the server's production checks to it. With
# ANIX_INSTALL_FRESH_CONFIG_CASES_DIR=<dir> it also writes the config of each
# awkward-credential case (<name>.yaml, .email, .password) for the same test.

# shellcheck disable=SC1003,SC2016
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

# --- 5. Without --admin-email the bootstrap administrator is an address the
#        server's `required,email` login check accepts. admin@localhost (the
#        earlier default) has no dot in its domain and is refused, so the
#        install's own login check failed and left a running service behind.
new_install default-admin
write_fresh_config >/dev/null 2>&1
default_email="$(config_section_value admin email)"
check "default admin: the email is admin@anixops.local, the server's own default" \
  test "${default_email}" = "admin@anixops.local"
check "default admin: the domain has a dot (a bare host name is refused)" \
  grep -Eq '^[^@[:space:]]+@[^@[:space:].]+(\.[^@[:space:].]+)+$' <<<"${default_email}"

# --- 6. Credentials with characters that awk, the shell or YAML treat
#        specially are written verbatim. The value goes into a YAML
#        double-quoted scalar, so only the backslash and the double quote are
#        escaped, once. (awk -v interprets escapes: ab\qcd was escaped to
#        ab\\qcd and then unescaped again to "ab\qcd", which does not load.)
#        internal/config/installer_config_test.go loads every file written here
#        with the real loader when ANIX_INSTALL_FRESH_CONFIG_CASES_DIR is set.
CASES_DIR="${ANIX_INSTALL_FRESH_CONFIG_CASES_DIR:-}"
[[ -z "${CASES_DIR}" ]] || mkdir -p "${CASES_DIR}"

# yaml_quote is the YAML double-quoted spelling of a value, computed with sed
# (the installer does it in bash).
yaml_quote() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

# awkward_case <name> <email> <password>
awkward_case() {
  local name="$1" email="$2" password="$3" status=0
  new_install "case-${name}"
  ADMIN_EMAIL="${email}" ADMIN_PASSWORD="${password}"
  (write_fresh_config) >"${temporary}/case-${name}.log" 2>&1 || status=$?
  check "${name}: the installer accepts the values" test "${status}" -eq 0
  check "${name}: email written verbatim" \
    grep -qxF "  email: \"$(yaml_quote "${email}")\"" "${CONFIG_FILE}"
  check "${name}: password written verbatim" \
    grep -qxF "  password: \"$(yaml_quote "${password}")\"" "${CONFIG_FILE}"
  check "${name}: the bootstrap password file holds the raw password" \
    test "$(cat "${INSTALL_DIR}/.bootstrap-admin-password")" = "${password}"
  check "${name}: one admin password line" test "$(grep -c '^  password:' "${CONFIG_FILE}")" -eq 1
  if [[ -n "${CASES_DIR}" && -s "${CONFIG_FILE}" ]]; then
    cp "${CONFIG_FILE}" "${CASES_DIR}/${name}.yaml"
    printf '%s' "${email}" >"${CASES_DIR}/${name}.email"
    printf '%s' "${password}" >"${CASES_DIR}/${name}.password"
  fi
}

# The values are literal on purpose: a lone backslash, and $ and ` that must not
# expand (SC1003, SC2016 are disabled for the file).
operator="admin@example.com"
awkward_case backslash-letter    "${operator}" 'ab\qcd'
awkward_case double-backslash    "${operator}" 'a\\b'
awkward_case lone-backslash      "${operator}" '\'
awkward_case trailing-backslash  "${operator}" 'secret\'
awkward_case escape-lookalikes   "${operator}" 'line\nbreak\t\x41é\0'
awkward_case double-quote        "${operator}" 'say "hi" now'
awkward_case quote-and-backslash "${operator}" 'a\"b"\c'
awkward_case single-quote        "${operator}" "it's a 'quoted' secret"
awkward_case dollar              "${operator}" 'p$HOME $(id) `id` ${PATH}'
awkward_case ampersand           "${operator}" 'a&b && \& \1 &'
awkward_case hash                "${operator}" 'pa#ss # not a comment'
awkward_case spaces              "${operator}" '  leading and  inner spaces  '
awkward_case unicode             "${operator}" 'pässwörd-密码-🔑'
awkward_case yaml-indicators     "${operator}" '*anchor &x !tag {a: b} [c] | > - ? : , @ % ~'
awkward_case email-apostrophe    "o'brien@example.com" 'pw-1'
awkward_case email-ampersand     'r&d@example.com' 'pw-2'
awkward_case email-backslash     'back\slash@example.com' 'pw-3'

# The Go test loads this file with the real configuration loader.
if [[ -n "${ANIX_INSTALL_FRESH_CONFIG_OUT:-}" ]]; then
  cp "${fresh_config}" "${ANIX_INSTALL_FRESH_CONFIG_OUT}"
fi

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'install fresh config: all checks passed\n'
