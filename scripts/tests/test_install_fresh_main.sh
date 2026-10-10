#!/usr/bin/env bash
# A fresh systemd install runs main() of scripts/install.sh to the end and the
# script exits 0, and so does the update that follows it.
#
# main() and everything it calls are the installer's own, in particular
# backup_current_release: test_identity_bootstrap_install.sh stubs it, which
# hid that a host with no previous release (nothing to back up) aborted there
# with status 1 before the binary, the unit or the service were installed. Only
# what needs a real host is replaced: the root check, systemctl, chown, curl
# and `install -o/-g`. The installation, the unit directory and the scratch
# directory live in a temporary directory, useradd and friends are tripwires,
# and no service is started.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/install.sh
source "${REPO_ROOT}/scripts/install.sh"
set +E
trap - ERR

temporary="$(mktemp -d)"
test_cleanup() { find "${temporary}" -depth -delete; }
trap test_cleanup EXIT

# --- The fake root: nothing below touches the host.
need_root() { :; }
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

# systemctl keeps the state of one service and records every call.
SYSTEMCTL_LOG="${temporary}/systemctl.log"
SERVICE_STATE="${temporary}/service-active"
: >"${SYSTEMCTL_LOG}"
systemctl() {
  printf '%s\n' "$*" >>"${SYSTEMCTL_LOG}"
  case "${1:-}" in
    is-active) [[ -f "${SERVICE_STATE}" ]] ;;
    restart|start) : >"${SERVICE_STATE}" ;;
    stop) rm -f "${SERVICE_STATE}" ;;
    *) : ;;
  esac
}

# Account and group management must never be reached: the installer is told to
# use the current user, so these exist only to be found by install_base_tools
# and to fail loudly (and be recorded) if a change makes the installer call them.
TRIPWIRE_LOG="${temporary}/tripwire.log"
: >"${TRIPWIRE_LOG}"
mkdir -p "${temporary}/tripwire"
for tool in useradd groupadd userdel groupdel usermod; do
  printf '#!/bin/sh\necho "%s $*" >>"%s"\nexit 1\n' "${tool}" "${TRIPWIRE_LOG}" >"${temporary}/tripwire/${tool}"
  chmod 0755 "${temporary}/tripwire/${tool}"
done
PATH="${temporary}/tripwire:${PATH}"

# --- A release the stub curl serves, checksummed like the real one.
VERSION_UNDER_TEST="v4.2.0"
RELEASE_DIR="${temporary}/release"
mkdir -p "${RELEASE_DIR}" "${temporary}/stage/bin" "${temporary}/stage/web"
archive="$(asset_name)"
binary="${archive%.tar.gz}"
printf '#!/bin/sh\necho "anix-control fixture"\n' >"${temporary}/stage/bin/${binary}"
chmod 0755 "${temporary}/stage/bin/${binary}"
tar -czf "${RELEASE_DIR}/${archive}" -C "${temporary}/stage/bin" "${binary}"
printf '<!doctype html><title>fixture</title>\n' >"${temporary}/stage/web/index.html"
tar -czf "${RELEASE_DIR}/anix-control-frontend.tar.gz" -C "${temporary}/stage/web" index.html
package_version="${VERSION_UNDER_TEST#v}"
for suffix in anxp manifest.json manifest.sig; do
  printf 'fixture %s\n' "${suffix}" >"${RELEASE_DIR}/identity-platform-${package_version}.${suffix}"
done
(
  cd "${RELEASE_DIR}"
  sha256sum -- "${archive}" anix-control-frontend.tar.gz \
    "identity-platform-${package_version}.anxp" \
    "identity-platform-${package_version}.manifest.json" \
    "identity-platform-${package_version}.manifest.sig" >SHA256SUMS.txt
)

