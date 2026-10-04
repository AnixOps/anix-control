#!/usr/bin/env bash
# AnixOps Agent installer: one-command node onboarding.
#
#   curl -fsSL https://<control>/install.sh | sudo bash -s -- \
#     --control https://<control> --node forward-41 --token anixagt_... \
#     [--mirror control|cn|github] [--reset] [--forward] [--timeout 180]
#
# Copy the command from the node page in Control ("复制安装命令"): it carries a
# single-use enrollment token bound to the node. The script installs the
# AnixOps Agent as a systemd service running as the unprivileged user
# anixops-agent (ambient CAP_NET_ADMIN and CAP_NET_BIND_SERVICE, sandboxed),
# removes the legacy forward runtime of this host (the nftables tables
# inet v2b_forward, ip v2b_forward and ip anixops_forward, and the clean
# agent's v2forward-agent service), turns on IP forwarding for a forward
# node (/etc/sysctl.d/90-anixops-forward.conf; a proxy node with --forward),
# migrates the directories of an earlier root install of the Agent, starts
# the Agent and waits until it has enrolled with Control. Running the same
# command again is safe: it upgrades the Agent in place and keeps its
# identity unless --reset is given.
#
# This file is byte for byte the release asset agent-install.sh. Verify it
# before running it:
#   curl -fsSLO https://<control>/install.sh
#   curl -fsSLO https://<control>/install.sh.sig
#   printf '%s' 'MCowBQYDK2VwAyEAlvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM=' | base64 -d >official.der
#   base64 -d install.sh.sig >install.sh.sig.bin
#   openssl pkeyutl -verify -pubin -keyform DER -inkey official.der -rawin \
#     -in install.sh -sigfile install.sh.sig.bin
#
# Documentation: docs/guide/agent-onboarding.md in the anix-control repository.

set -Eeuo pipefail

# The official AnixOps release key (plugins.official_public_key): base64 of
# the raw Ed25519 public key. It verifies Agent release signatures.
readonly OFFICIAL_PUBLIC_KEY="lvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM="

# ROOT prefixes every path the script writes; tests set it to a temporary
# directory. Paths written into configuration and units never carry it.
ROOT="${ANIX_INSTALL_ROOT:-}"

readonly AGENT_USER="anixops-agent"
readonly GOST_USER="anixops-gost"
readonly SERVICE="anix-agent.service"
readonly GOST_SERVICE="anixops-gost.service"
readonly LIB_DIR="/usr/lib/anixops-agent"
readonly BIN_LINK="/usr/local/bin/anix-agent"
readonly CONFIG_DIR="/etc/anixops/agent"
readonly CONFIG_FILE="${CONFIG_DIR}/config.json"
readonly STATE_DIR="/var/lib/anixops-agent"
readonly PKI_DIR="${STATE_DIR}/pki"
readonly STREAM_DIR="${STATE_DIR}/stream"
readonly CREDENTIAL_FILE="${STATE_DIR}/enroll.credential"
readonly GOST_DIR="/var/lib/anixops-gost"
readonly UNIT_DIR="/etc/systemd/system"
readonly POLKIT_RULE="/etc/polkit-1/rules.d/50-anixops-agent.rules"
readonly LOCK_FILE="/run/anixops-agent-install.lock"
# IP forwarding for forward nodes: nftables DNAT and gost relays route
# packets between interfaces.
readonly SYSCTL_FILE="/etc/sysctl.d/90-anixops-forward.conf"
readonly SYSCTL_SETTINGS=("net.ipv4.ip_forward=1" "net.ipv6.conf.all.forwarding=1")

# The legacy forward runtime the new Agent replaces (forward-sdk.md,
# section 10). Only these named objects are removed, never anything else.
readonly LEGACY_NFT_TABLES=("inet v2b_forward" "ip v2b_forward" "ip anixops_forward")
readonly LEGACY_UNITS=("v2forward-agent.service")
readonly LEGACY_PATHS=("/etc/v2board-forward-agent" "/usr/local/bin/v2forward-agent")
# The directories of an Agent installed by anix-agent's own root installer
# (scripts/install.sh): 'anix-agent migrate-paths' copies them to STATE_DIR.
readonly ROOT_INSTALL_DIRS=("/var/lib/anix-agent" "/var/lib/anixops/plugins")

CONTROL_URL=""
NODE=""
TOKEN="${ANIX_AGENT_TOKEN:-}"
MIRROR="control"
RESET=0
FORWARD=0
TIMEOUT=180
TMP_DIR=""

META_VERSION=""
META_GRPC=""
declare -A META_ASSET=()
declare -A META_SOURCE=()
declare -A META_SHA256=()

