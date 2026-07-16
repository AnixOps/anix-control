#!/usr/bin/env bash
# In-place upgrade for the legacy native panel layout discovered on pl-1:
# /usr/local/v2board/v2board, /etc/v2board/config.yaml, and
# /var/lib/v2board/frontend. It deliberately leaves the service unit and
# configuration unchanged.

set -Eeuo pipefail

readonly VERSION="${VERSION:-v2.5.0}"
readonly SERVICE_NAME="v2board"
readonly APP_BIN="/usr/local/v2board/v2board"
readonly CONFIG_FILE="/etc/v2board/config.yaml"
readonly UNIT_FILE="/etc/systemd/system/v2board.service"
readonly FRONTEND_DIR="/var/lib/v2board/frontend"
readonly HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:18080/health}"
readonly RELEASE_URL="https://github.com/AnixOps/v2board_AnixOps/releases/download/${VERSION}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
readonly TIMESTAMP
readonly BACKUP_DIR="/root/v2board-backup-${TIMESTAMP}"
WORK_DIR="$(mktemp -d)"
readonly WORK_DIR
readonly OLD_FRONTEND_DIR="${FRONTEND_DIR}.pre-${VERSION}-${TIMESTAMP}"
readonly ROLLBACK_SCRIPT="${BACKUP_DIR}/ROLLBACK.sh"

service_stopped=0
files_replaced=0

