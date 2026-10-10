#!/usr/bin/env bash
# Install or upgrade a GitHub Actions-built AnixOps Control release without cloning
# the repository or compiling on the target host.

set -Eeuo pipefail

REPO_OWNER="${REPO_OWNER:-AnixOps}"
REPO_NAME="${REPO_NAME:-anix-control}"
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
SKIP_START=0
COMMAND="install"
VERSION="${ANIX_CONTROL_VERSION:-${V2BOARD_VERSION:-}}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
PACKAGE_VERSION=""
IDENTITY_BOOTSTRAP_DIR=""
PLUGIN_HOST_RUNTIME_DIR=""
PLUGIN_ARTIFACT_DIR=""
FRESH_CONFIG=0

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

# Agent access (AgentEnrollment needs the built-in CA and gRPC with TLS).
# CA_KEK_FILE holds module_runtime.ca_kek; TLS_DIR the gRPC certificate.
CA_KEK_FILE=""
TLS_DIR=""
GRPC_TLS_CERT="${GRPC_TLS_CERT:-}"
GRPC_TLS_KEY="${GRPC_TLS_KEY:-}"
GRPC_NAME="${GRPC_NAME:-}"
# Agents verify Control's gRPC certificate against the node's system roots, so
# the installer checks it the same way. Tests point this at their own CA.
GRPC_TLS_TRUST_CA_FILE="${GRPC_TLS_TRUST_CA_FILE:-}"
LETSENCRYPT_LIVE_DIR="${LETSENCRYPT_LIVE_DIR:-/etc/letsencrypt/live}"
SYSTEMD_UNIT_DIR="${SYSTEMD_UNIT_DIR:-/etc/systemd/system}"

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
  install.sh [install|update|rollback|preflight|migrate|enable-agents] [options]

This native systemd installer installs the release tag you name. It never
resolves a moving "latest" release. Containers are the primary deployment
(docs/DEPLOYMENT.md).

Options:
  --version <tag>          Release tag to install, for example v4.2.0. Required for install and update.
  --admin-email <email>    Bootstrap admin email on a fresh installation.
  --admin-password <text>  Bootstrap admin password on a fresh installation.
  --install-dir <path>     Installation root. Default: /opt/anixops/control.
  --health-url <url>       Health endpoint to verify after start.
  --skip-start             Install files and systemd unit without starting the service.
  --grpc-name <host>       DNS name (or IP) Agents dial for gRPC. Sets agent_install.grpc_target,
                           and finds /etc/letsencrypt/live/<host>/ when no certificate is given.
  --grpc-tls-cert <path>   PEM certificate chain for the gRPC listener (publicly trusted).
  --grpc-tls-key <path>    Its private key.
  --plan <path>            Preflight JSON output path (default: migration-plan.json).
  -h, --help               Show this help.

A fresh install generates the CA key-encryption key (module_runtime.ca_kek,
config/secrets/module_ca_kek, mode 0600) and, given a publicly trusted
certificate, enables gRPC with TLS, so Agents can enroll at once. It never
replaces an existing key. `enable-agents` does the same on an existing install
without downloading a release. Agents verify Control's certificate against the
node's system roots: a self-signed certificate is refused.

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