ARCH=""
ASSET=""
EXPECTED_DIGEST=""
SOURCE=""
INSTALL_MODE="install"
CONFIG_STATE=""
CREDENTIAL_STATE=""
FORWARDING_STATE=""
MIGRATION_STATE=""
REMOVED=()
NOTES=()

info() { printf '[anixops] %s\n' "$*"; }
note() { NOTES+=("$*"); printf '[anixops] note: %s\n' "$*" >&2; }
die() {
  local code=1
  if [[ "${1:-}" =~ ^[0-9]+$ ]]; then
    code="$1"
    shift
  fi
  printf '[anixops] error: %s\n' "$*" >&2
  exit "${code}"
}

# path prints a system path under ROOT.
path() { printf '%s%s' "${ROOT}" "$1"; }

usage() {
  cat <<'EOF'
Usage: install.sh --control <https://control> --node <proxy-<id>|forward-<id>> --token <anixagt_...> [options]

  --control URL      the Control address nodes reach (from the copied command)
  --node NODE        the node the token is bound to: proxy-<id> or forward-<id>
  --token TOKEN      the single-use enrollment token (or ANIX_AGENT_TOKEN)
  --mirror M         where the Agent is downloaded from: control (default), cn, github
  --reset            discard this host's Agent identity and enroll again (needs --token)
  --forward          turn on IP forwarding on a proxy node too (always on for forward-<id>)
  --timeout SECONDS  how long to wait for the Agent to enroll (default 180)
  --group GROUP      node-group tokens (not supported by this Control yet)
  --offline FILE     install from a downloaded package (not in this release)
  -h, --help         show this help

Running the same command again upgrades the Agent in place and keeps its
identity; the token is then not needed and stays unused.
EOF
}

parse_args() {
  if [[ "${1:-}" == "uninstall" ]]; then
    die 2 "uninstall is not part of this installer release; it comes with 'anix-agent uninstall [--purge]' (forward-sdk.md, section 9)"
  fi
  local flag value
  while (($# > 0)); do
    flag="$1"
    value=""
    case "${flag}" in
      -h | --help)
        usage
        exit 0
        ;;
      --reset)
        RESET=1
        shift
        continue
        ;;
      --forward)
        FORWARD=1
        shift
        continue
        ;;
      --*=*)
        value="${flag#*=}"
        flag="${flag%%=*}"
        shift
        ;;
      --control | --node | --token | --mirror | --timeout | --group | --offline)
        (($# >= 2)) || die 2 "${flag} needs a value"
        value="$2"
        shift 2
        ;;
      *)
        die 2 "unknown argument: ${flag} (see --help)"
        ;;
    esac
    case "${flag}" in
      --control) CONTROL_URL="${value%/}" ;;
      --node) NODE="${value}" ;;
      --token) TOKEN="${value}" ;;
      --mirror) MIRROR="${value}" ;;
      --timeout) TIMEOUT="${value}" ;;
      --group) die 2 "node-group tokens are not supported by this Control yet; copy the command from the node's page (a node-bound token)" ;;
      --offline) die 2 "offline packages are not part of this installer release; download the Agent release on a connected machine or use --mirror cn" ;;
      *) die 2 "unknown argument: ${flag} (see --help)" ;;
    esac
  done
  validate_args
}

validate_args() {
  [[ -n "${CONTROL_URL}" ]] || die 2 "--control is required (copy the command from the node page)"
  [[ "${CONTROL_URL}" =~ ^https://[^/?#[:space:]\'\"]+$ ]] || die 2 "--control must be https://host[:port], not ${CONTROL_URL}"
  [[ "${NODE}" =~ ^(proxy|forward)-[1-9][0-9]{0,9}$ ]] || die 2 "--node must be proxy-<id> or forward-<id>"
  if [[ -n "${TOKEN}" && ! "${TOKEN}" =~ ^anixagt_[A-Za-z0-9_-]{20,}$ ]]; then
    die 2 "--token is not an AnixOps enrollment token (anixagt_...)"
  fi
  case "${MIRROR}" in
    control | cn | github) ;;
    *) die 2 "--mirror must be control, cn or github" ;;
  esac
  [[ "${TIMEOUT}" =~ ^[1-9][0-9]{0,4}$ ]] || die 2 "--timeout must be a number of seconds"
  if ((RESET)) && [[ -z "${TOKEN}" ]]; then
    die 2 "--reset needs --token: the Agent enrolls again"
  fi
}

require_root() {
  [[ "$(id -u)" == "0" ]] || die "run the installer as root (the copied command uses sudo)"
}

