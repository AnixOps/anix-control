#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${REPO_ROOT}/scripts/install.sh"

temporary="$(mktemp -d)"
cleanup() { find "${temporary}" -depth -delete; }
trap cleanup EXIT

# The production installer runs as root. Keep this focused configuration test
# unprivileged by preventing ownership changes to temporary fixtures.
chown() { :; }

CONFIG_FILE="${temporary}/config.yaml"
APP_USER="$(id -gn)"

# The official packages are built into the binary and always run: the
# installer drops the plugins.* switches Control no longer reads, keeps the
# other keys and never writes an identity package directory.
printf '%s\n' \
  'app:' \
  '  name: "AnixOps Control"' \
  'plugins:' \
  '  official_public_key: "test-root"' \
  '  control_execution_enabled: false' \
  '  control_poll_interval: "5s"' \
  '  dispatch_enabled: false' \
  '  dispatch_poll_interval: "5s"' \
  '  topology_execution_enabled: false' \
  '  topology_poll_interval: "5s"' \
  'grpc:' \
  '  enabled: true' >"${CONFIG_FILE}"

configure_plugin_execution_config
grep -Fx '  official_public_key: "test-root"' "${CONFIG_FILE}" >/dev/null
grep -Fx '  control_poll_interval: "5s"' "${CONFIG_FILE}" >/dev/null
grep -Fx '  enabled: true' "${CONFIG_FILE}" >/dev/null
if grep -qE '^[[:space:]]*(control_execution_enabled|dispatch_enabled|dispatch_poll_interval|topology_execution_enabled|topology_poll_interval):' "${CONFIG_FILE}"; then
  printf 'the installer must drop the removed plugins.* switches\n' >&2
  exit 1
fi
if grep -q 'identity_bootstrap_package_dir' "${CONFIG_FILE}"; then
  printf 'the installer must not write an identity package directory\n' >&2
  exit 1
fi

printf '%s\n' \
  'app:' \
  '  name: "AnixOps Control"' >"${CONFIG_FILE}"

configure_plugin_execution_config
if grep -q 'control_execution_enabled' "${CONFIG_FILE}"; then
  printf 'the installer must not write control_execution_enabled\n' >&2
  exit 1
fi
if grep -q 'identity_bootstrap_package_dir' "${CONFIG_FILE}"; then
  printf 'the installer must not write an identity package directory\n' >&2
  exit 1
fi

# An operator's existing directory line stays as it is (Control ignores it).
printf '%s\n' \
  'plugins:' \
  '  identity_bootstrap_package_dir: "/srv/operator-selected-bootstrap"' \
  '  control_execution_enabled: false' >"${CONFIG_FILE}"

configure_plugin_execution_config
grep -Fx '  identity_bootstrap_package_dir: "/srv/operator-selected-bootstrap"' "${CONFIG_FILE}" >/dev/null

FRESH_CONFIG=1
VERSION="v4.0.0"
INSTALL_DIR="${temporary}/install"
PLUGIN_HOST_RUNTIME_DIR="${temporary}/install/runtime/plugin-hosts"
PLUGIN_ARTIFACT_DIR="${temporary}/install/data/plugin-artifacts"
printf '%s\n' \
  'plugins:' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/var/lib/anixops/plugin-hosts"' \
  '  control_host_artifact_dir: "/var/lib/anixops/plugin-artifacts"' >"${CONFIG_FILE}"

configure_plugin_execution_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

FRESH_CONFIG=0
VERSION="v4.0.0"
printf '%s\n' \
  'plugins:' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/var/lib/anixops/plugin-hosts"' \
  '  control_host_artifact_dir: "/var/lib/anixops/plugin-artifacts"' >"${CONFIG_FILE}"

configure_plugin_execution_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

printf '%s\n' \
  'plugins:' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/srv/operator/plugin-hosts"' \
  '  control_host_artifact_dir: "/srv/operator/plugin-artifacts"' >"${CONFIG_FILE}"

configure_plugin_execution_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

