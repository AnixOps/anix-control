#!/usr/bin/env bash
# Install or upgrade a GitHub Actions-built AnixOps Control release without cloning
# the repository or compiling on the target host.

set -Eeuo pipefail

REPO_OWNER="${REPO_OWNER:-AnixOps}"
REPO_NAME="${REPO_NAME:-anix-control}"
API_BASE="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}"
RELEASE_BASE="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download"
RAW_BASE="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}"

SERVICE_NAME_EXPLICIT=0
APP_USER_EXPLICIT=0
INSTALL_DIR_EXPLICIT=0
[[ -n "${SERVICE_NAME+x}" ]] && SERVICE_NAME_EXPLICIT=1
[[ -n "${APP_USER+x}" ]] && APP_USER_EXPLICIT=1
[[ -n "${INSTALL_DIR+x}" ]] && INSTALL_DIR_EXPLICIT=1

SERVICE_NAME="${SERVICE_NAME:-anix-control}"
APP_USER="${APP_USER:-anixops}"
INSTALL_DIR="${INSTALL_DIR:-/opt/anixops/control}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/health}"

BINARY_PATH=""
LEGACY_BINARY_PATH=""
CONFIG_FILE=""
FRONTEND_DIR=""
BACKUP_ROOT=""
VERSION_FILE=""
TMP_DIR=""
BACKUP_DIR=""
WAS_ACTIVE=0
SKIP_START=0
COMMAND="install"
VERSION="${ANIX_CONTROL_VERSION:-${V2BOARD_VERSION:-}}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"

# Native production-layout migration paths.  They are overridable for fixture
# tests, but default to the documented paths on a real host.
LEGACY_PROGRAM_DIR="${LEGACY_PROGRAM_DIR:-/usr/local/v2board}"
LEGACY_CONFIG_DIR="${LEGACY_CONFIG_DIR:-/etc/v2board}"
LEGACY_DATA_DIR="${LEGACY_DATA_DIR:-/var/lib/v2board}"
LEGACY_SERVICE_FILE="${LEGACY_SERVICE_FILE:-/etc/systemd/system/v2board.service}"
MIGRATION_PLAN_PATH="${MIGRATION_PLAN_PATH:-migration-plan.json}"
MIGRATION_BACKUP_DIR="${MIGRATION_BACKUP_DIR:-/var/backups/anixops-control}"
TARGET_CONFIG_DIR="${TARGET_CONFIG_DIR:-/etc/anixops/control}"
TARGET_DATA_DIR="${TARGET_DATA_DIR:-/var/lib/anixops/control}"
TARGET_LOG_DIR="${TARGET_LOG_DIR:-/var/log/anixops/control}"
MIGRATION_SNAPSHOT=""

info() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