detect_platform() {
  [[ "$(uname -s)" == "Linux" ]] || die "the AnixOps Agent runs on Linux only"
  case "$(uname -m)" in
    x86_64 | amd64) ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *) die "unsupported architecture $(uname -m): the Agent is released for amd64 and arm64" ;;
  esac
  if [[ ! -d "$(path /run/systemd/system)" ]] || ! command -v systemctl >/dev/null 2>&1; then
    if command -v rc-service >/dev/null 2>&1 || [[ -e "$(path /sbin/openrc-run)" ]]; then
      die "OpenRC is not supported by this installer release yet: the Agent needs systemd"
    fi
    die "systemd is required: /run/systemd/system is missing"
  fi
}

require_tools() {
  local tool missing=()
  for tool in curl sha256sum install mktemp getent groupadd useradd usermod; do
    command -v "${tool}" >/dev/null 2>&1 || missing+=("${tool}")
  done
  if ! command -v unzip >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1; then
    missing+=("unzip")
  fi
  ((${#missing[@]} == 0)) || die "missing tools: ${missing[*]} (Debian/Ubuntu: apt-get install -y ${missing[*]}; RHEL: dnf install -y ${missing[*]})"
}

# preflight is where the O2 checks (kernel, nftables, ports, clock skew,
# Control reachability) run before anything is changed.
preflight() { :; }

# fetch downloads a URL over https; curl's URL globbing is off for IPv6
# literals.
fetch() {
  curl -fsSL -g --proto '=https' --retry 3 --connect-timeout 15 -o "$2" "$1"
}

read_metadata() {
  local file="$1" key a b
  while IFS=' ' read -r key a b || [[ -n "${key}" ]]; do
    case "${key}" in
      format) [[ "${a}" == "1" ]] || die "Control's install metadata has format ${a}; download the installer from this Control again" ;;
      agent_version) META_VERSION="${a}" ;;
      grpc_target) META_GRPC="${a}" ;;
      asset) [[ "${a}" =~ ^(amd64|arm64)$ && "${b}" =~ ^anix-agent-linux-[a-z0-9-]+\.zip$ ]] && META_ASSET["${a}"]="${b}" ;;
      source) [[ "${a}" =~ ^(control|cn|github)$ && "${b}" =~ ^https://[^[:space:]\'\"]+$ ]] && META_SOURCE["${a}"]="${b%/}" ;;
      sha256) [[ "${a}" =~ ^anix-agent-linux-[a-z0-9-]+\.zip$ && "${b}" =~ ^[0-9a-f]{64}$ ]] && META_SHA256["${a}"]="${b}" ;;
      *) ;;
    esac
  done <"${file}"
  [[ "${META_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]] || die "Control's install metadata names no Agent release"
  [[ -n "${META_GRPC}" ]] || die "Control's install metadata names no gRPC target (agent_install.grpc_target)"
  [[ "${META_GRPC}" =~ ^[][A-Za-z0-9.:-]+:[0-9]{1,5}$ ]] || die "Control's gRPC target ${META_GRPC} is not host:port"
  ASSET="${META_ASSET[${ARCH}]:-}"
  [[ -n "${ASSET}" ]] || die "the Agent release ${META_VERSION} has no ${ARCH} build"
  [[ -n "${META_SOURCE[github]:-}" ]] || die "Control's install metadata has no GitHub source"
}

fetch_metadata() {
  info "Reading the Agent release from ${CONTROL_URL}"
  fetch "${CONTROL_URL}/install/agent.env" "${TMP_DIR}/agent.env" ||
    die "cannot reach ${CONTROL_URL}/install/agent.env: check the address and that this host reaches Control over https"
  read_metadata "${TMP_DIR}/agent.env"
}

select_source() {
  case "${MIRROR}" in
    control) SOURCE="${META_SOURCE[control]:-}" ;;
    cn) SOURCE="${META_SOURCE[cn]:-${META_SOURCE[control]:-}}" ;;
    github) SOURCE="${META_SOURCE[github]}" ;;
  esac
  if [[ -z "${SOURCE}" ]]; then
    SOURCE="${META_SOURCE[github]}"
    note "the ${MIRROR} mirror is not set up on Control; downloading from GitHub releases"
  elif [[ "${MIRROR}" == "cn" && -z "${META_SOURCE[cn]:-}" ]]; then
    note "Control has no mainland mirror (agent_install.cn_mirror_url); downloading from Control"
  fi
}

# expected_digest sets EXPECTED_DIGEST, the asset's SHA-256 from a trusted
# origin: Control's metadata, else the release's .dgst on GitHub. A mirror's
# own checksum is never trusted.
expected_digest() {
  if [[ -n "${META_SHA256[${ASSET}]:-}" ]]; then
    EXPECTED_DIGEST="${META_SHA256[${ASSET}]}"
    return 0
  fi
  local dgst="${TMP_DIR}/${ASSET}.dgst" digest
  fetch "${META_SOURCE[github]}/${META_VERSION}/${ASSET}.dgst" "${dgst}" ||
    die "cannot get the checksum of ${ASSET} from Control or GitHub; put the Agent release in Control's agent_install.artifact_dir so Control publishes it"
  digest="$(sed -nE 's/^SHA(2-)?256(\([^)]*\))?=[[:space:]]*([0-9a-fA-F]{64})[[:space:]]*$/\3/p' "${dgst}" | head -n 1 | tr 'A-F' 'a-f')"
  [[ "${digest}" =~ ^[0-9a-f]{64}$ ]] || die "the checksum file of ${ASSET} has no SHA-256"
  EXPECTED_DIGEST="${digest}"
}

