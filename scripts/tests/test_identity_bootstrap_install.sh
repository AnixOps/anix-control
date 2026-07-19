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
IDENTITY_BOOTSTRAP_DIR="/opt/anixops/control/bootstrap/identity-platform-4.0.0"
APP_USER="$(id -gn)"

printf '%s\n' \
  'app:' \
  '  name: "AnixOps Control"' \
  'plugins:' \
  '  official_public_key: "test-root"' \
  '  identity_bootstrap_package_dir: ""' \
  '  control_execution_enabled: false' \
  '  dispatch_enabled: false' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx "  identity_bootstrap_package_dir: \"${IDENTITY_BOOTSTRAP_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx '  control_execution_enabled: true' "${CONFIG_FILE}" >/dev/null
[[ "$(grep -c '^[[:space:]]*identity_bootstrap_package_dir:' "${CONFIG_FILE}")" -eq 1 ]]
[[ "$(grep -c '^[[:space:]]*control_execution_enabled:' "${CONFIG_FILE}")" -eq 1 ]]

printf '%s\n' \
  'app:' \
  '  name: "AnixOps Control"' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx 'plugins:' "${CONFIG_FILE}" >/dev/null
grep -Fx "  identity_bootstrap_package_dir: \"${IDENTITY_BOOTSTRAP_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx '  control_execution_enabled: true' "${CONFIG_FILE}" >/dev/null

printf '%s\n' \
  'plugins:' \
  '  identity_bootstrap_package_dir: "/srv/operator-selected-bootstrap"' \
  '  control_execution_enabled: false' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx '  identity_bootstrap_package_dir: "/srv/operator-selected-bootstrap"' "${CONFIG_FILE}" >/dev/null
grep -Fx '  control_execution_enabled: true' "${CONFIG_FILE}" >/dev/null

FRESH_CONFIG=1
VERSION="v4.0.0"
INSTALL_DIR="${temporary}/install"
PLUGIN_HOST_RUNTIME_DIR="${temporary}/install/runtime/plugin-hosts"
PLUGIN_ARTIFACT_DIR="${temporary}/install/data/plugin-artifacts"
printf '%s\n' \
  'plugins:' \
  '  identity_bootstrap_package_dir: ""' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/var/lib/anixops/plugin-hosts"' \
  '  control_host_artifact_dir: "/var/lib/anixops/plugin-artifacts"' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

FRESH_CONFIG=0
VERSION="v4.0.0"
printf '%s\n' \
  'plugins:' \
  '  identity_bootstrap_package_dir: ""' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/var/lib/anixops/plugin-hosts"' \
  '  control_host_artifact_dir: "/var/lib/anixops/plugin-artifacts"' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

printf '%s\n' \
  'plugins:' \
  '  identity_bootstrap_package_dir: ""' \
  '  control_execution_enabled: false' \
  '  control_host_runtime_dir: "/srv/operator/plugin-hosts"' \
  '  control_host_artifact_dir: "/srv/operator/plugin-artifacts"' >"${CONFIG_FILE}"

configure_identity_bootstrap_config
grep -Fx "  control_host_runtime_dir: \"${PLUGIN_HOST_RUNTIME_DIR}\"" "${CONFIG_FILE}" >/dev/null
grep -Fx "  control_host_artifact_dir: \"${PLUGIN_ARTIFACT_DIR}\"" "${CONFIG_FILE}" >/dev/null

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
  download_identity_bootstrap_assets() { :; }
  stage_identity_bootstrap_assets() { :; }
  stop_running_service() { :; }
  backup_current_release() { :; }
  install_release_files() { :; }
  configure_identity_bootstrap_config() { :; }
  write_systemd_unit() { :; }

  main install --version v4.0.0 --install-dir "${INSTALL_DIR}" --skip-start
)

test_normal_v4_main_initializes_plugin_execution_dirs

if [[ "${EUID}" -eq 0 ]]; then
  TMP_DIR="${temporary}/downloads"
  INSTALL_DIR="${temporary}/install"
  VERSION="v4.0.0"
  BACKUP_ROOT="${INSTALL_DIR}/backups"
  PACKAGE_VERSION="4.0.0"
  IDENTITY_BOOTSTRAP_DIR="${INSTALL_DIR}/bootstrap/identity-platform-${PACKAGE_VERSION}"
  PLUGIN_HOST_RUNTIME_DIR="${INSTALL_DIR}/runtime/plugin-hosts"
  PLUGIN_ARTIFACT_DIR="${INSTALL_DIR}/data/plugin-artifacts"
  install -d "${TMP_DIR}"
  for asset in \
    "identity-platform-${PACKAGE_VERSION}.anxp" \
    "identity-platform-${PACKAGE_VERSION}.manifest.json" \
    "identity-platform-${PACKAGE_VERSION}.manifest.sig"; do
    printf '%s\n' "fixture ${asset}" >"${TMP_DIR}/${asset}"
  done

  stage_identity_bootstrap_assets
  [[ "$(stat -c '%a' "${IDENTITY_BOOTSTRAP_DIR}")" == '750' ]]
  for asset in "${IDENTITY_BOOTSTRAP_DIR}"/*; do
    [[ "$(stat -c '%a' "${asset}")" == '640' ]]
  done

  printf '%s\n' 'plugins:' >"${CONFIG_FILE}"
  ensure_layout_and_config
  [[ "$(stat -c '%a' "${PLUGIN_HOST_RUNTIME_DIR}")" == '700' ]]
  [[ "$(stat -c '%a' "${PLUGIN_ARTIFACT_DIR}")" == '700' ]]
  [[ "$(stat -c '%U' "${PLUGIN_HOST_RUNTIME_DIR}")" == "${APP_USER}" ]]
  [[ "$(stat -c '%U' "${PLUGIN_ARTIFACT_DIR}")" == "${APP_USER}" ]]
fi

printf '%s\n' 'identity bootstrap installer configuration tests passed'