usage() {
  cat <<'EOF'
Usage:
  install.sh [install|update|rollback|preflight|migrate] [options]

Options:
  --version <tag>          Release tag, for example v4.0.0-alpha.6. Defaults to GitHub's latest stable release.
  --admin-email <email>    Bootstrap admin email on a fresh installation.
  --admin-password <text>  Bootstrap admin password on a fresh installation.
  --install-dir <path>     Installation root. Default: /opt/anixops/control.
  --health-url <url>       Health endpoint to verify after start.
  --skip-start             Install files and systemd unit without starting the service.
  --plan <path>            Preflight JSON output path (default: migration-plan.json).
  -h, --help               Show this help.

Only GitHub Release assets are downloaded. The installer never clones this
repository and never performs Go or frontend builds on the target host.
EOF
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

file_sha256() {
  [[ -f "$1" ]] && sha256sum "$1" | awk '{print $1}' || printf ''
}

legacy_layout_present() {
  [[ -x "${LEGACY_PROGRAM_DIR}/v2board" && -f "${LEGACY_CONFIG_DIR}/config.yaml" && -d "${LEGACY_DATA_DIR}" ]]
}

preflight_migration() {
  local output="${MIGRATION_PLAN_PATH}" service_state="inactive" db="${LEGACY_DATA_DIR}/v2board.db"
  install -d "$(dirname "${output}")" 2>/dev/null || true
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet v2board.service 2>/dev/null; then service_state="active"; fi
  local config_hash db_hash unit_hash
  config_hash="$(file_sha256 "${LEGACY_CONFIG_DIR}/config.yaml")"
  db_hash="$(file_sha256 "${db}")"
  unit_hash="$(file_sha256 "${LEGACY_SERVICE_FILE}")"
  local disk_available; disk_available="$(df -Pk "${LEGACY_DATA_DIR}" 2>/dev/null | awk 'NR==2 {print $4}' | head -n1)"
  cat > "${output}" <<EOF
{
  "schema": 1,
  "source": {"program": "$(json_escape "${LEGACY_PROGRAM_DIR}/v2board")", "config": "$(json_escape "${LEGACY_CONFIG_DIR}/config.yaml")", "data": "$(json_escape "${LEGACY_DATA_DIR}")", "service": "$(json_escape "${LEGACY_SERVICE_FILE}")"},
  "target": {"program": "${INSTALL_DIR}", "config": "${TARGET_CONFIG_DIR}", "data": "${TARGET_DATA_DIR}", "logs": "${TARGET_LOG_DIR}", "service": "${SERVICE_NAME}.service"},
  "service_state": "${service_state}",
  "files": {"config_sha256": "${config_hash}", "database_sha256": "${db_hash}", "service_sha256": "${unit_hash}"},
  "database_readable": $( [[ -r "${db}" ]] && printf true || printf false ),
  "layout_present": $( legacy_layout_present && printf true || printf false ),
  "disk_available_kb": "${disk_available:-unknown}",
  "dependencies": {"curl": $(command -v curl >/dev/null 2>&1 && printf true || printf false), "tar": $(command -v tar >/dev/null 2>&1 && printf true || printf false), "sha256sum": $(command -v sha256sum >/dev/null 2>&1 && printf true || printf false), "systemctl": $(command -v systemctl >/dev/null 2>&1 && printf true || printf false)}
}
EOF
  info "Wrote migration preflight plan to ${output}"
}

need_root() {
  [[ "${EUID}" -eq 0 ]] || die "Run this installer as root (for example: sudo bash install.sh ...)."
}

detect_legacy_layout() {
  if [[ "${SERVICE_NAME_EXPLICIT}" -eq 1 || "${APP_USER_EXPLICIT}" -eq 1 || "${INSTALL_DIR_EXPLICIT}" -eq 1 ]]; then
    return
  fi

  if [[ -f /opt/v2board/config/config.yaml || -x /opt/v2board/bin/v2board ]]; then
    SERVICE_NAME="v2board"
    APP_USER="v2board"
    INSTALL_DIR="/opt/v2board"
    warn "Detected the supported /opt/v2board layout; upgrading it in place with the AnixOps Control binary."
    return
  fi

  if systemctl list-unit-files v2board.service --no-legend 2>/dev/null | grep -q '^v2board\.service' ||
     [[ -x /usr/local/v2board/v2board || -f /etc/v2board/config.yaml || -d /var/lib/v2board ]]; then
    die "Detected a legacy v2board service/layout that cannot be migrated safely by the default installer. Stop and back up the old service, then rerun with explicit SERVICE_NAME, APP_USER and INSTALL_DIR values for its layout."
  fi
}

validate_install_dir() {
  [[ "${INSTALL_DIR}" == /* && "${INSTALL_DIR}" != "/" && "${INSTALL_DIR}" != *".."* ]] || \
    die "--install-dir must be a safe absolute path."
}

install_base_tools() {
  local missing=()
  local command
  for command in curl tar sha256sum systemctl install useradd; do
    command -v "${command}" >/dev/null 2>&1 || missing+=("${command}")
  done
  [[ "${#missing[@]}" -eq 0 ]] && return

  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y ca-certificates coreutils curl tar
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates coreutils curl tar
  elif command -v yum >/dev/null 2>&1; then
    yum install -y ca-certificates coreutils curl tar
  else
    die "Missing required commands: ${missing[*]}. This installer requires a systemd Linux host."
  fi

  for command in curl tar sha256sum systemctl install useradd; do
    command -v "${command}" >/dev/null 2>&1 || die "Required command is still unavailable: ${command}"
  done
}

validate_version() {
  [[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]] || \
    die "Invalid release tag: ${VERSION}"
}

latest_release() {
  local tag
  tag="$(curl -fsSL --retry 3 --connect-timeout 10 \
    -H 'Accept: application/vnd.github+json' \
    -H 'User-Agent: anix-control-installer' \
    "${API_BASE}/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)"
  [[ -n "${tag}" ]] || die "Could not resolve the latest stable GitHub Release. Pass --version explicitly."
  printf '%s\n' "${tag}"
}

asset_name() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'anix-control-linux-amd64.tar.gz\n' ;;
    aarch64|arm64) printf 'anix-control-linux-arm64.tar.gz\n' ;;
    *) die "Unsupported architecture: $(uname -m). Supported release installers are linux amd64 and arm64." ;;
  esac
}

download_asset() {
  local name="$1"
  local target="${TMP_DIR}/${name}"
  info "Downloading ${name} from ${VERSION}"
  curl -fL --retry 3 --retry-delay 2 --connect-timeout 15 --max-time 900 \
    "${RELEASE_BASE}/${VERSION}/${name}" -o "${target}"
}

verify_asset() {
  local name="$1"
  local expected actual
  expected="$(awk -v name="${name}" '$2 == name { print $1; exit }' "${TMP_DIR}/SHA256SUMS.txt")"
  [[ "${expected}" =~ ^[a-fA-F0-9]{64}$ ]] || die "SHA256SUMS.txt has no valid checksum for ${name}"
  actual="$(sha256sum "${TMP_DIR}/${name}" | awk '{print $1}')"
  [[ "${actual}" == "${expected}" ]] || die "Checksum verification failed for ${name}"
  info "Verified SHA-256 for ${name}"
}

ensure_app_user() {
  if ! id -u "${APP_USER}" >/dev/null 2>&1; then
    useradd --system --home-dir "${INSTALL_DIR}" --shell /usr/sbin/nologin "${APP_USER}"
  fi
}

random_secret() {
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

validate_config_value() {
  local value="$1"
  [[ -n "${value}" && "${value}" != *$'\n'* && "${value}" != *'"'* ]] || \
    die "Bootstrap values must be non-empty and must not contain a double quote or newline."
}

write_fresh_config() {
  local template="${TMP_DIR}/config.yaml.example"
  local jwt_secret api_token escaped_email escaped_password

  [[ -n "${ADMIN_EMAIL}" ]] || ADMIN_EMAIL="admin@localhost"
  [[ -n "${ADMIN_PASSWORD}" ]] || ADMIN_PASSWORD="$(random_secret)"
  validate_config_value "${ADMIN_EMAIL}"
  validate_config_value "${ADMIN_PASSWORD}"
  jwt_secret="$(random_secret)"
  api_token="$(random_secret)"
  escaped_email="${ADMIN_EMAIL//\\/\\\\}"
  escaped_password="${ADMIN_PASSWORD//\\/\\\\}"

  info "Downloading the version-matched configuration template"
  curl -fsSL --retry 3 --connect-timeout 15 \
    "${RAW_BASE}/${VERSION}/config/config.yaml.example" -o "${template}"

  awk \
    -v jwt_secret="${jwt_secret}" \
    -v api_token="${api_token}" \
    -v admin_email="${escaped_email}" \
    -v admin_password="${escaped_password}" '
      /^[A-Za-z_][A-Za-z0-9_]*:/ {
        section = $0
        sub(/:.*/, "", section)
      }
      section == "jwt" && $0 ~ /^[[:space:]]+secret:/ {
        print "  secret: \"" jwt_secret "\""
        next
      }
      section == "app" && $0 ~ /^[[:space:]]+api_token:/ {
        print "  api_token: \"" api_token "\""
        next
      }
      section == "admin" && $0 ~ /^[[:space:]]+email:/ {
        print "  email: \"" admin_email "\""
        next
      }
      section == "admin" && $0 ~ /^[[:space:]]+password:/ {
        print "  password: \"" admin_password "\""
        next
      }
      { print }
    ' "${template}" > "${CONFIG_FILE}"

  chmod 0640 "${CONFIG_FILE}"
  chown root:"${APP_USER}" "${CONFIG_FILE}"
  printf '%s\n' "${ADMIN_PASSWORD}" > "${INSTALL_DIR}/.bootstrap-admin-password"
  chmod 0640 "${INSTALL_DIR}/.bootstrap-admin-password"
  chown root:"${APP_USER}" "${INSTALL_DIR}/.bootstrap-admin-password"
  warn "Fresh installation: bootstrap admin email is ${ADMIN_EMAIL}"
  warn "The one-time bootstrap password is stored in ${INSTALL_DIR}/.bootstrap-admin-password; move it to a password manager, then delete the file."
}

ensure_layout_and_config() {
  install -d -m 0750 -o "${APP_USER}" -g "${APP_USER}" \
    "${INSTALL_DIR}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/config" \
    "${INSTALL_DIR}/config/data" "${INSTALL_DIR}/logs" "${INSTALL_DIR}/web"
  install -d -m 0700 -o root -g root "${BACKUP_ROOT}"

  if [[ ! -f "${CONFIG_FILE}" ]]; then
    write_fresh_config
  else
    info "Existing configuration found; preserving ${CONFIG_FILE}"
  fi
}

snapshot_migration_state() {
  MIGRATION_SNAPSHOT="${MIGRATION_BACKUP_DIR}/$(date -u +%Y%m%dT%H%M%SZ)-migration"
  install -d -m 0700 "${MIGRATION_SNAPSHOT}"
  [[ -f "${LEGACY_SERVICE_FILE}" ]] && cp -a "${LEGACY_SERVICE_FILE}" "${MIGRATION_SNAPSHOT}/v2board.service"
  [[ -x "${LEGACY_PROGRAM_DIR}/v2board" ]] && cp -a "${LEGACY_PROGRAM_DIR}/v2board" "${MIGRATION_SNAPSHOT}/v2board"
  [[ -d "${LEGACY_PROGRAM_DIR}/public" ]] && cp -a "${LEGACY_PROGRAM_DIR}/public" "${MIGRATION_SNAPSHOT}/public"
  [[ -d "${LEGACY_CONFIG_DIR}" ]] && cp -a "${LEGACY_CONFIG_DIR}" "${MIGRATION_SNAPSHOT}/config"
  [[ -d "${LEGACY_DATA_DIR}" ]] && cp -a "${LEGACY_DATA_DIR}" "${MIGRATION_SNAPSHOT}/data"
  sha256sum "${MIGRATION_SNAPSHOT}"/v2board "${MIGRATION_SNAPSHOT}"/config/config.yaml "${MIGRATION_SNAPSHOT}"/data/v2board.db 2>/dev/null > "${MIGRATION_SNAPSHOT}/SHA256SUMS" || true
  info "Migration snapshot: ${MIGRATION_SNAPSHOT}"
}

copy_migration_config() {
  install -d -m 0750 "${TARGET_CONFIG_DIR}" "${TARGET_DATA_DIR}" "${TARGET_LOG_DIR}"
  cp -a "${LEGACY_CONFIG_DIR}/." "${TARGET_CONFIG_DIR}/"
  rm -f "${TARGET_CONFIG_DIR}/ssh.env"
  if [[ ! -f "${TARGET_CONFIG_DIR}/inventory.ini" ]]; then
    cat > "${TARGET_CONFIG_DIR}/inventory.ini" <<'EOF'
[v2bx_nodes]
# parent-01 ansible_host=CHANGE_ME ansible_port=22 ansible_user=root ansible_ssh_pass=CHANGE_ME node_id=CHANGE_ME v2bx_arch=amd64

[role_overseas_exit]
# parent-01

[role_cn_dedicated_nftables]
# parent-01

[role_cn_standard_agent]
# parent-01

[v2bx_nodes:children]
role_overseas_exit
role_cn_standard_agent

[forward_nodes:children]
role_cn_dedicated_nftables
EOF
  fi
  # Resolve the historical relative SQLite path against the new data root.
  if [[ -f "${TARGET_CONFIG_DIR}/config.yaml" ]]; then
    sed -i -E "s#(^[[:space:]]+database:[[:space:]]*)['\"]?data(/[^'\"]*)?['\"]?#\\1${TARGET_DATA_DIR}/v2board.db#" "${TARGET_CONFIG_DIR}/config.yaml" || true
    sed -i -E "s#(^[[:space:]]+path:[[:space:]]*)['\"]?public['\"]?#\\1${INSTALL_DIR}/web/public#" "${TARGET_CONFIG_DIR}/config.yaml" || true
    sed -i "s#${LEGACY_DATA_DIR}/frontend#${INSTALL_DIR}/web/public#g" "${TARGET_CONFIG_DIR}/config.yaml" || true
    copy_tls_asset "${TARGET_CONFIG_DIR}/config.yaml" tls_cert_file control-cert.pem
    copy_tls_asset "${TARGET_CONFIG_DIR}/config.yaml" tls_key_file control-key.pem
    copy_tls_asset "${TARGET_CONFIG_DIR}/config.yaml" cert_file control-cert.pem
    copy_tls_asset "${TARGET_CONFIG_DIR}/config.yaml" key_file control-key.pem
  fi
  if [[ -f "${LEGACY_DATA_DIR}/v2board.db" ]]; then
    install -m 0640 "${LEGACY_DATA_DIR}/v2board.db" "${TARGET_DATA_DIR}/v2board.db"
    [[ "$(file_sha256 "${LEGACY_DATA_DIR}/v2board.db")" == "$(file_sha256 "${TARGET_DATA_DIR}/v2board.db")" ]] || die "SQLite copy checksum mismatch"
  fi
  chown -R "${APP_USER}:${APP_USER}" "${TARGET_DATA_DIR}" "${TARGET_LOG_DIR}" 2>/dev/null || true
  chown root:"${APP_USER}" "${TARGET_CONFIG_DIR}" "${TARGET_CONFIG_DIR}/config.yaml" "${TARGET_CONFIG_DIR}/inventory.ini"
  chmod 0750 "${TARGET_CONFIG_DIR}"
  chmod 0640 "${TARGET_CONFIG_DIR}/config.yaml"
  chmod 0640 "${TARGET_CONFIG_DIR}/inventory.ini"
}

copy_tls_asset() {
  local config="$1" key="$2" name="$3" source target
  source="$(sed -n -E "s#^[[:space:]]+${key}:[[:space:]]*['\"]?([^'\"]+)['\"]?.*#\1#p" "${config}" | head -n1)"
  [[ "${source}" == /etc/letsencrypt/* && -r "${source}" ]] || return 0
  target="${TARGET_CONFIG_DIR}/${name}"
  cp -L "${source}" "${target}"
  chown root:"${APP_USER}" "${target}"
  chmod 0640 "${target}"
  sed -i "s#${source}#${target}#g" "${config}"
}

write_migration_unit() {
  write_systemd_unit
  # Migration config/data live outside the program root and must be writable.
  sed -i "s#ReadWritePaths=.*#ReadWritePaths=${INSTALL_DIR} ${TARGET_CONFIG_DIR} ${TARGET_DATA_DIR} ${TARGET_LOG_DIR}#" "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload
}

migrate_legacy_layout() {
  [[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]] || die "migrate requires --version <release tag>"
  legacy_layout_present || die "Expected legacy layout was not found"
  BINARY_PATH="${INSTALL_DIR}/bin/anix-control"
  LEGACY_BINARY_PATH="${INSTALL_DIR}/bin/v2board"
  CONFIG_FILE="${TARGET_CONFIG_DIR}/config.yaml"
  FRONTEND_DIR="${INSTALL_DIR}/web/public"
  BACKUP_ROOT="${MIGRATION_BACKUP_DIR}"
  VERSION_FILE="${INSTALL_DIR}/.release-version"
  TMP_DIR="$(mktemp -d)"
  install_base_tools
  ensure_app_user
  install -d -m 0750 -o "${APP_USER}" -g "${APP_USER}" "${INSTALL_DIR}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/web" "${INSTALL_DIR}/config/data" "${INSTALL_DIR}/logs" "${TARGET_CONFIG_DIR}" "${TARGET_DATA_DIR}" "${TARGET_LOG_DIR}"
  # Download and verify everything before touching the running legacy service.
  local binary_archive; binary_archive="$(asset_name)"
  download_asset "SHA256SUMS.txt"; download_asset "${binary_archive}"; download_asset "anix-control-frontend.tar.gz"
  verify_asset "${binary_archive}"; verify_asset "anix-control-frontend.tar.gz"
  snapshot_migration_state
  stop_legacy_service
  copy_migration_config
  install_release_files "${binary_archive}"
  write_migration_unit
  # Existing installations commonly use a non-default HTTP port.
  local migrated_port
  migrated_port="$(sed -n -E '/^server:/,/^[^[:space:]]/ s/^[[:space:]]+port:[[:space:]]*([0-9]+).*/\1/p' "${CONFIG_FILE}" | head -n1)"
  [[ "${migrated_port}" =~ ^[0-9]+$ ]] && HEALTH_URL="http://127.0.0.1:${migrated_port}/health"
  if [[ "${SKIP_START}" -eq 1 ]]; then return 0; fi
  if ! start_and_verify; then
    restore_migration_on_error
    die "Migration failed health verification; legacy service was restored"
  fi
  systemctl disable v2board.service >/dev/null 2>&1 || true
  info "Legacy layout migrated successfully; v2board.service is disabled"
}

restore_migration_on_error() {
  systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
  [[ -n "${MIGRATION_SNAPSHOT}" && -f "${MIGRATION_SNAPSHOT}/v2board.service" ]] && cp -a "${MIGRATION_SNAPSHOT}/v2board.service" "${LEGACY_SERVICE_FILE}"
  [[ -n "${MIGRATION_SNAPSHOT}" && -f "${MIGRATION_SNAPSHOT}/v2board" ]] && install -m 0755 "${MIGRATION_SNAPSHOT}/v2board" "${LEGACY_PROGRAM_DIR}/v2board"
  if [[ -n "${MIGRATION_SNAPSHOT}" && -d "${MIGRATION_SNAPSHOT}/public" ]]; then
    rm -rf "${LEGACY_PROGRAM_DIR}/public"
    cp -a "${MIGRATION_SNAPSHOT}/public" "${LEGACY_PROGRAM_DIR}/public"
  fi
  systemctl daemon-reload >/dev/null 2>&1 || true
  systemctl enable v2board.service >/dev/null 2>&1 || true
  systemctl start v2board.service >/dev/null 2>&1 || true
}

rollback_migration() {
  local snapshot="${1:-}"
  [[ -n "${snapshot}" && -d "${snapshot}" ]] || snapshot="$(find "${MIGRATION_BACKUP_DIR}" -mindepth 1 -maxdepth 1 -type d -name '*-migration' -print 2>/dev/null | sort | tail -n 1)"
  [[ -n "${snapshot}" && -d "${snapshot}" ]] || die "No migration snapshot found"
  systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
  [[ -f "${snapshot}/v2board.service" ]] && cp -a "${snapshot}/v2board.service" "${LEGACY_SERVICE_FILE}"
  [[ -f "${snapshot}/v2board" ]] && install -m 0755 "${snapshot}/v2board" "${LEGACY_PROGRAM_DIR}/v2board"
  systemctl daemon-reload; systemctl enable v2board.service >/dev/null; systemctl start v2board.service
  info "Rolled back legacy service from ${snapshot}"
}

backup_current_release() {
  [[ -x "${BINARY_PATH}" || -x "${LEGACY_BINARY_PATH}" || -d "${FRONTEND_DIR}" ]] || return
  BACKUP_DIR="${BACKUP_ROOT}/$(date -u +%Y%m%dT%H%M%SZ)-${VERSION}"
  install -d -m 0700 -o root -g root "${BACKUP_DIR}"
  if [[ -x "${BINARY_PATH}" ]]; then
    cp -a "${BINARY_PATH}" "${BACKUP_DIR}/anix-control"
  elif [[ -x "${LEGACY_BINARY_PATH}" ]]; then
    cp -aL "${LEGACY_BINARY_PATH}" "${BACKUP_DIR}/anix-control"
  fi
  [[ -d "${FRONTEND_DIR}" ]] && cp -a "${FRONTEND_DIR}" "${BACKUP_DIR}/frontend"
  [[ -f "${VERSION_FILE}" ]] && cp -a "${VERSION_FILE}" "${BACKUP_DIR}/release-version"
  info "Backed up the previous release to ${BACKUP_DIR}"
}

stop_running_service() {
  if systemctl is-active --quiet "${SERVICE_NAME}"; then
    WAS_ACTIVE=1
    systemctl stop "${SERVICE_NAME}"
  fi
}

stop_legacy_service() {
  if systemctl is-active --quiet v2board.service 2>/dev/null; then
    systemctl stop v2board.service
  fi
}

install_release_files() {
  local binary_archive="$1"
  local binary_name="${binary_archive%.tar.gz}"
  local stage="${TMP_DIR}/stage"

  install -d "${stage}/bin" "${stage}/web/public"
  tar -xzf "${TMP_DIR}/${binary_archive}" -C "${stage}/bin"
  [[ -f "${stage}/bin/${binary_name}" ]] || die "Release archive does not contain ${binary_name}"
  tar -xzf "${TMP_DIR}/anix-control-frontend.tar.gz" -C "${stage}/web/public"

  install -m 0755 "${stage}/bin/${binary_name}" "${BINARY_PATH}.new"
  mv -f "${BINARY_PATH}.new" "${BINARY_PATH}"
  rm -f "${LEGACY_BINARY_PATH}"
  ln -s "$(basename "${BINARY_PATH}")" "${LEGACY_BINARY_PATH}"
  rm -rf "${FRONTEND_DIR}.new"
  mv "${stage}/web/public" "${FRONTEND_DIR}.new"
  rm -rf "${FRONTEND_DIR}"
  mv "${FRONTEND_DIR}.new" "${FRONTEND_DIR}"
  chown -R "${APP_USER}:${APP_USER}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/web" "${INSTALL_DIR}/config/data" "${INSTALL_DIR}/logs"
  printf '%s\n' "${VERSION}" > "${VERSION_FILE}"
  chmod 0644 "${VERSION_FILE}"
}

write_systemd_unit() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=AnixOps Control
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_USER}
WorkingDirectory=${INSTALL_DIR}
Environment=HOME=${INSTALL_DIR}
ExecStart=${BINARY_PATH} -config ${CONFIG_FILE}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ReadWritePaths=${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}" >/dev/null
}

restore_backup() {
  [[ -n "${BACKUP_DIR}" && -d "${BACKUP_DIR}" ]] || return
  warn "Restoring the previous release from ${BACKUP_DIR}"
  if [[ -f "${BACKUP_DIR}/anix-control" ]]; then
    install -m 0755 "${BACKUP_DIR}/anix-control" "${BINARY_PATH}"
    rm -f "${LEGACY_BINARY_PATH}"
    ln -s "$(basename "${BINARY_PATH}")" "${LEGACY_BINARY_PATH}"
  fi
  if [[ -d "${BACKUP_DIR}/frontend" ]]; then
    rm -rf "${FRONTEND_DIR}"
    cp -a "${BACKUP_DIR}/frontend" "${FRONTEND_DIR}"
  fi
  [[ -f "${BACKUP_DIR}/release-version" ]] && cp -a "${BACKUP_DIR}/release-version" "${VERSION_FILE}"
  chown -R "${APP_USER}:${APP_USER}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/web" || true
}

start_and_verify() {
  local attempt
  systemctl restart "${SERVICE_NAME}"
  for attempt in $(seq 1 30); do
    if systemctl is-active --quiet "${SERVICE_NAME}" && curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      info "Service is healthy: ${HEALTH_URL}"
      return 0
    fi
    sleep 2
  done
  systemctl --no-pager --full status "${SERVICE_NAME}" || true
  return 1
}

parse_args() {
  if [[ "${1:-}" == "install" || "${1:-}" == "update" || "${1:-}" == "rollback" || "${1:-}" == "preflight" || "${1:-}" == "migrate" ]]; then
    COMMAND="$1"
    shift
  fi

  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --version) VERSION="${2:-}"; shift 2 ;;
      --admin-email) ADMIN_EMAIL="${2:-}"; shift 2 ;;
      --admin-password) ADMIN_PASSWORD="${2:-}"; shift 2 ;;
      --install-dir)
        INSTALL_DIR="${2:-}"
        INSTALL_DIR_EXPLICIT=1
        shift 2
        ;;
      --health-url) HEALTH_URL="${2:-}"; shift 2 ;;
      --plan) MIGRATION_PLAN_PATH="${2:-}"; shift 2 ;;
      --skip-start) SKIP_START=1; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "Unknown argument: $1" ;;
    esac
  done
}