# verify_signature checks file against a base64 raw Ed25519 signature by
# the official release key.
verify_signature() {
  local file="$1" signature="$2" key="${ANIX_INSTALL_PUBLIC_KEY:-${OFFICIAL_PUBLIC_KEY}}"
  command -v openssl >/dev/null 2>&1 || {
    note "openssl is missing: the release signature of ${ASSET} is not checked (the SHA-256 is)"
    return 0
  }
  if ! openssl pkeyutl -help 2>&1 | grep -q -- '-rawin'; then
    note "this OpenSSL cannot verify Ed25519 signatures (OpenSSL 3 needed): the SHA-256 of ${ASSET} is checked, its signature is not"
    return 0
  fi
  {
    printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'
    printf '%s' "${key}" | base64 -d
  } >"${TMP_DIR}/release-key.der" || die "the release key is not valid base64"
  tr -d ' \t\r\n' <"${signature}" | base64 -d >"${TMP_DIR}/signature.bin" 2>/dev/null || die "the signature of ${ASSET} is not base64"
  openssl pkeyutl -verify -pubin -keyform DER -inkey "${TMP_DIR}/release-key.der" -rawin \
    -in "${file}" -sigfile "${TMP_DIR}/signature.bin" >/dev/null 2>&1 ||
    die "the release signature of ${ASSET} does not verify: the download was altered; nothing was installed"
  info "Release signature verified"
}

download_agent() {
  local zip="${TMP_DIR}/${ASSET}" actual
  expected_digest
  info "Downloading the AnixOps Agent ${META_VERSION} (${ARCH}) from ${SOURCE}"
  fetch "${SOURCE}/${META_VERSION}/${ASSET}" "${zip}" || die "cannot download ${SOURCE}/${META_VERSION}/${ASSET}; try another --mirror"
  actual="$(sha256sum "${zip}" | awk '{print $1}')"
  [[ "${actual}" == "${EXPECTED_DIGEST}" ]] || die "checksum mismatch for ${ASSET}: expected ${EXPECTED_DIGEST}, got ${actual}; nothing was installed"
  info "Checksum verified (sha256 ${actual})"
  if fetch "${SOURCE}/${META_VERSION}/${ASSET}.sig" "${zip}.sig" 2>/dev/null; then
    verify_signature "${zip}" "${zip}.sig"
  else
    note "the Agent release has no signature yet: only its SHA-256 was checked"
  fi
  mkdir -p "${TMP_DIR}/agent"
  if command -v unzip >/dev/null 2>&1; then
    unzip -q -o "${zip}" -d "${TMP_DIR}/agent" || die "cannot unpack ${ASSET}"
  else
    python3 -m zipfile -e "${zip}" "${TMP_DIR}/agent" || die "cannot unpack ${ASSET}"
  fi
  [[ -f "${TMP_DIR}/agent/anix-agent" && ! -L "${TMP_DIR}/agent/anix-agent" ]] || die "${ASSET} holds no anix-agent binary"
}

# root_install tells whether this host runs, or ran, the Agent of
# anix-agent's root installer: a unit without User=anixops-agent, or its
# directories.
root_install() {
  local unit dir
  unit="$(path "${UNIT_DIR}/${SERVICE}")"
  if [[ -f "${unit}" ]] && ! grep -qxF "User=${AGENT_USER}" "${unit}"; then
    return 0
  fi
  for dir in "${ROOT_INSTALL_DIRS[@]}"; do
    [[ -d "$(path "${dir}")" ]] && return 0
  done
  return 1
}

# migrate_root_install copies a root install's identity, stream state and
# plugins to the sandboxed Agent's directories, owned by its user, before
# the new unit starts: the Agent as anixops-agent cannot read the old
# root-owned directories. The old ones are left in place.
migrate_root_install() {
  local bin args=(migrate-paths --chown "${AGENT_USER}")
  root_install || return 0
  bin="$(path "${LIB_DIR}/anix-agent")"
  if ! "${bin}" migrate-paths --help >/dev/null 2>&1; then
    note "the Agent ${META_VERSION} cannot migrate a root install's directories (no migrate-paths); it enrolls again with the token"
    MIGRATION_STATE="not migrated (the Agent has no migrate-paths)"
    return 0
  fi
  [[ -z "${ROOT}" ]] || args+=(--root "${ROOT}")
  info "Migrating the root-installed Agent's directories to ${STATE_DIR}"
  "${bin}" "${args[@]}" ||
    die "anix-agent migrate-paths could not copy the root install's directories (see above); nothing was started, fix the cause and re-run"
  MIGRATION_STATE="copied the root install's directories to ${STATE_DIR} (the old ones are left in place)"
}

