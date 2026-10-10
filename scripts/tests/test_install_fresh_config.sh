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
# awkward-credential case (<name>.yaml, .email, .password) for the same test,
# and the values the installer refuses (refused/<name>.email, .password), which
# the Go test proves the loader or the login check could not have carried.

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
# Characters next to the ones that are refused (section 7) are still carried:
# U+00A0, U+FEFF and U+FFFD are not controls, U+2027 and U+202A flank the line
# and paragraph separators U+2028 and U+2029 (blanks around them are the case
# the YAML loader trims), and the edges of the UTF-8 ranges.
awkward_case no-break-space      "${operator}" $'a\xc2\xa0b'
awkward_case zero-width-nbsp     "${operator}" $'\xef\xbb\xbfa\xef\xbb\xbf'
awkward_case replacement-char    "${operator}" $'a\xef\xbf\xbdb'
awkward_case separator-flanks    "${operator}" $'a \xe2\x80\xa7 b \xe2\x80\xaa c'
awkward_case range-edges         "${operator}" $'\xc2\xa0\xdf\xbf\xe0\xa0\x80\xed\x9f\xbf\xee\x80\x80\xef\xbf\xbd'
awkward_case astral-edges        "${operator}" $'\xf0\x90\x80\x80 \xf4\x8f\xbf\xbf'
awkward_case email-non-ascii     $'\xc3\xbcn\xc3\xaf@ex\xc3\xa4mple.com' 'pw-4'

# --- 7. A value that config.yaml or the login check cannot carry unchanged is
#        refused with a reason, before config.yaml is written (a corrected rerun
#        then starts from nothing). Accepting them made the install fail late,
#        or run with another password, and the next run kept that config.yaml:
#          - a control character: the YAML loader refuses most of them, folds a
#            carriage return and U+0085 into a space, and a raw tab is not valid
#            in the JSON of the install's login check;
#          - U+2028, U+2029: YAML line breaks, trimmed with the blanks next to them;
#          - bytes that are not UTF-8 (a password typed in a Latin-1 terminal).
REFUSED_DIR=""
if [[ -n "${CASES_DIR}" ]]; then
  REFUSED_DIR="${CASES_DIR}/refused"
  mkdir -p "${REFUSED_DIR}"
fi

# refusal_reported <status> <log> <fragment>: exit status 1 and the reason.
refusal_reported() { [[ "$1" -eq 1 ]] && grep -qF -- "$3" "$2"; }
# nothing_written <config> <password file>
nothing_written() { [[ ! -e "$1" && ! -e "$2" ]]; }

# refused_case <name> <email> <password> <fragment of the message>
refused_case() {
  local name="$1" email="$2" password="$3" fragment="$4" status=0
  new_install "refused-${name}"
  ADMIN_EMAIL="${email}" ADMIN_PASSWORD="${password}"
  (write_fresh_config) >"${temporary}/refused-${name}.log" 2>&1 || status=$?
  check "${name}: refused with the reason" \
    refusal_reported "${status}" "${temporary}/refused-${name}.log" "${fragment}"
  check "${name}: nothing is written" \
    nothing_written "${CONFIG_FILE}" "${INSTALL_DIR}/.bootstrap-admin-password"
  if [[ -n "${REFUSED_DIR}" ]]; then
    printf '%s' "${email}" >"${REFUSED_DIR}/${name}.email"
    printf '%s' "${password}" >"${REFUSED_DIR}/${name}.password"
  fi
}