info() { printf '[INFO] %s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

cleanup() {
  rm -rf "${WORK_DIR}"
}

rollback_on_error() {
  local status=$?
  trap - ERR
  if [[ "${service_stopped}" -eq 1 ]]; then
    if [[ "${files_replaced}" -eq 1 && -x "${ROLLBACK_SCRIPT}" ]]; then
      warn "Upgrade failed; restoring the previous binary and frontend."
      bash "${ROLLBACK_SCRIPT}" || true
    else
      warn "Upgrade stopped before file replacement; starting the existing service."
      systemctl start "${SERVICE_NAME}" || true
    fi
  fi
  exit "${status}"
}

trap cleanup EXIT
trap rollback_on_error ERR

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

validate_version() {
  [[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]] || \
    die "Invalid VERSION: ${VERSION}"
}

write_rollback_script() {
  cat >"${ROLLBACK_SCRIPT}" <<EOF
#!/usr/bin/env bash
set -Eeuo pipefail
systemctl stop "${SERVICE_NAME}" || true
install -m 0755 "${BACKUP_DIR}/v2board.previous" "${APP_BIN}"
if [[ -d "${OLD_FRONTEND_DIR}" ]]; then
  rm -rf "${FRONTEND_DIR}"
  mv "${OLD_FRONTEND_DIR}" "${FRONTEND_DIR}"
fi
systemctl start "${SERVICE_NAME}"
curl -fsS "${HEALTH_URL}"
echo
EOF
  chmod 0700 "${ROLLBACK_SCRIPT}"
}

main() {
  [[ "${EUID}" -eq 0 ]] || die "Run as root."
  validate_version

  local command
  for command in curl tar sha256sum pg_dump pg_restore runuser systemctl; do
    require_command "${command}"
  done

  test -x "${APP_BIN}" || die "Panel binary not found: ${APP_BIN}"
  test -f "${CONFIG_FILE}" || die "Panel config not found: ${CONFIG_FILE}"
  test -f "${UNIT_FILE}" || die "Systemd unit not found: ${UNIT_FILE}"
  test -d "${FRONTEND_DIR}" || die "Frontend directory not found: ${FRONTEND_DIR}"
  id postgres >/dev/null 2>&1 || die "The local postgres OS user is required for the backup."
  systemctl is-active --quiet "${SERVICE_NAME}" || die "${SERVICE_NAME} is not active; fix the current deployment before upgrading."
  curl -fsS --max-time 5 "${HEALTH_URL}" >/dev/null || die "Current health check failed: ${HEALTH_URL}"

  install -d -m 0700 "${BACKUP_DIR}"

  info "Backing up PostgreSQL database."
  runuser -u postgres -- pg_dump -Fc -d v2board >"${BACKUP_DIR}/v2board.postgres.dump"
  pg_restore -l "${BACKUP_DIR}/v2board.postgres.dump" >/dev/null
  sha256sum "${BACKUP_DIR}/v2board.postgres.dump" >"${BACKUP_DIR}/v2board.postgres.dump.sha256"

  info "Backing up binary, frontend, config, and systemd unit."
  install -m 0755 "${APP_BIN}" "${BACKUP_DIR}/v2board.previous"
  tar -C / -czf "${BACKUP_DIR}/runtime-files.tar.gz" \
    etc/v2board/config.yaml \
    etc/systemd/system/v2board.service \
    usr/local/v2board/v2board \
    var/lib/v2board/frontend
  sha256sum "${BACKUP_DIR}/runtime-files.tar.gz" >"${BACKUP_DIR}/runtime-files.tar.gz.sha256"
  write_rollback_script

  info "Downloading GitHub Release assets for ${VERSION}."
  cd "${WORK_DIR}"
  curl -fL --retry 3 --retry-delay 2 -o SHA256SUMS.txt "${RELEASE_URL}/SHA256SUMS.txt"
  curl -fL --retry 3 --retry-delay 2 -o v2board-linux-amd64.tar.gz \
    "${RELEASE_URL}/v2board-linux-amd64.tar.gz"
  curl -fL --retry 3 --retry-delay 2 -o v2board-frontend.tar.gz \
    "${RELEASE_URL}/v2board-frontend.tar.gz"
  awk '$2 == "v2board-linux-amd64.tar.gz" || $2 == "v2board-frontend.tar.gz"' \
    SHA256SUMS.txt > artifact-checksums.txt
  [[ "$(wc -l < artifact-checksums.txt)" -eq 2 ]] || \
    die "SHA256SUMS.txt does not contain both required release artifacts."
  sha256sum -c artifact-checksums.txt

  mkdir -p "${WORK_DIR}/frontend"
  tar -xzf v2board-linux-amd64.tar.gz
  tar -xzf v2board-frontend.tar.gz -C "${WORK_DIR}/frontend"
  test -x "${WORK_DIR}/v2board-linux-amd64" || die "Release archive has no linux amd64 binary."
  test -f "${WORK_DIR}/frontend/index.html" || die "Release archive has no frontend index.html."

  info "Stopping ${SERVICE_NAME} and replacing release files."
  systemctl stop "${SERVICE_NAME}"
  service_stopped=1
  files_replaced=1
  install -m 0755 "${WORK_DIR}/v2board-linux-amd64" "${APP_BIN}.new"
  mv -f "${APP_BIN}.new" "${APP_BIN}"
  mv "${FRONTEND_DIR}" "${OLD_FRONTEND_DIR}"
  mv "${WORK_DIR}/frontend" "${FRONTEND_DIR}"

  info "Starting ${SERVICE_NAME}."
  systemctl start "${SERVICE_NAME}"
  local healthy=0
  local attempt=1
  while (( attempt <= 30 )); do
    if curl -fsS --max-time 5 "${HEALTH_URL}" >"${BACKUP_DIR}/health-after-upgrade.json"; then
      healthy=1
      break
    fi
    sleep 2
    attempt=$((attempt + 1))
  done
  [[ "${healthy}" -eq 1 ]] || {
    systemctl --no-pager --full status "${SERVICE_NAME}" || true
    tail -n 100 /var/lib/v2board/logs/v2board-error.log || true
    false
  }
  systemctl is-active --quiet "${SERVICE_NAME}"

  local frontend_status
  frontend_status="$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:13000/ || true)"
  if [[ ! "${frontend_status}" =~ ^(200|301|302)$ ]]; then
    warn "Frontend port 13000 returned HTTP ${frontend_status}; inspect the reverse proxy and frontend path."
  fi

  trap - ERR
  info "Upgrade completed successfully."
  info "Backup directory: ${BACKUP_DIR}"
  info "Manual rollback: bash ${ROLLBACK_SCRIPT}"
  info "Keep the PostgreSQL dump and old frontend directory for at least 7 days."
}

main "$@"