ensure_users() {
  if ! getent group "${GOST_USER}" >/dev/null; then
    groupadd --system "${GOST_USER}"
  fi
  if ! getent passwd "${GOST_USER}" >/dev/null; then
    useradd --system --gid "${GOST_USER}" --home-dir "${GOST_DIR}" --no-create-home --shell /usr/sbin/nologin "${GOST_USER}"
  fi
  if ! getent group "${AGENT_USER}" >/dev/null; then
    groupadd --system "${AGENT_USER}"
  fi
  if ! getent passwd "${AGENT_USER}" >/dev/null; then
    useradd --system --gid "${AGENT_USER}" --groups "${GOST_USER}" --home-dir "${STATE_DIR}" --no-create-home \
      --shell /usr/sbin/nologin "${AGENT_USER}"
  else
    # The Agent reaches gost's API socket through the gost group; nothing
    # else may join it.
    usermod --append --groups "${GOST_USER}" "${AGENT_USER}"
  fi
}

# identity_status prints the Agent's identity record for NODE as one line of
# JSON, or nothing.
identity_status() {
  local bin
  bin="$(path "${LIB_DIR}/anix-agent")"
  [[ -x "${bin}" ]] || return 0
  "${bin}" identity --json --pki-dir "${PKI_DIR}" 2>/dev/null | tr -d '\n\t ' |
    grep -o "{[^{}]*\"node\":\"${NODE}\"[^{}]*}" | head -n 1 || true
}

# identity_field prints a string field of an identity record.
identity_field() {
  printf '%s' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -n 1 | cut -d '"' -f 4 || true
}

enrolled() {
  local state
  state="$(identity_field "$(identity_status)" state)"
  [[ "${state}" == "valid" || "${state}" == "renewal-due" ]]
}

stop_running_agent() {
  if [[ -f "$(path "${UNIT_DIR}/${SERVICE}")" ]]; then
    INSTALL_MODE="upgrade"
    # Forwarding keeps running: rules are in the kernel and gost is its own
    # unit.
    systemctl stop "${SERVICE}" >/dev/null 2>&1 || true
  fi
}

install_files() {
  install -d -m 0755 "$(path "${LIB_DIR}")" "$(path "$(dirname "${BIN_LINK}")")"
  install -m 0755 "${TMP_DIR}/agent/anix-agent" "$(path "${LIB_DIR}/anix-agent.new")"
  mv -f "$(path "${LIB_DIR}/anix-agent.new")" "$(path "${LIB_DIR}/anix-agent")"
  if [[ -f "${TMP_DIR}/agent/gost" && ! -L "${TMP_DIR}/agent/gost" ]]; then
    install -m 0755 "${TMP_DIR}/agent/gost" "$(path "${LIB_DIR}/gost.new")"
    mv -f "$(path "${LIB_DIR}/gost.new")" "$(path "${LIB_DIR}/gost")"
  fi
  ln -sfn "${LIB_DIR}/anix-agent" "$(path "${BIN_LINK}")"
  printf '%s\n' "${META_VERSION}" >"$(path "${LIB_DIR}/.release-version")"

  install -d -m 0750 "$(path "${CONFIG_DIR}")"
  chown "root:${AGENT_USER}" "$(path "${CONFIG_DIR}")"
  install -d -m 0700 "$(path "${STATE_DIR}")"
  chown "${AGENT_USER}:${AGENT_USER}" "$(path "${STATE_DIR}")"
  # Before pki and stream exist: migrate-paths copies only to new places.
  migrate_root_install
  install -d -m 0700 "$(path "${PKI_DIR}")" "$(path "${STREAM_DIR}")"
  chown "${AGENT_USER}:${AGENT_USER}" "$(path "${STATE_DIR}")" "$(path "${PKI_DIR}")" "$(path "${STREAM_DIR}")"
  install -d -m 0750 "$(path "${GOST_DIR}")"
  chown "${AGENT_USER}:${GOST_USER}" "$(path "${GOST_DIR}")"
}