uses_plugin_only_identity_bootstrap() {
  [[ "${VERSION}" =~ ^v([0-9]+)\. ]] && (( 10#${BASH_REMATCH[1]} >= 4 ))
}

# The installer installs the tag it is given. It never resolves a moving
# "latest" release, so publishing a release cannot change what an unpinned
# command installs. Checked before the installer changes anything on the host.
require_version() {
  [[ -n "${VERSION}" ]] || \
    die "${COMMAND} requires --version <release tag>, for example --version v4.2.0 (releases: https://github.com/${REPO_OWNER}/${REPO_NAME}/releases)"
  validate_version
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

identity_bootstrap_asset_prefix() {
  [[ -n "${PACKAGE_VERSION}" ]] || die "Identity bootstrap package version is not configured"
  printf 'identity-platform-%s' "${PACKAGE_VERSION}"
}

download_identity_bootstrap_assets() {
  local prefix asset
  prefix="$(identity_bootstrap_asset_prefix)"
  for asset in "${prefix}.anxp" "${prefix}.manifest.json" "${prefix}.manifest.sig"; do
    download_asset "${asset}"
    verify_asset "${asset}"
  done
}

stage_identity_bootstrap_assets() {
  local prefix parent asset
  prefix="$(identity_bootstrap_asset_prefix)"
  [[ -n "${IDENTITY_BOOTSTRAP_DIR}" ]] || die "Identity bootstrap directory is not configured"
  parent="${INSTALL_DIR}/bootstrap"
  [[ ! -L "${parent}" && ! -L "${IDENTITY_BOOTSTRAP_DIR}" ]] || die "Identity bootstrap path must not be a symbolic link"
  install -d -m 0750 -o root -g "${APP_USER}" "${parent}"
  install -d -m 0750 -o root -g "${APP_USER}" "${IDENTITY_BOOTSTRAP_DIR}"
  [[ -d "${IDENTITY_BOOTSTRAP_DIR}" && ! -L "${IDENTITY_BOOTSTRAP_DIR}" ]] || die "Identity bootstrap directory is unsafe"
  for asset in "${prefix}.anxp" "${prefix}.manifest.json" "${prefix}.manifest.sig"; do
    [[ -f "${TMP_DIR}/${asset}" && ! -L "${TMP_DIR}/${asset}" ]] || die "Verified identity bootstrap asset is unavailable: ${asset}"
    install -m 0640 -o root -g "${APP_USER}" "${TMP_DIR}/${asset}" "${IDENTITY_BOOTSTRAP_DIR}/${asset}"
  done
  info "Staged verified identity bootstrap package in ${IDENTITY_BOOTSTRAP_DIR}"
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

validate_install_managed_plugin_dir() {
  local value="$1"
  [[ -n "${value}" && "${value}" == "${INSTALL_DIR}/"* && "${value}" != *$'\n'* && "${value}" != *'"'* && "${value}" != *'..'* ]] || \
    die "Plugin runtime paths must be safe absolute children of the installation directory."
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
  FRESH_CONFIG=1
  printf '%s\n' "${ADMIN_PASSWORD}" > "${INSTALL_DIR}/.bootstrap-admin-password"
  chmod 0640 "${INSTALL_DIR}/.bootstrap-admin-password"
  chown root:"${APP_USER}" "${INSTALL_DIR}/.bootstrap-admin-password"
  warn "Fresh installation: bootstrap admin email is ${ADMIN_EMAIL}"
  warn "The one-time bootstrap password is stored in ${INSTALL_DIR}/.bootstrap-admin-password; move it to a password manager, then delete the file."
}

configure_identity_bootstrap_config() {
  [[ -f "${CONFIG_FILE}" ]] || die "Configuration file is missing: ${CONFIG_FILE}"
  [[ -n "${IDENTITY_BOOTSTRAP_DIR}" ]] || die "Identity bootstrap directory is not configured"
  validate_config_value "${IDENTITY_BOOTSTRAP_DIR}"

  local configure_runtime_paths=0 replace_runtime_paths=0
  if uses_plugin_only_identity_bootstrap; then
    [[ -n "${PLUGIN_HOST_RUNTIME_DIR}" && -n "${PLUGIN_ARTIFACT_DIR}" ]] || \
      die "Plugin execution directories are not configured"
    validate_install_managed_plugin_dir "${PLUGIN_HOST_RUNTIME_DIR}"
    validate_install_managed_plugin_dir "${PLUGIN_ARTIFACT_DIR}"
    configure_runtime_paths=1
    # The systemd unit permits writes only under INSTALL_DIR, so V4 package
    # execution cannot retain an external host/artifact runtime path.
    replace_runtime_paths=1
  fi

  local temporary
  temporary="$(mktemp "${CONFIG_FILE}.identity-bootstrap.XXXXXX")"
  if ! awk \
    -v bootstrap_dir="${IDENTITY_BOOTSTRAP_DIR}" \
    -v configure_runtime_paths="${configure_runtime_paths}" \
    -v replace_runtime_paths="${replace_runtime_paths}" \
    -v host_runtime_dir="${PLUGIN_HOST_RUNTIME_DIR}" \
    -v artifact_dir="${PLUGIN_ARTIFACT_DIR}" \
    -v legacy_host_runtime_dir="/var/lib/anixops/plugin-hosts" \
    -v legacy_artifact_dir="/var/lib/anixops/plugin-artifacts" '
    function field_value(line, value) {
      value = line
      sub(/^[^:]*:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*$/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (value ~ /^".*"$/) {
        sub(/^"/, "", value)
        sub(/"$/, "", value)
      }
      return value
    }
    function emit_missing() {
      if (!bootstrap_seen) {
        print "  identity_bootstrap_package_dir: \"" bootstrap_dir "\""
        bootstrap_seen = 1
      }
      if (!execution_seen) {
        print "  control_execution_enabled: true"
        execution_seen = 1
      }
      if (configure_runtime_paths && !host_runtime_seen) {
        print "  control_host_runtime_dir: \"" host_runtime_dir "\""
        host_runtime_seen = 1
      }
      if (configure_runtime_paths && !artifact_dir_seen) {
        print "  control_host_artifact_dir: \"" artifact_dir "\""
        artifact_dir_seen = 1
      }
    }
    /^plugins:[[:space:]]*(#.*)?$/ {
      if (in_plugins) {
        emit_missing()
      }
      plugins_seen = 1
      in_plugins = 1
      print
      next
    }
    in_plugins && /^[A-Za-z_][A-Za-z0-9_]*:/ {
      emit_missing()
      in_plugins = 0
    }
    in_plugins && /^[[:space:]]+identity_bootstrap_package_dir:/ {
      existing = $0
      sub(/^[^:]*:[[:space:]]*/, "", existing)
      sub(/[[:space:]]+#.*$/, "", existing)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", existing)
      if (existing == "" || existing == "\"\"") {
        print "  identity_bootstrap_package_dir: \"" bootstrap_dir "\""
      } else {
        print
      }
      bootstrap_seen = 1
      next
    }
    in_plugins && /^[[:space:]]+control_execution_enabled:/ {
      print "  control_execution_enabled: true"
      execution_seen = 1
      next
    }
    in_plugins && /^[[:space:]]+control_host_runtime_dir:/ {
      existing = field_value($0)
      if (configure_runtime_paths && (replace_runtime_paths || existing == "" || existing == legacy_host_runtime_dir)) {
        print "  control_host_runtime_dir: \"" host_runtime_dir "\""
      } else {
        print
      }
      host_runtime_seen = 1
      next
    }
    in_plugins && /^[[:space:]]+control_host_artifact_dir:/ {
      existing = field_value($0)
      if (configure_runtime_paths && (replace_runtime_paths || existing == "" || existing == legacy_artifact_dir)) {
        print "  control_host_artifact_dir: \"" artifact_dir "\""
      } else {
        print
      }
      artifact_dir_seen = 1
      next
    }
    { print }
    END {
      if (in_plugins) {
        emit_missing()
      }
      if (!plugins_seen) {
        print "plugins:"
        print "  identity_bootstrap_package_dir: \"" bootstrap_dir "\""
        print "  control_execution_enabled: true"
        if (configure_runtime_paths) {
          print "  control_host_runtime_dir: \"" host_runtime_dir "\""
          print "  control_host_artifact_dir: \"" artifact_dir "\""
        }
      }
    }
  ' "${CONFIG_FILE}" > "${temporary}"; then
    find "${temporary}" -depth -delete
    die "Could not update identity bootstrap configuration"
  fi
  chmod 0640 "${temporary}"
  chown root:"${APP_USER}" "${temporary}"
  mv -f "${temporary}" "${CONFIG_FILE}"
}

# ---------------------------------------------------------------------------
# Agent access: the built-in CA key and gRPC TLS (AgentEnrollment).

# config_section_value <section> <key> prints a top-level section's direct key
# from CONFIG_FILE, unquoted; empty when unset.
config_section_value() {
  local section="$1" key="$2"
  [[ -f "${CONFIG_FILE}" ]] || return 0
  awk -v section="${section}" -v key="${key}" '
    /^[A-Za-z_][A-Za-z0-9_]*:/ {
      current = $0
      sub(/:.*/, "", current)
      next
    }
    current == section && index($0, "  " key ":") == 1 {
      value = substr($0, length(key) + 4)
      sub(/^[[:space:]]+/, "", value)
      if (value ~ /^"/) {
        sub(/^"/, "", value)
        sub(/".*$/, "", value)
      } else if (value ~ /^\047/) {
        sub(/^\047/, "", value)
        sub(/\047.*$/, "", value)
      } else {
        sub(/[[:space:]]+#.*$/, "", value)
        sub(/[[:space:]]+$/, "", value)
      }
      print value
      exit
    }
  ' "${CONFIG_FILE}"
}

# set_config_section_values <section> <key=yaml-scalar>... replaces the keys in
# a top-level section of CONFIG_FILE, appending missing keys and the section.
set_config_section_values() {
  local section="$1" assignments temporary
  shift
  assignments="$(printf '%s\n' "$@")"
  temporary="$(mktemp "${CONFIG_FILE}.edit.XXXXXX")"
  if ! awk -v section="${section}" -v assignments="${assignments}" '
    BEGIN {
      lines = split(assignments, entries, "\n")
      for (i = 1; i <= lines; i++) {
        if (entries[i] == "") continue
        key = entries[i]
        sub(/=.*/, "", key)
        value = entries[i]
        sub(/^[^=]*=/, "", value)
        order[++count] = key
        values[key] = value
      }
    }
    function emit_missing(   i) {
      for (i = 1; i <= count; i++) {
        if (!(order[i] in seen)) {
          print "  " order[i] ": " values[order[i]]
          seen[order[i]] = 1
        }
      }
    }
    /^[A-Za-z_][A-Za-z0-9_]*:/ {
      if (in_section) {
        emit_missing()
        in_section = 0
      }
      name = $0
      sub(/:.*/, "", name)
      if (name == section) {
        in_section = 1
        found = 1
        print
        next
      }
    }
    in_section && match($0, /^  [A-Za-z_][A-Za-z0-9_]*:/) {
      key = substr($0, 3, RLENGTH - 3)
      if (key in values) {
        print "  " key ": " values[key]
        seen[key] = 1
        next
      }
    }
    { print }
    END {
      if (in_section) emit_missing()
      if (!found) {
        print section ":"
        emit_missing()
      }
    }
  ' "${CONFIG_FILE}" > "${temporary}"; then
    find "${temporary}" -depth -delete
    die "Could not update the ${section} configuration in ${CONFIG_FILE}"
  fi
  chmod 0640 "${temporary}"
  chown root:"${APP_USER}" "${temporary}"
  mv -f "${temporary}" "${CONFIG_FILE}"
}

# ca_kek_fingerprint prints the first 16 hex digits of the SHA-256 of the key
# file: enough to compare copies, never the key itself.
ca_kek_fingerprint() {
  sha256sum "$1" | cut -c1-16
}

# configured_ca_kek_source names where a CA key is configured other than
# CA_KEK_FILE (config.yaml or a systemd drop-in); empty when nowhere.
configured_ca_kek_source() {
  local dropin="${SYSTEMD_UNIT_DIR}/${SERVICE_NAME}.service.d"
  if [[ -n "$(config_section_value module_runtime ca_kek)" ]]; then
    printf '%s (module_runtime.ca_kek)\n' "${CONFIG_FILE}"
  elif [[ -d "${dropin}" ]] && grep -rqs 'ANIX_CONTROL_MODULE_RUNTIME_CA_KEK' "${dropin}"; then
    printf '%s\n' "${dropin}"
  fi
}

# ca_kek_file_in_use reports whether the unit should pass CA_KEK_FILE: it
# exists and no other source sets the key (an environment variable would
# silently override config.yaml).
ca_kek_file_in_use() {
  [[ -n "${CA_KEK_FILE}" && -s "${CA_KEK_FILE}" && ! -L "${CA_KEK_FILE}" && -z "$(configured_ca_kek_source)" ]]
}

# ensure_ca_kek creates CA_KEK_FILE with 32 random bytes (base64) when no CA
# key is configured anywhere. An existing key is never replaced: changing it
# makes the CA stored in the database unusable (rotation is a manual
# procedure, docs/UPGRADE.md).
ensure_ca_kek() {
  local source temporary
  [[ ! -L "${CA_KEK_FILE}" ]] || die "${CA_KEK_FILE} must not be a symbolic link"
  source="$(configured_ca_kek_source)"
  if [[ -s "${CA_KEK_FILE}" ]]; then
    if [[ -n "${source}" ]]; then
      warn "The CA key is set in ${source} and in ${CA_KEK_FILE}; Control uses ${source}. Remove one of them (keep the one whose key sealed the CA)."
      return 0
    fi
    chmod 0600 "${CA_KEK_FILE}"
    chown "${APP_USER}:${APP_USER}" "${CA_KEK_FILE}"
    info "Keeping the existing CA key-encryption key ${CA_KEK_FILE} (fingerprint $(ca_kek_fingerprint "${CA_KEK_FILE}"))"
    return 0
  fi
  if [[ -n "${source}" ]]; then
    info "The CA key-encryption key is set in ${source}; keeping it."
    return 0
  fi
  install -d -m 0700 -o "${APP_USER}" -g "${APP_USER}" "$(dirname "${CA_KEK_FILE}")"
  temporary="$(mktemp "${CA_KEK_FILE}.XXXXXX")"
  chmod 0600 "${temporary}"
  head -c 32 /dev/urandom | base64 | tr -d '\n' > "${temporary}"
  [[ "$(wc -c < "${temporary}")" -eq 44 ]] || { find "${temporary}" -depth -delete; die "Could not generate the CA key-encryption key"; }
  chown "${APP_USER}:${APP_USER}" "${temporary}"
  mv -f "${temporary}" "${CA_KEK_FILE}"
  warn "Generated the CA key-encryption key ${CA_KEK_FILE} (mode 0600, fingerprint $(ca_kek_fingerprint "${CA_KEK_FILE}"))."
  warn "Back it up with the database: without it the CA that signs Agent certificates cannot be used, and every Agent must enroll again."
}

grpc_name_is_ip() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ || "$1" == *:* ]]
}

validate_grpc_name() {
  [[ -z "${GRPC_NAME}" ]] && return 0
  [[ "${GRPC_NAME}" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || grpc_name_is_ip "${GRPC_NAME}" || \
    die "--grpc-name must be a DNS name or an IP address"
  [[ "${GRPC_NAME}" != *:*:* || "${GRPC_NAME}" =~ ^[0-9A-Fa-f:]+$ ]] || die "--grpc-name must be a DNS name or an IP address"
}

# resolve_grpc_tls_source fills GRPC_TLS_CERT/KEY from Let's Encrypt for
# GRPC_NAME when the operator gave no certificate.
resolve_grpc_tls_source() {
  if [[ -n "${GRPC_TLS_CERT}" || -n "${GRPC_TLS_KEY}" ]]; then
    [[ -n "${GRPC_TLS_CERT}" && -n "${GRPC_TLS_KEY}" ]] || die "--grpc-tls-cert and --grpc-tls-key go together"
    return 0
  fi
  [[ -n "${GRPC_NAME}" ]] || return 0
  local live="${LETSENCRYPT_LIVE_DIR}/${GRPC_NAME}"
  if [[ -r "${live}/fullchain.pem" && -r "${live}/privkey.pem" ]]; then
    GRPC_TLS_CERT="${live}/fullchain.pem"
    GRPC_TLS_KEY="${live}/privkey.pem"
    info "Using the Let's Encrypt certificate ${live} for gRPC"
  fi
}

# verify_grpc_tls_certificate checks what an Agent checks: the chain against
# the system roots, the name it dials, and that the key belongs to it.
verify_grpc_tls_certificate() {
  local cert="$1" key="$2" name="$3" cert_key key_key output
  [[ -f "${cert}" && -r "${cert}" ]] || die "gRPC certificate not found: ${cert}"
  [[ -f "${key}" && -r "${key}" ]] || die "gRPC private key not found: ${key}"
  if ! command -v openssl >/dev/null 2>&1; then
    warn "openssl is not installed: cannot check ${cert} the way Agents do. It must chain to a public CA and name the host Agents dial."
    return 0
  fi
  openssl x509 -in "${cert}" -noout >/dev/null 2>&1 || die "${cert} is not a PEM certificate"
  cert_key="$(openssl x509 -in "${cert}" -noout -pubkey 2>/dev/null | openssl pkey -pubin -outform DER 2>/dev/null | sha256sum)"
  key_key="$(openssl pkey -in "${key}" -pubout -outform DER 2>/dev/null | sha256sum)"
  [[ "${cert_key}" == "${key_key}" ]] || die "${key} is not the private key of ${cert}"
  local verify=(openssl verify -purpose sslserver -untrusted "${cert}")
  [[ -n "${GRPC_TLS_TRUST_CA_FILE}" ]] && verify+=(-CAfile "${GRPC_TLS_TRUST_CA_FILE}")
  if ! output="$("${verify[@]}" "${cert}" 2>&1)"; then
    warn "${output//$'\n'/ }"
    die "${cert} does not verify against this host's system CA roots. Agents verify Control's gRPC certificate against their system roots and cannot be told to trust a private or self-signed CA: use a publicly trusted certificate (Let's Encrypt: config/deploy/grpc_tls/setup_certbot.sh, or your reverse proxy's certificate for the same name)."
  fi
  if [[ -n "${name}" ]]; then
    local check=(-checkhost "${name}")
    grpc_name_is_ip "${name}" && check=(-checkip "${name}")
    output="$(openssl x509 -in "${cert}" -noout "${check[@]}" 2>&1 || true)"
    [[ "${output}" == *"does match"* ]] || die "${cert} is not valid for ${name}, the name Agents dial"
  fi
  info "The gRPC certificate ${cert} verifies against the system roots${name:+ for ${name}}"
}

# agent_grpc_name prints the host Agents dial: --grpc-name, else the host of
# agent_install.grpc_target or agent_install.public_url (empty: the panel's
# request origin, unknown here).
agent_grpc_name() {
  local value
  if [[ -n "${GRPC_NAME}" ]]; then
    printf '%s\n' "${GRPC_NAME}"
    return
  fi
  value="$(config_section_value agent_install grpc_target)"
  if [[ -n "${value}" ]]; then
    value="${value%:*}"
    value="${value#[}"
    printf '%s\n' "${value%]}"
    return
  fi
  value="$(config_section_value agent_install public_url)"
  value="${value#https://}"
  value="${value%%/*}"
  value="${value%:*}"
  printf '%s\n' "${value}"
}

# preflight_grpc_tls validates the operator's TLS input before anything is
# installed.
preflight_grpc_tls() {
  validate_grpc_name
  resolve_grpc_tls_source
  [[ -n "${GRPC_TLS_CERT}" ]] || return 0
  verify_grpc_tls_certificate "${GRPC_TLS_CERT}" "${GRPC_TLS_KEY}" "$(agent_grpc_name)"
}

# install_grpc_tls copies the certificate into TLS_DIR (the service user cannot
# read /etc/letsencrypt) and enables gRPC with it on all interfaces.
install_grpc_tls() {
  local port host
  [[ -n "${GRPC_TLS_CERT}" ]] || return 0
  [[ ! -L "${TLS_DIR}" ]] || die "${TLS_DIR} must not be a symbolic link"
  install -d -m 0750 -o "${APP_USER}" -g "${APP_USER}" "${TLS_DIR}"
  install -m 0644 -o "${APP_USER}" -g "${APP_USER}" "${GRPC_TLS_CERT}" "${TLS_DIR}/control.crt.new"
  install -m 0600 -o "${APP_USER}" -g "${APP_USER}" "${GRPC_TLS_KEY}" "${TLS_DIR}/control.key.new"
  mv -f "${TLS_DIR}/control.crt.new" "${TLS_DIR}/control.crt"
  mv -f "${TLS_DIR}/control.key.new" "${TLS_DIR}/control.key"
  host="$(config_section_value grpc host)"
  case "${host}" in
    ""|127.0.0.1|localhost|::1) host='"0.0.0.0"' ;;
    *) host="\"${host}\"" ;;
  esac
  set_config_section_values grpc \
    "enabled=true" \
    "host=${host}" \
    "tls_cert_file=\"${TLS_DIR}/control.crt\"" \
    "tls_key_file=\"${TLS_DIR}/control.key\""
  if [[ -n "${GRPC_NAME}" ]]; then
    port="$(config_section_value grpc port)"
    [[ "${port}" =~ ^[0-9]+$ ]] || port=50051
    if grpc_name_is_ip "${GRPC_NAME}" && [[ "${GRPC_NAME}" == *:* ]]; then
      set_config_section_values agent_install "grpc_target=\"[${GRPC_NAME}]:${port}\""
    else
      set_config_section_values agent_install "grpc_target=\"${GRPC_NAME}:${port}\""
    fi
  fi
  info "gRPC TLS is enabled with ${TLS_DIR}/control.crt (key mode 0600)"
}

warn_agent_tls_missing() {
  local port name
  port="$(config_section_value grpc port)"
  [[ "${port}" =~ ^[0-9]+$ ]] || port=50051
  name="$(agent_grpc_name)"
  name="${name:-grpc.example.com}"
  warn "Agents cannot connect yet: the gRPC listener has no TLS certificate."
  warn "Agents verify Control's certificate against the node's system CA roots and cannot pin a private CA, so it must be a publicly trusted certificate for the name they dial. The installer does not create a self-signed one: Agents would reject it."
  warn "Next steps (docs/guide/release-installation.md, \"Agent access\"):"
  warn "  1. Get a certificate for ${name}: DOMAIN=${name} CLOUDFLARE_API_TOKEN=<token> sudo -E bash config/deploy/grpc_tls/setup_certbot.sh (or use your reverse proxy's certificate for the same name)."
  warn "  2. sudo bash install.sh enable-agents --grpc-name ${name}   (or add --grpc-tls-cert <fullchain.pem> --grpc-tls-key <privkey.pem>)"
  warn "  3. Open TCP ${port} from the nodes to this host."
}

# configure_agent_access sets up what AgentEnrollment needs. generate_kek=1
# creates the CA key when none is configured (fresh installs, enable-agents);
# otherwise a missing key is only reported.
configure_agent_access() {
  local generate_kek="$1"
  if [[ "${generate_kek}" -eq 1 ]]; then
    ensure_ca_kek
  elif [[ -s "${CA_KEK_FILE}" || -n "$(configured_ca_kek_source)" ]]; then
    ensure_ca_kek
  else
    warn "No CA key-encryption key is configured (module_runtime.ca_kek): Agents cannot enroll. Run: sudo bash install.sh enable-agents (docs/UPGRADE.md)."
  fi
  if [[ -n "${GRPC_TLS_CERT}" ]]; then
    install_grpc_tls
  elif [[ -z "$(config_section_value grpc tls_cert_file)" || "$(config_section_value grpc enabled)" != "true" ]]; then
    warn_agent_tls_missing
  fi
}

backup_ca_kek() {
  [[ -n "${BACKUP_DIR}" && -s "${CA_KEK_FILE}" && ! -L "${CA_KEK_FILE}" ]] || return 0
  cp -p "${CA_KEK_FILE}" "${BACKUP_DIR}/module_ca_kek"
}

# enable_agents configures Agent access on an existing install without
# downloading a release, then restarts the service.
enable_agents() {
  local migrated=0
  # A layout moved by `migrate` keeps its configuration in TARGET_CONFIG_DIR.
  if [[ ! -f "${CONFIG_FILE}" && -f "${TARGET_CONFIG_DIR}/config.yaml" ]]; then
    CONFIG_FILE="${TARGET_CONFIG_DIR}/config.yaml"
    migrated=1
  fi
  [[ -f "${CONFIG_FILE}" ]] || die "No installation at ${INSTALL_DIR} (missing ${CONFIG_FILE}); run install first."
  preflight_grpc_tls
  configure_agent_access 1
  if [[ "${migrated}" -eq 1 ]]; then
    write_migration_unit
  else
    write_systemd_unit
  fi
  if [[ "${SKIP_START}" -eq 1 ]]; then
    info "Agent access configured; service restart was skipped."
    return 0
  fi
  start_and_verify || die "The service did not become healthy. Inspect: journalctl -u ${SERVICE_NAME} -n 200 --no-pager"
  info "Agent access configured. Check: ${BINARY_PATH} -config ${CONFIG_FILE} agents transports --check-required"
}

ensure_layout_and_config() {
  install -d -m 0750 -o "${APP_USER}" -g "${APP_USER}" \
    "${INSTALL_DIR}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/config" \
    "${INSTALL_DIR}/config/data" "${INSTALL_DIR}/logs" "${INSTALL_DIR}/web"
  install -d -m 0700 -o root -g root "${BACKUP_ROOT}"
  if uses_plugin_only_identity_bootstrap; then
    validate_install_managed_plugin_dir "${PLUGIN_HOST_RUNTIME_DIR}"
    validate_install_managed_plugin_dir "${PLUGIN_ARTIFACT_DIR}"
    [[ ! -L "${PLUGIN_HOST_RUNTIME_DIR}" && ! -L "${PLUGIN_ARTIFACT_DIR}" ]] || \
      die "Plugin execution directories must not be symbolic links"
    install -d -m 0700 -o "${APP_USER}" -g "${APP_USER}" \
      "${PLUGIN_HOST_RUNTIME_DIR}" "${PLUGIN_ARTIFACT_DIR}"
    [[ -d "${PLUGIN_HOST_RUNTIME_DIR}" && ! -L "${PLUGIN_HOST_RUNTIME_DIR}" ]] || \
      die "Plugin host runtime directory is unsafe"
    [[ -d "${PLUGIN_ARTIFACT_DIR}" && ! -L "${PLUGIN_ARTIFACT_DIR}" ]] || \
      die "Plugin artifact directory is unsafe"
  fi

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
  sed -i "s#ReadWritePaths=.*#ReadWritePaths=${INSTALL_DIR} ${TARGET_CONFIG_DIR} ${TARGET_DATA_DIR} ${TARGET_LOG_DIR}#" "${SYSTEMD_UNIT_DIR}/${SERVICE_NAME}.service"
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
  PACKAGE_VERSION="${VERSION#v}"
  IDENTITY_BOOTSTRAP_DIR="${INSTALL_DIR}/bootstrap/identity-platform-${PACKAGE_VERSION}"
  PLUGIN_HOST_RUNTIME_DIR="${INSTALL_DIR}/runtime/plugin-hosts"
  PLUGIN_ARTIFACT_DIR="${INSTALL_DIR}/data/plugin-artifacts"
  CA_KEK_FILE="${INSTALL_DIR}/config/secrets/module_ca_kek"
  TLS_DIR="${INSTALL_DIR}/config/tls"
  preflight_grpc_tls
  install_base_tools
  ensure_app_user
  install -d -m 0750 -o "${APP_USER}" -g "${APP_USER}" "${INSTALL_DIR}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/web" "${INSTALL_DIR}/config/data" "${INSTALL_DIR}/logs" "${TARGET_CONFIG_DIR}" "${TARGET_DATA_DIR}" "${TARGET_LOG_DIR}"
  # Download and verify everything before touching the running legacy service.
  local binary_archive; binary_archive="$(asset_name)"
  download_asset "SHA256SUMS.txt"; download_asset "${binary_archive}"; download_asset "anix-control-frontend.tar.gz"
  verify_asset "${binary_archive}"; verify_asset "anix-control-frontend.tar.gz"
  if uses_plugin_only_identity_bootstrap; then
    download_identity_bootstrap_assets
    stage_identity_bootstrap_assets
  fi
  snapshot_migration_state
  stop_legacy_service
  copy_migration_config
  install_release_files "${binary_archive}"
  if uses_plugin_only_identity_bootstrap; then
    configure_identity_bootstrap_config
  fi
  # A migrated install keeps its configuration: a missing CA key is reported,
  # not generated (enable-agents adds it).
  configure_agent_access 0
  write_migration_unit
  # Existing installations commonly use a non-default HTTP port.
  local migrated_port
  migrated_port="$(sed -n -E '/^server:/,/^[^[:space:]]/ s/^[[:space:]]+port:[[:space:]]*([0-9]+).*/\1/p' "${CONFIG_FILE}" | head -n1)"
  [[ "${migrated_port}" =~ ^[0-9]+$ ]] && HEALTH_URL="http://127.0.0.1:${migrated_port}/health"
  if [[ "${SKIP_START}" -eq 1 ]]; then return 0; fi
  if ! start_and_verify || ! verify_plugin_only_startup; then
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
  [[ -f "${CONFIG_FILE}" ]] && cp -a "${CONFIG_FILE}" "${BACKUP_DIR}/config.yaml"
  [[ -f "${VERSION_FILE}" ]] && cp -a "${VERSION_FILE}" "${BACKUP_DIR}/release-version"
  backup_ca_kek
  info "Backed up the previous release to ${BACKUP_DIR}"
}

stop_running_service() {
  if systemctl is-active --quiet "${SERVICE_NAME}"; then
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
  local ca_kek_environment=""
  if ca_kek_file_in_use; then
    ca_kek_environment="Environment=ANIX_CONTROL_MODULE_RUNTIME_CA_KEK_FILE=${CA_KEK_FILE}"
  fi
  cat > "${SYSTEMD_UNIT_DIR}/${SERVICE_NAME}.service" <<EOF
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
${ca_kek_environment}
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
  [[ -f "${BACKUP_DIR}/config.yaml" ]] && cp -a "${BACKUP_DIR}/config.yaml" "${CONFIG_FILE}"
  [[ -f "${BACKUP_DIR}/release-version" ]] && cp -a "${BACKUP_DIR}/release-version" "${VERSION_FILE}"
  chown -R "${APP_USER}:${APP_USER}" "${INSTALL_DIR}/bin" "${INSTALL_DIR}/web" || true
}

start_and_verify() {
  systemctl restart "${SERVICE_NAME}"
  for _ in $(seq 1 30); do
    if systemctl is-active --quiet "${SERVICE_NAME}" && curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      info "Service is healthy: ${HEALTH_URL}"
      return 0
    fi
    sleep 2
  done
  systemctl --no-pager --full status "${SERVICE_NAME}" || true
  return 1
}

identity_login_url() {
  local base
  base="${HEALTH_URL%/health}"
  [[ "${base}" != "${HEALTH_URL}" ]] || base="${HEALTH_URL%/}"
  printf '%s/api/v2/login' "${base}"
}

verify_identity_gateway() {
  local login_url status
  login_url="$(identity_login_url)"
  for _ in $(seq 1 30); do
    status="$(curl -sS --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' \
      -H 'Content-Type: application/json' --data '{}' "${login_url}" || true)"
    case "${status}" in
      200|400|401)
        info "Identity package gateway is ready: ${login_url}"
        return 0
        ;;
    esac
    sleep 2
  done
  warn "Identity package gateway did not become ready: ${login_url}"
  return 1
}

verify_identity_login() {
  if [[ "${FRESH_CONFIG}" -ne 1 && ( -z "${ADMIN_EMAIL}" || -z "${ADMIN_PASSWORD}" ) ]]; then
    return 0
  fi
  local login_url payload response
  login_url="$(identity_login_url)"
  payload="{\"email\":\"$(json_escape "${ADMIN_EMAIL}")\",\"password\":\"$(json_escape "${ADMIN_PASSWORD}")\"}"
  for _ in $(seq 1 30); do
    response="$(curl -fsS --connect-timeout 5 --max-time 15 -H 'Content-Type: application/json' --data "${payload}" "${login_url}" || true)"
    if [[ "${response}" == *'"code":0'* ]]; then
      info "Identity package login verification succeeded"
      return 0
    fi
    sleep 2
  done
  warn "Identity package login verification failed"
  return 1
}

verify_plugin_only_startup() {
  if ! uses_plugin_only_identity_bootstrap; then
    return 0
  fi
  verify_identity_gateway && verify_identity_login
}

parse_args() {
  if [[ "${1:-}" == "install" || "${1:-}" == "update" || "${1:-}" == "rollback" || "${1:-}" == "preflight" || "${1:-}" == "migrate" || "${1:-}" == "enable-agents" ]]; then
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
      --grpc-name) GRPC_NAME="${2:-}"; shift 2 ;;
      --grpc-tls-cert) GRPC_TLS_CERT="${2:-}"; shift 2 ;;
      --grpc-tls-key) GRPC_TLS_KEY="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) die "Unknown argument: $1" ;;
    esac
  done
}

set_install_paths() {
  BINARY_PATH="${INSTALL_DIR}/bin/anix-control"
  LEGACY_BINARY_PATH="${INSTALL_DIR}/bin/v2board"
  CONFIG_FILE="${INSTALL_DIR}/config/config.yaml"
  FRONTEND_DIR="${INSTALL_DIR}/web/public"
  BACKUP_ROOT="${INSTALL_DIR}/backups"
  VERSION_FILE="${INSTALL_DIR}/.release-version"
  PLUGIN_HOST_RUNTIME_DIR="${INSTALL_DIR}/runtime/plugin-hosts"
  PLUGIN_ARTIFACT_DIR="${INSTALL_DIR}/data/plugin-artifacts"
  CA_KEK_FILE="${INSTALL_DIR}/config/secrets/module_ca_kek"
  TLS_DIR="${INSTALL_DIR}/config/tls"
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
    rollback_migration ""
    exit 0
  fi
  if [[ "${COMMAND}" == "enable-agents" ]]; then
    detect_legacy_layout
    validate_install_dir
    set_install_paths
    enable_agents
    exit 0
  fi
  warn "Containers are the primary deployment: https://github.com/AnixOps/anix-control/blob/go_dev/docs/DEPLOYMENT.md"
  require_version
  detect_legacy_layout
  validate_install_dir
  install_base_tools

  set_install_paths
  TMP_DIR="$(mktemp -d)"
  PACKAGE_VERSION="${VERSION#v}"
  IDENTITY_BOOTSTRAP_DIR="${INSTALL_DIR}/bootstrap/identity-platform-${PACKAGE_VERSION}"
  # Check the operator's certificate before anything is installed.
  preflight_grpc_tls

  ensure_app_user
  ensure_layout_and_config

  local binary_archive
  binary_archive="$(asset_name)"
  download_asset "SHA256SUMS.txt"
  download_asset "${binary_archive}"
  download_asset "anix-control-frontend.tar.gz"
  verify_asset "${binary_archive}"
  verify_asset "anix-control-frontend.tar.gz"
  if uses_plugin_only_identity_bootstrap; then
    download_identity_bootstrap_assets
    stage_identity_bootstrap_assets
  fi

  stop_running_service
  backup_current_release
  install_release_files "${binary_archive}"
  if uses_plugin_only_identity_bootstrap; then
    configure_identity_bootstrap_config
  fi
  # After the backup, so a failed update restores the previous config. Fresh
  # installs get the CA key; existing ones keep theirs (or are told how to
  # add one).
  configure_agent_access "${FRESH_CONFIG}"
  write_systemd_unit

  if [[ "${SKIP_START}" -eq 1 ]]; then
    info "Installed ${VERSION}; service start was skipped."
    exit 0
  fi

  if ! start_and_verify || ! verify_plugin_only_startup; then
    if [[ -n "${BACKUP_DIR}" ]]; then
      restore_backup
      systemctl restart "${SERVICE_NAME}" || true
    fi
    die "Installation failed health verification. Inspect: journalctl -u ${SERVICE_NAME} -n 200 --no-pager"
  fi

  info "Installed ${VERSION} successfully."
  [[ "${COMMAND}" == "rollback" ]] && info "Rollback completed by installing the requested release tag."
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