# curl serves the release assets, the version-matched configuration template
# and the health and login endpoints; it records every URL and login payload.
CURL_LOG="${temporary}/curl.log"
LOGIN_LOG="${temporary}/login.log"
: >"${CURL_LOG}"
: >"${LOGIN_LOG}"
curl() {
  local out="" url="" write_out="" data=""
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      -o|--output) out="$2"; shift 2 ;;
      -w|--write-out) write_out="$2"; shift 2 ;;
      --data|-d) data="$2"; shift 2 ;;
      -H|--retry|--retry-delay|--connect-timeout|--max-time) shift 2 ;;
      -*) shift ;;
      *) url="$1"; shift ;;
    esac
  done
  printf '%s\n' "${url}" >>"${CURL_LOG}"
  case "${url}" in
    */config/config.yaml.example)
      cp "${REPO_ROOT}/config/config.yaml.example" "${out}"
      ;;
    */releases/download/*)
      [[ -f "${RELEASE_DIR}/${url##*/}" ]] || return 22
      cp "${RELEASE_DIR}/${url##*/}" "${out}"
      ;;
    */api/v2/login)
      printf '%s\n' "${data}" >>"${LOGIN_LOG}"
      # A JSON string cannot hold a raw control character: the server answers 400.
      [[ "${data}" != *[[:cntrl:]]* ]] || return 22
      if [[ -n "${write_out}" ]]; then printf '200'; else printf '{"code":0,"data":{"token":"fixture"}}'; fi
      ;;
    */health) ;;
    *) return 22 ;;
  esac
}

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }
check() {
  local name="$1"
  shift
  if "$@"; then pass "${name}"; else fail "${name}"; fi
}
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

# Nothing from the caller's environment may decide the result.
ADMIN_EMAIL="" ADMIN_PASSWORD="" GRPC_NAME="" GRPC_TLS_CERT="" GRPC_TLS_KEY=""
APP_USER="$(id -un)"
SERVICE_NAME="anix-control"
SYSTEMD_UNIT_DIR="${temporary}/systemd"
mkdir -p "${SYSTEMD_UNIT_DIR}"
export TMPDIR="${temporary}/tmp"
mkdir -p "${TMPDIR}"

# run_main <log> <main arguments...> runs the real main() the way the script
# does at its end: as a plain statement under errexit, trap and all, in a
# subshell so that its exit does not end this test. RUN_STATUS is its status.
# It must not run in an `if` or `||` context: errexit is ignored there, and a
# statement that fails in main would be skipped instead of aborting it.
RUN_STATUS=0
run_main() {
  local log="$1"
  shift
  set +e
  (
    trap cleanup EXIT
    set -e
    main "$@"
  ) >"${log}" 2>&1
  RUN_STATUS=$?
  set -e
}

# The function under test is the installer's own, not a stub.
check "backup_current_release is the installer's own" \
  grep -qF "Backed up the previous release" <(declare -f backup_current_release)

# --- 1. Fresh install on an empty installation directory.
FRESH="${temporary}/fresh/control"
mkdir -p "${FRESH}"
check "fresh: the installation directory starts empty" test -z "$(ls -A "${FRESH}")"
run_main "${temporary}/fresh.log" install --version "${VERSION_UNDER_TEST}" \
  --install-dir "${FRESH}" --health-url "http://127.0.0.1:18080/health"
check "fresh: main exits 0" test "${RUN_STATUS}" -eq 0
check "fresh: reports success" grep -qF "Installed ${VERSION_UNDER_TEST} successfully." "${temporary}/fresh.log"
check "fresh: the binary is installed" test -x "${FRESH}/bin/anix-control"
check "fresh: the v2board compatibility link points at it" \
  test "$(readlink "${FRESH}/bin/v2board")" = anix-control