render_config() {
  local node_id="${NODE#*-}"
  cat <<EOF
{
  "Log": {"Level": "info", "Output": ""},
  "Cores": [],
  "Nodes": [
    {
      "ApiHost": "${CONTROL_URL}",
      "Transport": "grpc",
      "GRPCHost": "${META_GRPC}",
      "GRPCUseTLS": true,
      "AgentControlEnabled": true,
      "AgentControlAllowInsecure": false,
      "AgentNode": "${NODE}",
      "NodeID": ${node_id},
      "AgentIdentity": {
        "Enroll": "auto",
        "CertDir": "${PKI_DIR}",
        "EnrollCredentialFile": "${CREDENTIAL_FILE}"
      },
      "AgentStream": {"StateDir": "${STREAM_DIR}"}
    }
  ]
}
EOF
}

write_config() {
  local config
  config="$(path "${CONFIG_FILE}")"
  if [[ -f "${config}" ]] && ((RESET == 0)); then
    if ! grep -q "\"AgentNode\": \"${NODE}\"" "${config}"; then
      die "this host's Agent is installed for another node (${CONFIG_FILE}); re-run with --reset and a token for ${NODE} to switch it"
    fi
    CONFIG_STATE="kept ${CONFIG_FILE}"
    return 0
  fi
  if [[ -f "${config}" ]]; then
    cp -p "${config}" "${config}.bak.$(date -u +%Y%m%dT%H%M%SZ)"
  fi
  render_config >"${config}.new"
  chmod 0640 "${config}.new"
  chown "root:${AGENT_USER}" "${config}.new"
  mv -f "${config}.new" "${config}"
  CONFIG_STATE="wrote ${CONFIG_FILE}"
}

write_credential() {
  local file tmp
  file="$(path "${CREDENTIAL_FILE}")"
  if ((RESET)); then
    find "$(path "${PKI_DIR}")" -mindepth 1 -delete
    info "Discarded this host's Agent identity (--reset)"
  elif enrolled; then
    CREDENTIAL_STATE="already enrolled as ${NODE}; the token was not used"
    rm -f "${file}"
    return 0
  fi
  [[ -n "${TOKEN}" ]] || die "this host's Agent is not enrolled: --token is required"
  tmp="$(umask 077 && mktemp "${file}.XXXXXX")"
  printf '%s\n' "${TOKEN}" >"${tmp}"
  chmod 0600 "${tmp}"
  chown "${AGENT_USER}:${AGENT_USER}" "${tmp}"
  mv -f "${tmp}" "${file}"
  CREDENTIAL_STATE="enrollment token written to ${CREDENTIAL_FILE} (mode 0600; the Agent removes it after use)"
}

# cleanup_legacy removes the legacy forward runtime of this host before the
# new Agent starts (forward-sdk.md, section 10): the named nftables tables
# and the clean agent's unit and files. Nothing else is touched.
cleanup_legacy() {
  local table unit legacy_path
  if command -v nft >/dev/null 2>&1; then
    for table in "${LEGACY_NFT_TABLES[@]}"; do
      # shellcheck disable=SC2086 # "family name" is two words on purpose.
      if nft list table ${table} >/dev/null 2>&1; then
        # shellcheck disable=SC2086
        nft delete table ${table} || die "cannot delete the legacy nftables table ${table}"
        REMOVED+=("nftables table ${table}")
      fi
    done
  else
    note "nft is not installed: no legacy nftables tables to remove"
  fi
  for unit in "${LEGACY_UNITS[@]}"; do
    if [[ -f "$(path "${UNIT_DIR}/${unit}")" || -f "$(path "/lib/systemd/system/${unit}")" || -f "$(path "/usr/lib/systemd/system/${unit}")" ]]; then
      systemctl disable --now "${unit}" >/dev/null 2>&1 || true
      rm -f "$(path "${UNIT_DIR}/${unit}")" "$(path "/lib/systemd/system/${unit}")" "$(path "/usr/lib/systemd/system/${unit}")"
      REMOVED+=("systemd unit ${unit}")
    fi
  done
  for legacy_path in "${LEGACY_PATHS[@]}"; do
    if [[ -e "$(path "${legacy_path}")" || -L "$(path "${legacy_path}")" ]]; then
      rm -rf "$(path "${legacy_path}")"
      REMOVED+=("${legacy_path}")
    fi
  done
}

render_sysctl() {
  local setting
  printf '# Written by the AnixOps installer (agent-install.sh): IP forwarding for\n'
  printf '# the forward drivers of the AnixOps Agent. Re-running the installer rewrites it.\n'
  for setting in "${SYSCTL_SETTINGS[@]}"; do
    printf '%s\n' "${setting/=/ = }"
  done
}