# The installer's key is the built-in official package root.
configured_root="$(sed -n 's/^[[:space:]]*official_public_key:[[:space:]]*"\([^"]*\)".*/\1/p' "${REPO_ROOT}/internal/config/defaults.yaml")"
[[ "${OFFICIAL_PUBLIC_KEY}" == "${configured_root}" ]] || {
  printf 'scripts/install.sh OFFICIAL_PUBLIC_KEY does not match internal/config/defaults.yaml\n' >&2
  exit 1
}

# SHA256SUMS.txt is trusted only after its signature verifies.
test_release_checksums_signature() (
  TMP_DIR="${temporary}/checksums"
  install -d "${TMP_DIR}"
  openssl genpkey -algorithm ED25519 -out "${TMP_DIR}/key.pem" 2>/dev/null
  OFFICIAL_PUBLIC_KEY="$(openssl pkey -in "${TMP_DIR}/key.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64)"
  printf '%s  anix-control-linux-amd64.tar.gz\n' "$(printf 'binary' | sha256sum | cut -d' ' -f1)" >"${TMP_DIR}/SHA256SUMS.txt"
  openssl pkeyutl -sign -inkey "${TMP_DIR}/key.pem" -rawin -in "${TMP_DIR}/SHA256SUMS.txt" -out "${TMP_DIR}/sig.bin"
  base64 -w0 "${TMP_DIR}/sig.bin" >"${TMP_DIR}/signature"
  download_asset() { cp "${TMP_DIR}/signature" "${TMP_DIR}/$1"; }
  verify_release_checksums >/dev/null
  printf 'tampered\n' >>"${TMP_DIR}/SHA256SUMS.txt"
  if (verify_release_checksums >/dev/null 2>&1); then
    printf 'a tampered SHA256SUMS.txt must be refused\n' >&2
    exit 1
  fi
)
test_release_checksums_signature

test_normal_v4_main_initializes_plugin_execution_dirs() (
  VERSION=""
  COMMAND="install"
  SKIP_START=0
  INSTALL_DIR="${temporary}/main-flow-install"
  PLUGIN_HOST_RUNTIME_DIR=""
  PLUGIN_ARTIFACT_DIR=""
  cleanup() {
    [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]] && find "${TMP_DIR}" -depth -delete
  }

  need_root() { :; }
  detect_legacy_layout() { :; }
  validate_install_dir() { :; }
  install_base_tools() { :; }
  ensure_app_user() { :; }
  ensure_layout_and_config() {
    [[ "${PLUGIN_HOST_RUNTIME_DIR}" == "${INSTALL_DIR}/runtime/plugin-hosts" ]]
    [[ "${PLUGIN_ARTIFACT_DIR}" == "${INSTALL_DIR}/data/plugin-artifacts" ]]
  }
  asset_name() { printf 'anix-control-linux-amd64.tar.gz'; }
  download_asset() { :; }
  verify_asset() { :; }
  verify_release_checksums() { :; }
  stop_running_service() { :; }
  backup_current_release() { :; }
  install_release_files() { :; }
  configure_plugin_execution_config() { :; }
  write_systemd_unit() { :; }

  main install --version v4.0.0 --install-dir "${INSTALL_DIR}" --skip-start
)

test_normal_v4_main_initializes_plugin_execution_dirs

if [[ "${EUID}" -eq 0 ]]; then
  TMP_DIR="${temporary}/downloads"
  INSTALL_DIR="${temporary}/install"
  VERSION="v4.0.0"
  BACKUP_ROOT="${INSTALL_DIR}/backups"
  PLUGIN_HOST_RUNTIME_DIR="${INSTALL_DIR}/runtime/plugin-hosts"
  PLUGIN_ARTIFACT_DIR="${INSTALL_DIR}/data/plugin-artifacts"
  install -d "${TMP_DIR}"

  printf '%s\n' 'plugins:' >"${CONFIG_FILE}"
  ensure_layout_and_config
  [[ "$(stat -c '%a' "${PLUGIN_HOST_RUNTIME_DIR}")" == '700' ]]
  [[ "$(stat -c '%a' "${PLUGIN_ARTIFACT_DIR}")" == '700' ]]
  [[ "$(stat -c '%U' "${PLUGIN_HOST_RUNTIME_DIR}")" == "${APP_USER}" ]]
  [[ "$(stat -c '%U' "${PLUGIN_ARTIFACT_DIR}")" == "${APP_USER}" ]]
fi

for version in v1.0.0 v3.9.9 v3.10.0-rc.1; do
  VERSION="${version}"
  if uses_plugin_execution; then
    printf 'package execution must not apply to %s\n' "${version}" >&2
    exit 1
  fi
done

for version in v4.0.0 v4.2.0-rc.3 v5.0.0 v10.0.0; do
  VERSION="${version}"
  if ! uses_plugin_execution; then
    printf 'package execution must apply to %s\n' "${version}" >&2
    exit 1
  fi
done

printf '%s\n' 'package execution installer configuration tests passed'
