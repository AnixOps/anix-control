#!/usr/bin/env bash
# Build and deploy the v2board panel binary to production.
#
# Usage: ./config/deploy/deploy_panel.sh
#
# Steps: build -> backup current binary -> stop service -> install new
# binary -> restart service -> verify it's listening and the gRPC server
# came up. Run from the repo root, or anywhere (paths are resolved
# relative to this script).
#
# Run as root, or as a user with sudo access.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICE_NAME="${SERVICE_NAME:-v2board.service}"
INSTALL_PATH="${INSTALL_PATH:-}"
CONFIG_PATH="${CONFIG_PATH:-}"
WORKING_DIR="${WORKING_DIR:-}"
BUILD_OUTPUT="${REPO_ROOT}/v2board"
REPO_UID="$(stat -c %u "${REPO_ROOT}")"
REPO_GID="$(stat -c %g "${REPO_ROOT}")"
DEPLOY_STOPPED=0
DEPLOY_DONE=0
BACKUP_PATH=""

if [[ "${EUID}" -eq 0 ]]; then
  SUDO=()
else
  SUDO=(sudo)
fi

restore_repo_ownership() {
  if [[ "${EUID}" -eq 0 ]]; then
    chown -R "${REPO_UID}:${REPO_GID}" "${REPO_ROOT}/web/public" 2>/dev/null || true
    if [[ -e "${BUILD_OUTPUT}" ]]; then
      chown "${REPO_UID}:${REPO_GID}" "${BUILD_OUTPUT}" 2>/dev/null || true
    fi
  fi
}

prepare_repo_build_ownership() {
  if [[ "${EUID}" -eq 0 && -d "${REPO_ROOT}/web/public" ]]; then
    chown -R "${REPO_UID}:${REPO_GID}" "${REPO_ROOT}/web/public" 2>/dev/null || true
  fi
}

ensure_frontend_outdir_writable() {
  local outdir="${REPO_ROOT}/web/public"
  if [[ ! -e "${outdir}" ]]; then
    return 0
  fi
  if [[ -d "${outdir}/assets" && ! -w "${outdir}/assets" ]]; then
    echo "!! ${outdir}/assets is not writable by $(id -un)."
    echo "!! Fix once with: sudo chown -R ${REPO_UID}:${REPO_GID} '${outdir}'"
    return 1
  fi
  if [[ ! -w "${outdir}" ]]; then
    echo "!! ${outdir} is not writable by $(id -un)."
    echo "!! Fix once with: sudo chown -R ${REPO_UID}:${REPO_GID} '${outdir}'"
    return 1
  fi
}

rollback_deployment() {
  if [[ "${DEPLOY_STOPPED}" -ne 1 || "${DEPLOY_DONE}" -eq 1 ]]; then
    return
  fi

  echo "!! Deployment failed after stopping ${SERVICE_NAME}; attempting rollback"
  if [[ -n "${BACKUP_PATH}" && -e "${BACKUP_PATH}" ]]; then
    "${SUDO[@]}" cp "${BACKUP_PATH}" "${INSTALL_PATH}" || true
    "${SUDO[@]}" chmod 755 "${INSTALL_PATH}" || true
    "${SUDO[@]}" systemctl restart "${SERVICE_NAME}" || true
    echo "!! Rolled back to ${BACKUP_PATH}"
  else
    "${SUDO[@]}" systemctl start "${SERVICE_NAME}" || true
    echo "!! No previous binary exists for rollback"
  fi
  echo "!! Check logs: sudo journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
}

cleanup() {
  local exit_code=$?
  if [[ "${exit_code}" -ne 0 ]]; then
    rollback_deployment
  fi
  restore_repo_ownership
  exit "${exit_code}"
}

command_available() {
  local candidate="$1"
  if [[ "${candidate}" == */* ]]; then
    [[ -x "${candidate}" ]]
  else
    command -v "${candidate}" >/dev/null 2>&1
  fi
}

prepend_command_dir() {
  local candidate="$1"
  if [[ "${candidate}" == */* ]]; then
    export PATH="$(dirname "${candidate}"):${PATH}"
  fi
}