# enable_forwarding turns on IPv4 and IPv6 forwarding for a forward node, or
# a proxy node installed with --forward, now and at every boot. A failure
# to apply is a note, not an error: the file applies at the next boot.
enable_forwarding() {
  if [[ "${NODE}" != forward-* ]] && ((FORWARD == 0)); then
    FORWARDING_STATE="unchanged (a proxy node; --forward turns IP forwarding on)"
    return 0
  fi
  local file
  file="$(path "${SYSCTL_FILE}")"
  install -d -m 0755 "$(dirname "${file}")"
  # The Agent prints the drop-in it expects; an Agent without the command
  # gets the same settings from the installer.
  if ! "$(path "${LIB_DIR}/anix-agent")" forward sysctl-dropin >"${file}.new" 2>/dev/null ||
    ! grep -q '^net\.ipv4\.ip_forward *= *1' "${file}.new"; then
    render_sysctl >"${file}.new"
  fi
  chmod 0644 "${file}.new"
  mv -f "${file}.new" "${file}"
  FORWARDING_STATE="wrote ${SYSCTL_FILE} (${SYSCTL_SETTINGS[*]})"
  if ! command -v sysctl >/dev/null 2>&1; then
    note "sysctl is missing: IP forwarding turns on at the next boot (${SYSCTL_FILE})"
    FORWARDING_STATE+="; applies at the next boot"
    return 0
  fi
  # -e: a host without IPv6 has no net.ipv6 keys; IPv4 still applies.
  if sysctl -e -q -p "${file}" >/dev/null 2>&1; then
    FORWARDING_STATE+="; applied"
  else
    note "sysctl could not apply ${SYSCTL_FILE} now: IP forwarding turns on at the next boot (sysctl -p ${SYSCTL_FILE})"
    FORWARDING_STATE+="; applies at the next boot"
  fi
}

render_agent_unit() {
  cat <<EOF
# ${SERVICE}: the AnixOps Agent. Written by the AnixOps installer
# (agent-install.sh); re-running the installer rewrites it.
[Unit]
Description=AnixOps Agent
Documentation=https://github.com/AnixOps/anix-control/blob/go_dev/docs/guide/agent-onboarding.md
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${LIB_DIR}/anix-agent server -c ${CONFIG_FILE}
Restart=on-failure
RestartSec=5s
User=${AGENT_USER}
Group=${AGENT_USER}
# gost's API and metrics sockets are guarded by the gost group (F4b).
SupplementaryGroups=${GOST_USER}
UMask=0077
LimitNOFILE=1048576
# /run/anixops-agent: the plugins' sockets (ProtectSystem=strict).
RuntimeDirectory=anixops-agent
RuntimeDirectoryMode=0750

# Privileges (H13): nftables and tc over netlink, low ports; nothing else.
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=yes

# Sandbox (forward-sdk.md, section 14).
ProtectSystem=strict
ReadWritePaths=${STATE_DIR} ${GOST_DIR}
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF
}

# render_gost_unit is contracts/forward/v1/gost/anixops-gost.service, the
# unit sdk/forward/driver/gost renders (a Control test keeps them equal).
render_gost_unit() {
  cat <<'EOF'
# anixops-gost.service: the gost of the AnixOps Agent's forward driver.
# Rendered by sdk/forward/driver/gost (UnitFile); installed by the AnixOps
# installer. The Agent starts, reloads (SIGHUP) and stops it; it is a unit
# of its own so that restarting or upgrading the Agent keeps forwarding.
[Unit]
Description=AnixOps forward gost (anixops-forward-driver gost v1)
Documentation=https://github.com/AnixOps/anix-control/blob/go_dev/docs/architecture/forward-sdk.md
After=network-online.target
Wants=network-online.target
# Without a configuration there is nothing to serve: the driver writes it
# on its first apply and deletes it on Remove.
ConditionPathExists=/var/lib/anixops-gost/gost.json

[Service]
Type=exec
ExecStart=/usr/lib/anixops-agent/gost -C /var/lib/anixops-gost/gost.json
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=2s
TimeoutStopSec=10s
User=anixops-gost
Group=anixops-gost
UMask=0007
RuntimeDirectory=anixops-gost
RuntimeDirectoryMode=0750
LimitNOFILE=1048576

# Privileges: binding ports below 1024, nothing else.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes

# Sandbox: the file system is read only but for RuntimeDirectory.
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
ProtectProc=invisible
ProcSubset=pid
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RemoveIPC=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged
SystemCallErrorNumber=EPERM

[Install]
WantedBy=multi-user.target
EOF
}

# render_polkit_rule lets the Agent user start, stop and reload
# anixops-gost.service, and nothing else.
render_polkit_rule() {
  cat <<EOF
// Written by the AnixOps installer: ${AGENT_USER} may start, stop and
// reload ${GOST_SERVICE} only.
polkit.addRule(function(action, subject) {
  if (action.id == "org.freedesktop.systemd1.manage-units" &&
      action.lookup("unit") == "${GOST_SERVICE}" &&
      subject.user == "${AGENT_USER}") {
    var verb = action.lookup("verb");
    if (verb == "start" || verb == "stop" || verb == "restart" ||
        verb == "reload" || verb == "try-restart" || verb == "reload-or-restart") {
      return polkit.Result.YES;
    }
  }
});
EOF
}