control="must not contain a control character"
utf8="must be valid UTF-8 text without control characters"
refused_case tab                 "${operator}" $'pa\tss' "${control}"
refused_case carriage-return     "${operator}" $'pa\rss' "${control}"
refused_case trailing-cr         "${operator}" $'pass\r' "${control}"
refused_case line-break          "${operator}" $'pa\nss' "${control}"
refused_case escape              "${operator}" $'pa\x1bss' "${control}"
refused_case delete              "${operator}" $'pa\x7fss' "${control}"
refused_case email-tab           $'ad\tmin@example.com' 'pw-1' "${control}"
refused_case email-cr            $'admin@example.com\r' 'pw-2' "${control}"
refused_case email-line-break    $'admin@example.com\n' 'pw-3' "${control}"
refused_case next-line           "${operator}" $'pa\xc2\x85ss' "${utf8}"
refused_case c1-control          "${operator}" $'pa\xc2\x9fss' "${utf8}"
refused_case line-separator      "${operator}" $'a \xe2\x80\xa8 b' "${utf8}"
refused_case paragraph-separator "${operator}" $'a\xe2\x80\xa9 b ' "${utf8}"
refused_case noncharacter-fffe   "${operator}" $'pa\xef\xbf\xbess' "${utf8}"
refused_case noncharacter-ffff   "${operator}" $'pa\xef\xbf\xbfss' "${utf8}"
refused_case latin1              "${operator}" $'p\xe4ssw\xf6rd' "${utf8}"
refused_case email-latin1        $'caf\xe9@example.com' 'pw-4' "${utf8}"
refused_case lone-continuation   "${operator}" $'pa\x80ss' "${utf8}"
refused_case invalid-byte        "${operator}" $'pa\xffss' "${utf8}"
refused_case overlong            "${operator}" $'pa\xc0\x80ss' "${utf8}"
refused_case surrogate           "${operator}" $'pa\xed\xa0\x80ss' "${utf8}"
refused_case above-10ffff        "${operator}" $'pa\xf4\x90\x80\x80ss' "${utf8}"
refused_case truncated           "${operator}" $'pass\xe2\x82' "${utf8}"

# An empty value is refused too (the installer fills in a default before it asks).
status=0
(validate_admin_value password "") >"${temporary}/refused-empty.log" 2>&1 || status=$?
check "empty: refused with the reason" refusal_reported "${status}" "${temporary}/refused-empty.log" "must not be empty"

# Every C0 control and DEL, every C1 control, and the rest of the printable ASCII.
sweep() { # sweep <accepted|refused> <value...>; prints the values that got the other answer
  local want="$1" value status wrong=0
  shift
  for value in "$@"; do
    status=0
    (validate_admin_value password "${value}") >/dev/null 2>&1 || status=$?
    if [[ "${want}" == refused && "${status}" -ne 1 ]] || [[ "${want}" == accepted && "${status}" -ne 0 ]]; then
      wrong=$((wrong + 1))
    fi
  done
  [[ "${wrong}" -eq 0 ]]
}
c0=() c1=() ascii=()
for byte in $(seq 1 31) 127; do
  printf -v hex '%02x' "${byte}"
  c0+=("$(printf '%b' "pa\\x${hex}ss")")
done
for byte in $(seq 128 159); do
  printf -v hex '%02x' "${byte}"
  c1+=("$(printf '%b' "pa\\xc2\\x${hex}ss")")
done
for byte in $(seq 32 126); do
  printf -v hex '%02x' "${byte}"
  ascii+=("$(printf '%b' "pa\\x${hex}ss")")
done
check "sweep: all 32 C0 controls and DEL are refused" sweep refused "${c0[@]}"
check "sweep: all 32 C1 controls (U+0080..U+009F) are refused" sweep refused "${c1[@]}"
check "sweep: all 95 printable ASCII characters are accepted" sweep accepted "${ascii[@]}"

# --- 8. The verdict does not depend on the host's locale: bash matches bytes
#        under LC_ALL=C (a UTF-8 locale used to be the only one that saw a
#        well-formed sequence, and an invalid one confuses its pattern matcher).
locale_failures=0
for locale_name in C POSIX C.UTF-8 en_US.UTF-8; do
  while IFS='|' read -r want value; do
    got="$(LC_ALL="${locale_name}"; admin_value_error "$(printf '%b' "${value}")")"
    [[ "${got}" == "${want}" ]] || { locale_failures=$((locale_failures + 1)); printf '     %s: %q gave %q, want %q\n' "${locale_name}" "${value}" "${got}" "${want}"; }
  done <<'EOF'
|plain-secret_1!
|p\xc3\xa4ssw\xc3\xb6rd-\xe5\xaf\x86\xe7\xa0\x81-\xf0\x9f\x94\x91
text|p\xe4ssw\xf6rd
text|pa\xc2\x85ss
text|a \xe2\x80\xa8 b
control|pa\tss
control|pass\r
EOF
done
check "locale: C, POSIX and UTF-8 locales give the same verdicts" test "${locale_failures}" -eq 0

# The Go test loads this file with the real configuration loader.
if [[ -n "${ANIX_INSTALL_FRESH_CONFIG_OUT:-}" ]]; then
  cp "${fresh_config}" "${ANIX_INSTALL_FRESH_CONFIG_OUT}"
fi

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  exit 1
fi
printf 'install fresh config: all checks passed\n'