check "fresh: the frontend is installed" test -f "${FRESH}/web/public/index.html"
check "fresh: the release version is recorded" test "$(cat "${FRESH}/.release-version")" = "${VERSION_UNDER_TEST}"
check "fresh: the systemd unit is written" test -s "${SYSTEMD_UNIT_DIR}/anix-control.service"
check "fresh: the unit starts the installed binary with the installed config" \
  grep -qxF "ExecStart=${FRESH}/bin/anix-control -config ${FRESH}/config/config.yaml" "${SYSTEMD_UNIT_DIR}/anix-control.service"
check "fresh: the config is written" test -s "${FRESH}/config/config.yaml"
check "fresh: the config is production" test "$(top_level env "${FRESH}/config/config.yaml")" = production
check "fresh: the identity bootstrap package is staged" \
  test -s "${FRESH}/bootstrap/identity-platform-${package_version}/identity-platform-${package_version}.anxp"
check "fresh: the identity bootstrap directory is configured" \
  grep -qF "identity_bootstrap_package_dir: \"${FRESH}/bootstrap/identity-platform-${package_version}\"" "${FRESH}/config/config.yaml"
check "fresh: the CA key is generated" test -s "${FRESH}/config/secrets/module_ca_kek"
check "fresh: the bootstrap password file is written" test -s "${FRESH}/.bootstrap-admin-password"
check "fresh: nothing was backed up (there was no previous release)" \
  test -z "$(find "${FRESH}/backups" -mindepth 1 -maxdepth 1 2>/dev/null)"
check "fresh: the unit was enabled" grep -qxF "enable anix-control" "${SYSTEMCTL_LOG}"
check "fresh: the service was started" grep -qxF "restart anix-control" "${SYSTEMCTL_LOG}"
admin_email="$(awk '/^admin:/ {section = 1; next} /^[A-Za-z_]/ {section = 0} section && /^[[:space:]]+email:/ {gsub(/^[^"]*"|".*$/, ""); print; exit}' "${FRESH}/config/config.yaml")"
check "fresh: an administrator email is configured" test -n "${admin_email}"
check "fresh: the login check used the configured administrator" \
  grep -qF "\"email\":\"${admin_email}\"" "${LOGIN_LOG}"
check "fresh: no scratch directory is left behind" test -z "$(ls -A "${TMPDIR}")"
check "fresh: no account or group command was run" test ! -s "${TRIPWIRE_LOG}"

# --- 2. Update: the same directory, an existing configuration.
config_before="$(sha256sum "${FRESH}/config/config.yaml")"
: >"${SYSTEMCTL_LOG}"
run_main "${temporary}/update.log" update --version "${VERSION_UNDER_TEST}" \
  --install-dir "${FRESH}" --health-url "http://127.0.0.1:18080/health"
check "update: main exits 0" test "${RUN_STATUS}" -eq 0
check "update: reports success" grep -qF "Installed ${VERSION_UNDER_TEST} successfully." "${temporary}/update.log"
check "update: the existing config is preserved byte for byte" \
  test "$(sha256sum "${FRESH}/config/config.yaml")" = "${config_before}"
check "update: the running service was stopped first" grep -qxF "stop anix-control" "${SYSTEMCTL_LOG}"
check "update: the service was started again" grep -qxF "restart anix-control" "${SYSTEMCTL_LOG}"
backup="$(find "${FRESH}/backups" -mindepth 1 -maxdepth 1 -type d -name "*-${VERSION_UNDER_TEST}" | head -n 1)"
check "update: the previous release was backed up" test -n "${backup}"
check "update: the backup holds the binary" test -x "${backup}/anix-control"
check "update: the backup holds the config" test -s "${backup}/config.yaml"
check "update: no scratch directory is left behind" test -z "$(ls -A "${TMPDIR}")"

# --- 3. rollback is the install flow with the requested tag.
run_main "${temporary}/rollback.log" rollback --version "${VERSION_UNDER_TEST}" \
  --install-dir "${FRESH}" --health-url "http://127.0.0.1:18080/health"