install_units() {
  install -d -m 0755 "$(path "${UNIT_DIR}")"
  render_agent_unit >"$(path "${UNIT_DIR}/${SERVICE}")"
  render_gost_unit >"$(path "${UNIT_DIR}/${GOST_SERVICE}")"
  chmod 0644 "$(path "${UNIT_DIR}/${SERVICE}")" "$(path "${UNIT_DIR}/${GOST_SERVICE}")"
  if [[ -d "$(path "$(dirname "${POLKIT_RULE}")")" ]]; then
    render_polkit_rule >"$(path "${POLKIT_RULE}")"
    chmod 0644 "$(path "${POLKIT_RULE}")"
  else
    note "polkit is not installed: the Agent cannot start or reload ${GOST_SERVICE} until it is (apt-get install -y polkitd)"
  fi
  systemctl daemon-reload
  # gost stays inert until the Agent writes its configuration.
  systemctl enable "${GOST_SERVICE}" >/dev/null 2>&1 || note "cannot enable ${GOST_SERVICE}"
  systemctl enable "${SERVICE}" >/dev/null
  systemctl restart "${SERVICE}"
}

wait_enrolled() {
  local waited=0
  info "Waiting for the Agent to enroll with Control (up to ${TIMEOUT}s)"
  while ((waited < TIMEOUT)); do
    if enrolled; then
      return 0
    fi
    sleep 2
    waited=$((waited + 2))
  done
  printf '[anixops] error: the Agent did not enroll within %ss.\n' "${TIMEOUT}" >&2
  printf '[anixops]   Check: journalctl -u %s -n 50 --no-pager\n' "${SERVICE}" >&2
  printf '[anixops]   A used or expired token needs a new command from the node page; re-run it (the install is kept).\n' >&2
  exit 1
}

print_summary() {
  local status item
  status="$(identity_status)"
  info "AnixOps Agent ${META_VERSION} is running (${INSTALL_MODE})"
  printf '  node:        %s\n' "${NODE}"
  printf '  identity:    %s (serial %s, expires %s)\n' "$(identity_field "${status}" spiffe_id)" \
    "$(identity_field "${status}" serial)" "$(identity_field "${status}" not_after)"
  printf '  control:     %s (gRPC %s)\n' "${CONTROL_URL}" "${META_GRPC}"
  printf '  config:      %s\n' "${CONFIG_STATE}"
  printf '  credential:  %s\n' "${CREDENTIAL_STATE}"
  printf '  forwarding:  %s\n' "${FORWARDING_STATE}"
  if [[ -n "${MIGRATION_STATE}" ]]; then
    printf '  migration:   %s\n' "${MIGRATION_STATE}"
  fi
  printf '  service:     %s (systemctl status %s; journalctl -u %s)\n' "${SERVICE}" "${SERVICE}" "${SERVICE}"
  if ((${#REMOVED[@]} == 0)); then
    printf '  legacy:      nothing to remove\n'
  else
    printf '  legacy:      removed\n'
    for item in "${REMOVED[@]}"; do
      printf '                 - %s\n' "${item}"
    done
  fi
  for item in "${NOTES[@]}"; do
    printf '  note:        %s\n' "${item}"
  done
}

cleanup_tmp() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}

on_error() {
  printf '[anixops] error: the installer stopped at line %s. Fix the cause and re-run the same command; it is safe to repeat.\n' "$1" >&2
}

take_lock() {
  command -v flock >/dev/null 2>&1 || return 0
  mkdir -p "$(path "$(dirname "${LOCK_FILE}")")"
  exec 9>"$(path "${LOCK_FILE}")"
  flock -n 9 || die "another installer run is in progress"
}

# require_token stops a first install without a token before anything is
# changed.
require_token() {
  if [[ -z "${TOKEN}" && ! -f "$(path "${CONFIG_FILE}")" ]]; then
    die "this host's Agent is not enrolled: --token is required (copy the command from the node page)"
  fi
}

main() {
  parse_args "$@"
  require_root
  detect_platform
  require_tools
  require_token
  take_lock
  preflight
  TMP_DIR="$(mktemp -d)"
  trap cleanup_tmp EXIT
  trap 'on_error "${LINENO}"' ERR
  fetch_metadata
  select_source
  download_agent
  ensure_users
  stop_running_agent
  install_files
  write_config
  write_credential
  cleanup_legacy
  enable_forwarding
  install_units
  wait_enrolled
  print_summary
}

if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
  main "$@"
fi
