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
  install.sh [install|update|rollback] [options]

Options:
  --version <tag>          Release tag, for example v3.0.0-alpha.1. Defaults to GitHub's latest stable release.
  --admin-email <email>    Bootstrap admin email on a fresh installation.
  --admin-password <text>  Bootstrap admin password on a fresh installation.
  --install-dir <path>     Installation root. Default: /opt/anixops/control.
  --health-url <url>       Health endpoint to verify after start.
  --skip-start             Install files and systemd unit without starting the service.
  -h, --help               Show this help.

Only GitHub Release assets are downloaded. The installer never clones this
repository and never performs Go or frontend builds on the target host.
EOF
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
  if [[ "${1:-}" == "install" || "${1:-}" == "update" || "${1:-}" == "rollback" ]]; then
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
      --skip-start) SKIP_START=1; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "Unknown argument: $1" ;;
    esac
  done
}

main() {
  parse_args "$@"
  need_root
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