check "rollback: main exits 0" test "${RUN_STATUS}" -eq 0
check "rollback: says so" grep -qF "Rollback completed by installing the requested release tag." "${temporary}/rollback.log"

# --- 4. --skip-start on a directory that does not exist yet.
SKIPPED="${temporary}/skipped/control"
run_main "${temporary}/skip.log" install --version "${VERSION_UNDER_TEST}" \
  --install-dir "${SKIPPED}" --skip-start
check "skip-start: main exits 0" test "${RUN_STATUS}" -eq 0
check "skip-start: the binary is installed" test -x "${SKIPPED}/bin/anix-control"
check "skip-start: the unit is written" test -s "${SYSTEMD_UNIT_DIR}/anix-control.service"
check "skip-start: says the start was skipped" grep -qF "service start was skipped" "${temporary}/skip.log"

# --- 5. A bootstrap value that config.yaml or the login check cannot carry
#        unchanged (a control character, bytes that are not UTF-8) is refused
#        before config.yaml is written, with the reason. It used to be written
#        as given: a tab made the install's own login check post invalid JSON
#        ("Installation failed health verification" for a healthy service), a
#        carriage return was folded into a space, and either way the next run
#        kept that config.yaml.
# refused_main <name> <email> <password> <fragment of the message>
refused_main() {
  local name="$1" email="$2" password="$3" fragment="$4"
  local dir="${temporary}/${name}/control"
  : >"${CURL_LOG}"
  : >"${SYSTEMCTL_LOG}"
  run_main "${temporary}/${name}.log" install --version "${VERSION_UNDER_TEST}" \
    --install-dir "${dir}" --admin-email "${email}" --admin-password "${password}" \
    --health-url "http://127.0.0.1:18080/health"
  check "${name}: main exits 1" test "${RUN_STATUS}" -eq 1
  check "${name}: says why" grep -qF "${fragment}" "${temporary}/${name}.log"
  check "${name}: no config.yaml is written" test ! -e "${dir}/config/config.yaml"
  check "${name}: no bootstrap password file is written" test ! -e "${dir}/.bootstrap-admin-password"
  check "${name}: no binary is installed" test ! -e "${dir}/bin/anix-control"
  check "${name}: nothing is downloaded" test ! -s "${CURL_LOG}"
  check "${name}: the service is not touched" test ! -s "${SYSTEMCTL_LOG}"
}
refused_main tab-password admin@example.com $'pa\tss' "must not contain a control character"
refused_main cr-password admin@example.com $'pass\r' "must not contain a control character"
refused_main cr-email $'admin@example.com\r' secret "must not contain a control character"
refused_main latin1-password admin@example.com $'p\xe4ssw\xf6rd' "must be valid UTF-8 text"

# The same directory, corrected: the refusal left nothing behind to trip over.
run_main "${temporary}/corrected.log" install --version "${VERSION_UNDER_TEST}" \
  --install-dir "${temporary}/tab-password/control" --admin-email admin@example.com \
  --admin-password "corrected secret" --health-url "http://127.0.0.1:18080/health"
check "corrected rerun: main exits 0" test "${RUN_STATUS}" -eq 0
check "corrected rerun: the password is the corrected one" \
  grep -qxF '  password: "corrected secret"' "${temporary}/tab-password/control/config/config.yaml"
check "corrected rerun: the login check posted it" \
  grep -qF '"password":"corrected secret"' "${LOGIN_LOG}"

check "no account or group command was run at all" test ! -s "${TRIPWIRE_LOG}"

if [[ "${failures}" -ne 0 ]]; then
  printf '%d check(s) failed\n' "${failures}"
  for log in fresh update rollback skip corrected; do
    if [[ -f "${temporary}/${log}.log" ]]; then
      printf -- '--- %s.log (last lines)\n' "${log}"
      tail -n 8 "${temporary}/${log}.log"
    fi
  done
  exit 1
fi
printf 'install fresh main: all checks passed\n'