main() {
  parse_args "$@"
  if [[ "${COMMAND}" == "preflight" ]]; then
    preflight_migration
    exit 0
  fi
  need_root
  if [[ "${COMMAND}" == "migrate" ]]; then
    [[ -n "${VERSION}" ]] || die "migrate requires --version <release tag>"
    migrate_legacy_layout
    exit 0
  fi
  if [[ "${COMMAND}" == "rollback" && "${VERSION}" == "" ]]; then
    rollback_migration
    exit 0
  fi
  detect_legacy_layout
  validate_install_dir
  install_base_tools
  [[ -n "${VERSION}" ]] || VERSION="$(latest_release)"
  validate_version

  BINARY_PATH="${INSTALL_DIR}/bin/anix-control"
  LEGACY_BINARY_PATH="${INSTALL_DIR}/bin/v2board"
  CONFIG_FILE="${INSTALL_DIR}/config/config.yaml"
  FRONTEND_DIR="${INSTALL_DIR}/web/public"
  BACKUP_ROOT="${INSTALL_DIR}/backups"
  VERSION_FILE="${INSTALL_DIR}/.release-version"
  TMP_DIR="$(mktemp -d)"

  ensure_app_user
  ensure_layout_and_config

  local binary_archive
  binary_archive="$(asset_name)"
  download_asset "SHA256SUMS.txt"
  download_asset "${binary_archive}"
  download_asset "anix-control-frontend.tar.gz"
  verify_asset "${binary_archive}"
  verify_asset "anix-control-frontend.tar.gz"

  stop_running_service
  backup_current_release
  install_release_files "${binary_archive}"
  write_systemd_unit

  if [[ "${SKIP_START}" -eq 1 ]]; then
    info "Installed ${VERSION}; service start was skipped."
    exit 0
  fi

  if ! start_and_verify; then
    if [[ -n "${BACKUP_DIR}" ]]; then
      restore_backup
      systemctl restart "${SERVICE_NAME}" || true
    fi
    die "Installation failed health verification. Inspect: journalctl -u ${SERVICE_NAME} -n 200 --no-pager"
  fi

  info "Installed ${VERSION} successfully."
  [[ "${COMMAND}" == "rollback" ]] && info "Rollback completed by installing the requested release tag."
}

main "$@"