node_version_supported() {
  local raw="$1"
  local major minor patch
  if [[ ! "${raw}" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    return 1
  fi
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  # Vite 8 requires Node ^20.19.0 or >=22.12.0.
  if (( major == 20 && minor >= 19 )); then
    return 0
  fi
  if (( major == 22 && minor >= 12 )); then
    return 0
  fi
  if (( major > 22 )); then
    return 0
  fi
  return 1
}

ensure_node_version_supported() {
  local version
  [[ -n "${NODE_BIN:-}" ]] || return 0
  version="$("${NODE_BIN}" --version)"
  if ! node_version_supported "${version}"; then
    echo "!! Node ${version} is too old for the frontend build."
    echo "!! Required: Node 20.19+ or 22.12+. Set NODE_BIN=/path/to/node or update /home/dev/.local/opt/node."
    return 1
  fi
}

detect_systemd_paths() {
  local exec_start detected_install detected_config detected_workdir

  exec_start=$("${SUDO[@]}" systemctl show "${SERVICE_NAME}" -p ExecStart --value 2>/dev/null || true)
  detected_install=$(sed -n 's/.*path=\([^ ;}]*\).*/\1/p' <<<"${exec_start}" | head -n1)
  detected_config=$(sed -n 's/.*-config[= ]\([^ ;}]*\).*/\1/p' <<<"${exec_start}" | head -n1)
  detected_workdir=$("${SUDO[@]}" systemctl show "${SERVICE_NAME}" -p WorkingDirectory --value 2>/dev/null || true)

  if [[ -z "${INSTALL_PATH}" && -n "${detected_install}" ]]; then
    INSTALL_PATH="${detected_install}"
  fi
  if [[ -z "${CONFIG_PATH}" && -n "${detected_config}" ]]; then
    CONFIG_PATH="${detected_config}"
  fi
  if [[ -z "${WORKING_DIR}" && -n "${detected_workdir}" ]]; then
    WORKING_DIR="${detected_workdir}"
  fi

  INSTALL_PATH="${INSTALL_PATH:-/usr/local/v2board/v2board}"
  CONFIG_PATH="${CONFIG_PATH:-/etc/v2board/config.yaml}"
  WORKING_DIR="${WORKING_DIR:-$(dirname "${CONFIG_PATH}")}"
}

read_config_value() {
  local section="$1"
  local key="$2"
  local fallback="$3"
  local value

  value=$("${SUDO[@]}" awk -v section="${section}" -v key="${key}" '
    $0 ~ "^[[:space:]]*" section ":" { in_section = 1; next }
    in_section && $0 ~ "^[[:alnum:]_]+:" { exit }
    in_section && $1 == key ":" { print $2; exit }
  ' "${CONFIG_PATH}" 2>/dev/null || true)
  value="${value%%#*}"
  value="${value//\"/}"
  value="${value//\'/}"
  value="${value//$'\r'/}"

  if [[ -z "${value}" ]]; then
    value="${fallback}"
  fi
  printf '%s\n' "${value}"
}

resolve_runtime_path() {
  local raw_path="$1"
  local config_dir install_dir candidate
  local candidates=()

  if [[ -z "${raw_path}" ]]; then
    return
  fi
  if [[ "${raw_path}" == /* ]]; then
    printf '%s\n' "${raw_path}"
    return
  fi

  config_dir="$(dirname "${CONFIG_PATH}")"
  install_dir="$(dirname "${INSTALL_PATH}")"
  if [[ "$(basename "${config_dir}")" == "config" ]]; then
    candidates+=("$(dirname "${config_dir}")/${raw_path}")
  fi
  candidates+=("${config_dir}/${raw_path}")
  candidates+=("${install_dir}/${raw_path}")
  candidates+=("${WORKING_DIR}/${raw_path}")

  for candidate in "${candidates[@]}"; do
    if [[ -e "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return
    fi
  done
  printf '%s\n' "${candidates[0]}"
}

append_frontend_dir() {
  local dir="$1"
  local existing
  [[ -n "${dir}" ]] || return
  for existing in "${FRONTEND_DIRS[@]:-}"; do
    if [[ "${existing}" == "${dir}" ]]; then
      return
    fi
  done
  FRONTEND_DIRS+=("${dir}")
}

sync_frontend_dir() {
  local frontend_dir="$1"
  echo "    -> ${frontend_dir}"
  "${SUDO[@]}" mkdir -p "${frontend_dir}"
  if command -v rsync >/dev/null 2>&1; then
    "${SUDO[@]}" rsync -a --delete "${REPO_ROOT}/web/public/" "${frontend_dir}/"
  else
    "${SUDO[@]}" find "${frontend_dir}" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
    "${SUDO[@]}" cp -a "${REPO_ROOT}/web/public/." "${frontend_dir}/"
  fi
  "${SUDO[@]}" find "${frontend_dir}" -type d -exec chmod 755 {} +
  "${SUDO[@]}" find "${frontend_dir}" -type f -exec chmod 644 {} +
}

check_port() {
  local label="$1"
  local port="$2"
  if ! command -v ss >/dev/null 2>&1; then
    echo "    ss not found; skipped ${label} port check"
    return
  fi
  if ss -tlnp 2>/dev/null | grep -q ":${port} "; then
    echo "    ${label} port ${port} listening"
  else
    echo "!! ${label} port ${port} not listening"
    return 1
  fi
}

assert_self_test_equal() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "self-test failed: ${label}: got '${actual}', want '${expected}'" >&2
    return 1
  fi
}

run_self_test() {
  local tmpdir fakebin original_path original_repo_root
  tmpdir="$(mktemp -d)"
  fakebin="${tmpdir}/bin"
  original_path="${PATH}"
  original_repo_root="${REPO_ROOT}"
  mkdir -p "${fakebin}"

  cat >"${fakebin}/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "show" && "$3" == "-p" && "$4" == "ExecStart" ]]; then
  echo "{ path=/opt/anix/v2board ; argv[]=/opt/anix/v2board -config /opt/anix/config/config.yaml ; ignore_errors=no }"
  exit 0
fi
if [[ "$1" == "show" && "$3" == "-p" && "$4" == "WorkingDirectory" ]]; then
  echo "/srv/v2board"
  exit 0
fi
exit 1
EOF
  chmod +x "${fakebin}/systemctl"

  SUDO=()
  PATH="${fakebin}:${PATH}"
  SERVICE_NAME="v2board.service"
  INSTALL_PATH=""
  CONFIG_PATH=""
  WORKING_DIR=""
  detect_systemd_paths
  assert_self_test_equal "${INSTALL_PATH}" "/opt/anix/v2board" "detected install path"
  assert_self_test_equal "${CONFIG_PATH}" "/opt/anix/config/config.yaml" "detected config path"
  assert_self_test_equal "${WORKING_DIR}" "/srv/v2board" "detected working directory"

  mkdir -p "${tmpdir}/panel/config" "${tmpdir}/panel/web/public" "${tmpdir}/runtime"
  CONFIG_PATH="${tmpdir}/panel/config/config.yaml"
  INSTALL_PATH="${tmpdir}/install/v2board"
  WORKING_DIR="${tmpdir}/runtime"
  cat >"${CONFIG_PATH}" <<'EOF'
server:
  port: 18080
frontend:
  enable: true
  port: 13000
  path: "web/public"
grpc:
  enabled: true
  port: 50051
EOF

  assert_self_test_equal "$(read_config_value "server" "port" "8080")" "18080" "server.port"
  assert_self_test_equal "$(read_config_value "frontend" "enable" "false")" "true" "frontend.enable"
  assert_self_test_equal "$(read_config_value "frontend" "path" "missing")" "web/public" "frontend.path"
  assert_self_test_equal "$(read_config_value "grpc" "enabled" "false")" "true" "grpc.enabled"
  assert_self_test_equal "$(read_config_value "missing" "key" "fallback")" "fallback" "missing fallback"
  assert_self_test_equal "$(resolve_runtime_path "web/public")" "${tmpdir}/panel/web/public" "relative runtime path"
  assert_self_test_equal "$(resolve_runtime_path "/absolute/public")" "/absolute/public" "absolute runtime path"

  FRONTEND_DIRS=()
  append_frontend_dir "${tmpdir}/panel/web/public"
  append_frontend_dir "${tmpdir}/panel/web/public"
  append_frontend_dir "${tmpdir}/runtime/web/public"
  assert_self_test_equal "${#FRONTEND_DIRS[@]}" "2" "frontend dir de-duplication"

  mkdir -p "${tmpdir}/node/bin"
  cat >"${tmpdir}/node/bin/node" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == */npm ]]; then
  echo "fake-npm-via-env-node"
else
  echo "fake-node"
fi
EOF
  cat >"${tmpdir}/node/bin/npm" <<'EOF'
#!/usr/bin/env node
EOF
  chmod +x "${tmpdir}/node/bin/node"
  chmod +x "${tmpdir}/node/bin/npm"
  PATH="/usr/bin:/bin"
  prepend_command_dir "${tmpdir}/node/bin/npm"
  assert_self_test_equal "$(command -v node)" "${tmpdir}/node/bin/node" "NPM_BIN directory exposes node"
  assert_self_test_equal "$("${tmpdir}/node/bin/npm" --version)" "fake-npm-via-env-node" "npm shebang resolves node"
  assert_self_test_equal "$(node_version_supported "v20.19.0"; echo $?)" "0" "node 20.19 supported"
  assert_self_test_equal "$(node_version_supported "v22.12.0"; echo $?)" "0" "node 22.12 supported"
  assert_self_test_equal "$(node_version_supported "v24.0.0"; echo $?)" "0" "node 24 supported"
  assert_self_test_equal "$(node_version_supported "v20.18.1"; echo $?)" "1" "node 20.18 rejected"
  assert_self_test_equal "$(node_version_supported "v21.9.0"; echo $?)" "1" "node 21 rejected"
  assert_self_test_equal "$(node_version_supported "v22.11.0"; echo $?)" "1" "node 22.11 rejected"

  REPO_ROOT="${tmpdir}/repo"
  mkdir -p "${REPO_ROOT}/web/public/assets"
  if [[ "${EUID}" -ne 0 ]]; then
    chmod 555 "${REPO_ROOT}/web/public/assets"
    if ensure_frontend_outdir_writable >/dev/null 2>&1; then
      echo "self-test failed: unwritable frontend assets should be rejected" >&2
      return 1
    fi
  fi
  chmod 755 "${REPO_ROOT}/web/public/assets"
  assert_self_test_equal "$(ensure_frontend_outdir_writable >/dev/null 2>&1; echo $?)" "0" "writable frontend outdir"
  REPO_ROOT="${original_repo_root}"

  PATH="${original_path}"
  rm -rf "${tmpdir}"
  echo "deploy_panel.sh self-test passed"
}

case "${1:-}" in
  --self-test)
    run_self_test
    exit 0
    ;;
  -h|--help)
    echo "Usage: $0 [--self-test]"
    exit 0
    ;;
esac

trap cleanup EXIT

detect_systemd_paths
prepare_repo_build_ownership

if [[ -n "${NPM_BIN:-}" ]]; then
  prepend_command_dir "${NPM_BIN}"
elif [[ -x "/home/dev/.local/opt/node/bin/node" ]]; then
  export PATH="/home/dev/.local/opt/node/bin:${PATH}"
fi

if [[ -n "${NPM_BIN:-}" ]]; then
  if ! command_available "${NPM_BIN}"; then
    echo "!! NPM_BIN is not executable or not found: ${NPM_BIN}"
    exit 1
  fi
elif command -v npm >/dev/null 2>&1; then
  NPM_BIN="npm"
fi

if [[ -n "${NODE_BIN:-}" ]]; then
  prepend_command_dir "${NODE_BIN}"
  if ! command_available "${NODE_BIN}"; then
    echo "!! NODE_BIN is not executable or not found: ${NODE_BIN}"
    exit 1
  fi
elif command -v node >/dev/null 2>&1; then
  NODE_BIN="node"
elif [[ -x "/home/dev/.local/opt/node/bin/node" ]]; then
  NODE_BIN="/home/dev/.local/opt/node/bin/node"
else
  NODE_BIN=""
fi
if [[ -n "${NPM_BIN:-}" && -z "${NODE_BIN}" ]]; then
  echo "!! node not found. Set NODE_BIN=/path/to/node or add node to PATH."
  exit 1
fi
ensure_node_version_supported

if [[ -n "${GO_BIN:-}" ]] && ! command_available "${GO_BIN}"; then
  echo "!! GO_BIN is not executable or not found: ${GO_BIN}"
  exit 1
elif command -v go >/dev/null 2>&1; then
  GO_BIN="go"
elif [[ -x "/usr/local/go/bin/go" ]]; then
  GO_BIN="/usr/local/go/bin/go"
else
  echo "!! go not found. Set GO_BIN=/path/to/go"
  exit 1
fi
echo "==> Go: $("${GO_BIN}" version)"
if [[ -n "${NODE_BIN}" ]]; then
  echo "==> Node: $("${NODE_BIN}" --version)"
fi
if [[ -n "${NPM_BIN:-}" ]]; then
  echo "==> npm: $("${NPM_BIN}" --version)"
fi
echo "==> Target service: ${SERVICE_NAME}"
echo "    binary: ${INSTALL_PATH}"
echo "    config: ${CONFIG_PATH}"
echo "    working directory: ${WORKING_DIR}"

echo "==> Building panel binary from ${REPO_ROOT}"
cd "${REPO_ROOT}"
VERSION="${VERSION:-$(sed -n 's/^[[:space:]]*version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' cmd/server/main.go | head -n1)}"
VERSION="${VERSION:-dev}"
BUILD_TIME=$(date +%Y%m%d%H%M%S)
BUILD_CODE="${BUILD_TIME}"
GIT_COMMIT=$(git -c "safe.directory=${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo unknown)
"${GO_BIN}" build \
  -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.buildCode=${BUILD_CODE} -X main.commit=${GIT_COMMIT}" \
  -o "${BUILD_OUTPUT}" \
  cmd/server/main.go
NEW_SIZE=$(stat -c%s "${BUILD_OUTPUT}")
echo "    built ${BUILD_OUTPUT} (${NEW_SIZE} bytes)"
echo "    version ${VERSION}, build code ${BUILD_CODE}, commit ${GIT_COMMIT}"

echo "==> Building frontend assets"
ensure_frontend_outdir_writable
if [[ -n "${NPM_BIN:-}" ]]; then
  (cd "${REPO_ROOT}/web" && "${NPM_BIN}" run build)
elif [[ -n "${NODE_BIN}" && -f "${REPO_ROOT}/web/node_modules/vite/bin/vite.js" ]]; then
  (cd "${REPO_ROOT}/web" && "${NODE_BIN}" node_modules/vite/bin/vite.js build)
else
  echo "!! npm not found and fallback vite entry is unavailable. Run npm install in web/ or set NPM_BIN."
  exit 1
fi

TIMESTAMP=$(date +%Y%m%d%H%M%S)
BACKUP_PATH="${INSTALL_PATH}.bak.${TIMESTAMP}"
API_PORT="$(read_config_value "server" "port" "18080")"
FRONTEND_PORT="$(read_config_value "frontend" "port" "13000")"
FRONTEND_ENABLED="$(read_config_value "frontend" "enable" "true")"
FRONTEND_PATH_RAW="$(read_config_value "frontend" "path" "web/public")"
FRONTEND_STATIC_DIR="$(resolve_runtime_path "${FRONTEND_PATH_RAW}")"
GRPC_PORT="$(read_config_value "grpc" "port" "50051")"
GRPC_ENABLED="$(read_config_value "grpc" "enabled" "false")"
TLS_ENABLED="$(read_config_value "tls" "enable" "false")"
FRONTEND_DIRS=()
append_frontend_dir "${FRONTEND_STATIC_DIR}"
append_frontend_dir "${WORKING_DIR}/web/public"
append_frontend_dir "$(dirname "${CONFIG_PATH}")/web/public"
append_frontend_dir "$(dirname "${INSTALL_PATH}")/web/public"
append_frontend_dir "/var/lib/v2board/web/public"
append_frontend_dir "/var/lib/v2board/frontend"
append_frontend_dir "/opt/v2board/web/public"
append_frontend_dir "/etc/v2board/web/public"

if [[ -e "${INSTALL_PATH}" ]]; then
  echo "==> Backing up current binary to ${BACKUP_PATH}"
  "${SUDO[@]}" cp "${INSTALL_PATH}" "${BACKUP_PATH}"
else
  echo "==> No existing binary at ${INSTALL_PATH}; skipping backup"
  BACKUP_PATH=""
fi

echo "==> Stopping ${SERVICE_NAME}"
"${SUDO[@]}" systemctl stop "${SERVICE_NAME}" || true
DEPLOY_STOPPED=1

echo "==> Installing new binary"
"${SUDO[@]}" mkdir -p "$(dirname "${INSTALL_PATH}")"
"${SUDO[@]}" cp "${BUILD_OUTPUT}" "${INSTALL_PATH}"
"${SUDO[@]}" chown root:root "${INSTALL_PATH}"
"${SUDO[@]}" chmod 755 "${INSTALL_PATH}"

echo "==> Syncing frontend assets"
for frontend_dir in "${FRONTEND_DIRS[@]}"; do
  sync_frontend_dir "${frontend_dir}"
done

echo "==> Starting ${SERVICE_NAME}"
"${SUDO[@]}" systemctl start "${SERVICE_NAME}"

sleep 2

echo "==> Verifying"
if ! "${SUDO[@]}" systemctl is-active --quiet "${SERVICE_NAME}"; then
  echo "!! Service failed to start"
  exit 1
fi
echo "    ${SERVICE_NAME} is active"

check_port "api" "${API_PORT}"

if [[ "${FRONTEND_ENABLED}" == "true" ]]; then
  check_port "frontend" "${FRONTEND_PORT}"
fi

if [[ "${GRPC_ENABLED}" == "true" ]]; then
  check_port "grpc" "${GRPC_PORT}"
fi

if command -v curl >/dev/null 2>&1 && [[ "${TLS_ENABLED}" != "true" ]]; then
  curl -fsS --max-time 5 "http://127.0.0.1:${API_PORT}/health" >/dev/null
  echo "    /health OK"
fi

DEPLOY_DONE=1
if [[ -n "${BACKUP_PATH}" ]]; then
  echo "==> Done. Old binary kept at ${BACKUP_PATH}"
else
  echo "==> Done"
fi
