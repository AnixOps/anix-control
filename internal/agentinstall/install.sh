#!/usr/bin/env bash
# AnixOps Agent installer: one-command node onboarding.
#
#   curl -fsSL https://<control>/install.sh | sudo bash -s -- \
#     -u https://<control> -t anixjoin_... \
#     [--name hk-01] [--host 203.0.113.5] [--mirror control|cn|github] \
#     [--reset] [--forward] [--timeout 180] [--offline bundle.tar.gz] \
#     [--port-range 20000-30000] [--no-accept-ra] [--skip-preflight]
#   sudo bash install.sh uninstall [--purge]
#
# Copy the command from Control (节点 › 接入节点). Its token says what the
# machine becomes: a node group's join token (anixjoin_...) makes it a new
# node of the group, the same command on any number of machines; a one-time
# enrollment token (anixagt_...) enrolls the existing node it is bound to
# (moving a node to a new machine, or after a credential rotation). The
# script first checks the host (preflight: systemd, kernel, nftables,
# polkit, firewalls, disk, clock skew and Control's https and gRPC
# addresses) and changes nothing when a check fails, except that a forward
# node without nft gets nftables from the package manager first (also when
# the join is what makes the host a forward node). It then installs the
# AnixOps Agent as a systemd service running as the unprivileged user
# anixops-agent (ambient CAP_NET_ADMIN and CAP_NET_BIND_SERVICE, sandboxed),
# removes the legacy forward runtime of this host (the nftables tables
# inet v2b_forward, ip v2b_forward and ip anixops_forward, the clean
# agent's v2forward-agent service, and the gost an earlier install set up:
# anixops-gost.service, its binary, directory and account), migrates the
# directories of an earlier root install of the Agent, and joins Control: 'anix-agent join', run as
# anixops-agent, exchanges the token for the node's certificate and names
# the node. It then writes the configuration for that node, turns on IP
# forwarding for a forward node (/etc/sysctl.d/90-anixops-forward.conf; a
# proxy node with --forward) and starts the Agent. Running the same command
# again is safe: it upgrades the Agent in place and keeps its identity
# unless --reset is given. --offline installs from a bundle made by
# 'anix-control agent offline-bundle' (no downloads; joining still needs
# Control). 'uninstall' removes the Agent and keeps its identity; --purge
# also removes its state, users and the forwarding objects it owns.
#
# This file is byte for byte the release asset agent-install.sh. Verify it
# before running it:
#   curl -fsSLO https://<control>/install.sh
#   curl -fsSLO https://<control>/install.sh.sig
#   printf '%s' 'MCowBQYDK2VwAyEAjW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE=' | base64 -d >official.der
#   base64 -d install.sh.sig >install.sh.sig.bin
#   openssl pkeyutl -verify -pubin -keyform DER -inkey official.der -rawin \
#     -in install.sh -sigfile install.sh.sig.bin
#
# Documentation: docs/guide/agent-onboarding.md in the anix-control repository.

set -Eeuo pipefail

# The official AnixOps release key (plugins.official_public_key): base64 of
# the raw Ed25519 public key. It verifies Agent release signatures.
readonly OFFICIAL_PUBLIC_KEY="jW26nr2tbthASoeq6RmIpx8Ah+uhPNIv9V1ewRVb1VE="

# ROOT prefixes every path the script writes; tests set it to a temporary
# directory. Paths written into configuration and units never carry it.
ROOT="${ANIX_INSTALL_ROOT:-}"

readonly AGENT_USER="anixops-agent"
readonly SERVICE="anix-agent.service"
# The privileged updater (forward-sdk.md section 9, O4): the Agent runs as
# its own user under ProtectSystem=strict and cannot replace its binary.
# For a Control-pushed upgrade it stages the verified release and writes
# UPDATER_REQUEST; the path unit starts the root oneshot, which runs the
# installed (root-owned) anix-agent to verify the release again, keep the
# previous binary for a rollback and restart the Agent.
readonly UPDATER_SERVICE="anixops-agent-updater.service"
readonly UPDATER_PATH="anixops-agent-updater.path"
readonly LIB_DIR="/usr/lib/anixops-agent"
readonly BIN_LINK="/usr/local/bin/anix-agent"
readonly CONFIG_DIR="/etc/anixops/agent"
readonly CONFIG_FILE="${CONFIG_DIR}/config.json"
readonly STATE_DIR="/var/lib/anixops-agent"
readonly PKI_DIR="${STATE_DIR}/pki"
readonly STREAM_DIR="${STATE_DIR}/stream"
# The token reaches 'anix-agent join' in this file (0600, the Agent's),
# removed as soon as the join returns; it is never written to the
# configuration. enroll.credential is the file earlier releases left.
readonly TOKEN_FILE="${STATE_DIR}/join.token"
readonly LEGACY_CREDENTIAL_FILE="${STATE_DIR}/enroll.credential"
readonly UPGRADE_DIR="${STATE_DIR}/upgrade"
readonly UPDATER_REQUEST="${UPGRADE_DIR}/request.json"
readonly UNIT_DIR="/etc/systemd/system"
readonly POLKIT_RULE="/etc/polkit-1/rules.d/50-anixops-agent.rules"
readonly LOCK_FILE="/run/anixops-agent-install.lock"
# IP forwarding for forward nodes: nftables DNAT and the relays route
# packets between interfaces.
readonly SYSCTL_FILE="/etc/sysctl.d/90-anixops-forward.conf"
readonly SYSCTL_SETTINGS=("net.ipv4.ip_forward=1" "net.ipv6.conf.all.forwarding=1")

# The legacy forward runtime the new Agent replaces (forward-sdk.md,
# section 10). Only these named objects are removed, never anything else.
readonly LEGACY_NFT_TABLES=("inet v2b_forward" "ip v2b_forward" "ip anixops_forward")
readonly LEGACY_UNITS=("v2forward-agent.service" "anixops-gost.service")
readonly LEGACY_PATHS=("/etc/v2board-forward-agent" "/usr/local/bin/v2forward-agent" "/usr/lib/anixops-agent/gost" "/var/lib/anixops-gost")
# The account of the gost an earlier install set up (the network model
# replaced gost).
readonly LEGACY_USER="anixops-gost"
# The directories of an Agent installed by anix-agent's own root installer
# (scripts/install.sh): 'anix-agent migrate-paths' copies them to STATE_DIR.
readonly ROOT_INSTALL_DIRS=("/var/lib/anix-agent" "/var/lib/anixops/plugins")

# What the forward drivers create on a host (sdk/forward/driver/nftables):
# one nftables table, which carries the ownership comment, and one HTB root
# qdisc with a fixed handle per rate-limited interface. uninstall --purge
# removes only these, never a table or qdisc without the mark.
readonly FWD_TABLE="inet anixops_fwd"
readonly FWD_TABLE_COMMENT="anixops-forward-driver v1"
readonly FWD_TC_HANDLE="af00:"

# Preflight limits (forward-sdk.md, section 9).
readonly MIN_KERNEL="5.10"   # the nftables driver's table, counter and set element comments
readonly MIN_NFT="0.9.7"     # the same comments in nft
readonly MIN_SYSTEMD=240     # Type=exec in the units
readonly MIN_POLKIT="0.106"  # JavaScript rules (/etc/polkit-1/rules.d)
readonly SKEW_WARN=30        # seconds
readonly SKEW_FAIL=300       # seconds: TLS certificates stop verifying
readonly MIN_FREE_KB=$((200 * 1024))

CONTROL_URL=""
# NODE is the node this host runs as, proxy-<id> or forward-<id>: from the
# existing configuration on a re-run, else from the join.
NODE=""
TOKEN="${ANIX_AGENT_TOKEN:-}"
TOKEN_ON_COMMAND_LINE=0
NODE_NAME=""
CONNECT_HOST=""
MIRROR="control"
RESET=0
FORWARD=0
TIMEOUT=180
TMP_DIR=""
ACTION="install"
PURGE=0
OFFLINE=""
SKIP_PREFLIGHT=0
ACCEPT_RA=1
PORT_RANGE=""
PREFLIGHT_FAILS=0
FORWARD_CHECKED=0

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
JOIN_STATE=""
JOIN_GROUP=""
JOIN_APPROVAL=""
JOIN_SPIFFE=""
JOIN_NOT_AFTER=""
PUBLIC_ADDRESSES=()
FORWARDING_STATE=""
MIGRATION_STATE=""
FORWARDING_NOTE=""
REMOVED=()
KEPT=()
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

# have tells whether a command is available.
have() { command -v "$1" >/dev/null 2>&1; }

# version_ge tells whether version $1 is at least $2.
version_ge() {
  [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n 1)" == "$2" ]]
}

# forward_node tells whether this host forwards: a forward node, or a proxy
# node installed with --forward. Before a fresh join the node is not known
# yet: then only --forward says so.
forward_node() { [[ "${NODE}" == forward-* ]] || ((FORWARD)); }

usage() {
  cat <<'EOF'
Usage: install.sh -u <https://control> -t <anixjoin_...|anixagt_...> [options]
       install.sh uninstall [--purge]

  -u, --control URL     the Control address nodes reach (from the copied command)
  -t, --token TOKEN     a node group's join token (anixjoin_...: this host becomes a
                        new node of the group), or a node's one-time enrollment token
                        (anixagt_...: this host becomes that node); or ANIX_AGENT_TOKEN
  --name NAME           the new node's name (default: this host's name; Control adds
                        -2, -3 when the group has it already)
  --host HOST           the address clients connect to (default: this host's public
                        address as Control sees it)
  --mirror M            where the Agent is downloaded from: control (default), cn, github
  --offline FILE        install from an offline bundle (anix-control agent offline-bundle);
                        nothing is downloaded, joining still needs Control
  --reset               discard this host's Agent identity and join again (needs a token);
                        with a join token Control always creates a new node
  --forward             turn on IP forwarding on a proxy node too (always on for a
                        forward node)
  --no-accept-ra        do not write net.ipv6.conf.<if>.accept_ra = 2 for SLAAC
                        interfaces when forwarding is on (by default it is written, so
                        the IPv6 default route from router advertisements keeps working)
  --port-range FROM-TO  the node's forward port range: preflight lists ports in use there
  --skip-preflight      install although a preflight check fails (not recommended)
  --timeout SECONDS     how long the join may take (default 180)
  -h, --help            show this help

Running the same command again upgrades the Agent in place and keeps its
identity; the token is then not needed and stays unused.

uninstall stops and removes the Agent and its files and keeps the identity, configuration and state for a later install;
--purge also removes those, the users, the sysctl drop-in and the forwarding
objects the drivers created (the nftables table inet anixops_fwd and the tc
root qdiscs af00:, only when they carry the drivers' marks).
EOF
}

parse_args() {
  if [[ "${1:-}" == "uninstall" ]]; then
    ACTION="uninstall"
    shift
    parse_uninstall_args "$@"
    return 0
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
      --reset | --forward | --accept-ra | --no-accept-ra | --skip-preflight)
        case "${flag}" in
          --reset) RESET=1 ;;
          --forward) FORWARD=1 ;;
          # The default since accept_ra became automatic; kept for old commands.
          --accept-ra) ACCEPT_RA=1 ;;
          --no-accept-ra) ACCEPT_RA=0 ;;
          --skip-preflight) SKIP_PREFLIGHT=1 ;;
        esac
        shift
        continue
        ;;
      --*=*)
        value="${flag#*=}"
        flag="${flag%%=*}"
        shift
        ;;
      -u | -t | --control | --token | --name | --host | --mirror | --timeout | --offline | --port-range | --node | --group)
        (($# >= 2)) || die 2 "${flag} needs a value"
        value="$2"
        shift 2
        ;;
      *)
        die 2 "unknown argument: ${flag} (see --help)"
        ;;
    esac
    case "${flag}" in
      -u | --control) CONTROL_URL="${value%/}" ;;
      -t | --token)
        TOKEN="${value}"
        TOKEN_ON_COMMAND_LINE=1
        ;;
      --name) NODE_NAME="${value}" ;;
      --host) CONNECT_HOST="${value}" ;;
      --mirror) MIRROR="${value}" ;;
      --timeout) TIMEOUT="${value}" ;;
      --offline) OFFLINE="${value}" ;;
      --port-range) PORT_RANGE="${value}" ;;
      --node | --group) die 2 "${flag} is gone: the token names its node or its node group. Copy the current command from Control (install.sh -u <control> -t <token>)" ;;
      *) die 2 "unknown argument: ${flag} (see --help)" ;;
    esac
  done
  validate_args
}

parse_uninstall_args() {
  while (($# > 0)); do
    case "$1" in
      --purge) PURGE=1 ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die 2 "unknown argument to uninstall: $1 (uninstall [--purge])" ;;
    esac
    shift
  done
}

validate_args() {
  [[ -n "${CONTROL_URL}" ]] || die 2 "-u (--control) is required (copy the command from Control)"
  [[ "${CONTROL_URL}" =~ ^https://[^/?#[:space:]\'\"]+$ ]] || die 2 "-u (--control) must be https://host[:port], not ${CONTROL_URL}"
  if [[ -n "${TOKEN}" && ! "${TOKEN}" =~ ^anixjoin_[A-Za-z0-9_-]{43}$ && ! "${TOKEN}" =~ ^anixagt_[A-Za-z0-9_-]{20,}$ ]]; then
    die 2 "-t (--token) is not an AnixOps join token (anixjoin_...) or enrollment token (anixagt_...)"
  fi
  if [[ -n "${NODE_NAME}" ]] && { ((${#NODE_NAME} > 64)) || [[ "${NODE_NAME}" == *[[:cntrl:]]* ]]; }; then
    die 2 "--name must be at most 64 characters without control characters"
  fi
  if [[ -n "${CONNECT_HOST}" && ! "${CONNECT_HOST}" =~ ^[A-Za-z0-9._:-]{1,253}$ ]]; then
    die 2 "--host must be a host name or an IP address (without brackets or a port)"
  fi
  case "${MIRROR}" in
    control | cn | github) ;;
    *) die 2 "--mirror must be control, cn or github" ;;
  esac
  [[ "${TIMEOUT}" =~ ^[1-9][0-9]{0,4}$ ]] || die 2 "--timeout must be a number of seconds"
  if ((RESET)) && [[ -z "${TOKEN}" ]]; then
    die 2 "--reset needs a token (-t): the host joins again"
  fi
  if [[ -n "${OFFLINE}" && ! -f "${OFFLINE}" ]]; then
    die 2 "--offline ${OFFLINE}: no such file (copy the bundle to this host first)"
  fi
  if [[ -n "${PORT_RANGE}" ]]; then
    if [[ ! "${PORT_RANGE}" =~ ^[1-9][0-9]{0,4}-[1-9][0-9]{0,4}$ ]] || ((${PORT_RANGE%-*} > ${PORT_RANGE#*-} || ${PORT_RANGE#*-} > 65535)); then
      die 2 "--port-range must be FROM-TO with 1 <= FROM <= TO <= 65535"
    fi
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
  if [[ ! -d "$(path /run/systemd/system)" ]] || ! have systemctl; then
    if have rc-service || [[ -e "$(path /sbin/openrc-run)" ]]; then
      die "OpenRC is not supported by this installer release yet: the Agent needs systemd"
    fi
    die "systemd is required: /run/systemd/system is missing"
  fi
}

require_tools() {
  local tool missing=()
  for tool in curl sha256sum install mktemp getent groupadd useradd usermod; do
    have "${tool}" || missing+=("${tool}")
  done
  # The join runs as the Agent's user.
  if ! have runuser && ! have setpriv; then
    missing+=("util-linux")
  fi
  if ! have unzip && ! have python3; then
    missing+=("unzip")
  fi
  if [[ -n "${OFFLINE}" ]]; then
    # An offline bundle is trusted only through its signatures.
    have tar || missing+=("tar")
    have openssl || missing+=("openssl")
  fi
  ((${#missing[@]} == 0)) || die "missing tools: ${missing[*]} (Debian/Ubuntu: apt-get install -y ${missing[*]}; RHEL: dnf install -y ${missing[*]})"
}

# Preflight (forward-sdk.md, section 9): checks that run after Control's
# metadata is read and before anything on the host changes. A failed check
# prints its reason and a fix and stops the installer once every check ran;
# a warning goes to the summary.

pf_ok() { info "preflight: ok    $*"; }
pf_skip() { info "preflight: skip  $*"; }
pf_warn() { note "preflight: $*"; }
pf_fail() {
  PREFLIGHT_FAILS=$((PREFLIGHT_FAILS + 1))
  printf '[anixops] preflight: FAIL  %s\n' "$1" >&2
  if [[ -n "${2:-}" ]]; then
    printf '[anixops]   fix: %s\n' "$2" >&2
  fi
}

check_systemd() {
  local version
  version="$(systemctl --version 2>/dev/null | awk 'NR == 1 {print $2}')" || version=""
  version="${version%%[!0-9]*}"
  if [[ -z "${version}" ]]; then
    pf_warn "cannot read the systemd version (systemctl --version)"
  elif ((version < MIN_SYSTEMD)); then
    pf_fail "systemd ${version} is older than ${MIN_SYSTEMD}: the Agent's units use Type=exec" \
      "use a distribution with systemd ${MIN_SYSTEMD} or later (Debian 11, Ubuntu 20.04, RHEL 9 or later)"
  else
    pf_ok "systemd ${version}"
  fi
}

check_kernel() {
  local release version
  release="$(uname -r)" || release=""
  if [[ ! "${release}" =~ ^([0-9]+)\.([0-9]+) ]]; then
    pf_warn "cannot read the kernel version (uname -r: ${release})"
    return 0
  fi
  version="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
  if version_ge "${version}" "${MIN_KERNEL}"; then
    pf_ok "Linux ${release}"
  elif forward_node; then
    pf_fail "Linux ${release} is older than ${MIN_KERNEL}: the nftables forward driver needs ${MIN_KERNEL} or later" \
      "upgrade the kernel (Debian 11 backports, Ubuntu 22.04 or later)"
  else
    pf_warn "Linux ${release} is older than ${MIN_KERNEL}: this node cannot forward with nftables later"
  fi
}

# install_nftables installs nft with the host's package manager on a
# forward node that lacks it, before the checks: the nftables forward driver
# needs it, and a host that only learns it is a forward node at the join
# would otherwise stop after joining. A failed install leaves nft missing,
# which check_nftables reports with the command to run.
install_nftables() {
  have nft && return 0
  local rc=0
  info "nft is missing: installing nftables for the forward driver"
  if have apt-get; then
    DEBIAN_FRONTEND=noninteractive apt-get install -y nftables >/dev/null 2>&1 ||
      { DEBIAN_FRONTEND=noninteractive apt-get update >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y nftables >/dev/null 2>&1; } || rc=$?
  elif have dnf; then
    dnf install -y nftables >/dev/null 2>&1 || rc=$?
  elif have yum; then
    yum install -y nftables >/dev/null 2>&1 || rc=$?
  elif have apk; then
    apk add --no-cache nftables >/dev/null 2>&1 || rc=$?
  elif have pacman; then
    pacman -S --noconfirm --needed nftables >/dev/null 2>&1 || rc=$?
  elif have zypper; then
    zypper --non-interactive install nftables >/dev/null 2>&1 || rc=$?
  else
    note "no known package manager to install nftables with"
    return 0
  fi
  if ((rc == 0)) && have nft; then
    note "installed nftables (the nftables forward driver needs nft)"
  else
    note "installing nftables failed (exit ${rc})"
  fi
  return 0
}

check_nftables() {
  local version
  if ! have nft; then
    pf_fail "nft is missing: the nftables forward driver needs it" "apt-get install -y nftables (RHEL: dnf install -y nftables)"
  else
    version="$(nft --version 2>/dev/null)" || version=""
    if [[ "${version}" =~ v([0-9]+(\.[0-9]+)+) ]]; then
      version="${BASH_REMATCH[1]}"
      if version_ge "${version}" "${MIN_NFT}"; then
        pf_ok "nftables ${version}"
      else
        pf_fail "nftables ${version} is older than ${MIN_NFT}: the forward driver needs table, counter and set element comments" \
          "install nftables ${MIN_NFT} or later (Debian 11, Ubuntu 22.04 or later)"
      fi
    else
      pf_warn "cannot read the nftables version (nft --version)"
    fi
  fi
  if ! have tc; then
    pf_warn "tc is missing: bandwidth limits are off on this node (apt-get install -y iproute2)"
  fi
  if [[ -e "$(path /proc/sys/net/netfilter/nf_conntrack_max)" ]]; then
    pf_ok "conntrack (nf_conntrack loaded)"
  elif have modinfo && modinfo nf_conntrack >/dev/null 2>&1; then
    pf_ok "conntrack (nf_conntrack loads on first use)"
  else
    pf_warn "conntrack (nf_conntrack) is neither loaded nor installed as a module: nftables DNAT needs it"
  fi
}

check_polkit() {
  local version
  if ! have pkaction; then
    pf_warn "polkit is not installed: 诊断 cannot restart the Agent (apt-get install -y polkitd)"
    return 0
  fi
  version="$(pkaction --version 2>/dev/null)" || version=""
  if [[ ! "${version}" =~ ([0-9]+(\.[0-9]+)*) ]]; then
    pf_warn "cannot read the polkit version (pkaction --version)"
    return 0
  fi
  version="${BASH_REMATCH[1]}"
  if version_ge "${version}" "${MIN_POLKIT}"; then
    pf_ok "polkit ${version}"
  else
    # A .pkla file grants an action for every unit; the installer writes
    # none rather than let the Agent manage any unit.
    pf_warn "polkit ${version} reads no rules from /etc/polkit-1/rules.d (${MIN_POLKIT} or later): 诊断 cannot restart the Agent. Upgrade polkit (Ubuntu 24.04, Debian 12)"
  fi
}

check_firewalls() {
  local policy warned=0
  if systemctl is-active --quiet firewalld 2>/dev/null; then
    pf_warn "firewalld is active: its zone must forward the traffic (firewall-cmd --permanent --zone=<zone> --add-forward; firewall-cmd --reload)"
    warned=1
  fi
  if grep -qsx 'ENABLED=yes' "$(path /etc/ufw/ufw.conf)" &&
    grep -qsx 'DEFAULT_FORWARD_POLICY="DROP"' "$(path /etc/default/ufw)"; then
    pf_warn "ufw is active with DEFAULT_FORWARD_POLICY=\"DROP\": forwarded traffic is dropped (set DEFAULT_FORWARD_POLICY=\"ACCEPT\" in /etc/default/ufw and run ufw reload, or add ufw route rules)"
    warned=1
  fi
  if have iptables; then
    policy="$(iptables -S FORWARD 2>/dev/null | awk 'NR == 1')" || policy=""
    if [[ "${policy}" == "-P FORWARD DROP" ]]; then
      pf_warn "the iptables FORWARD policy is DROP (Docker sets it): forwarded packets are dropped (accept them in DOCKER-USER, or iptables -P FORWARD ACCEPT)"
      warned=1
    fi
  fi
  ((warned)) || pf_ok "no firewall manager drops forwarded traffic"
}

# slaac_interfaces prints the interfaces with an IPv6 default route learned
# from router advertisements.
slaac_interfaces() {
  have ip || return 0
  { ip -6 route show default proto ra 2>/dev/null || true; } |
    awk '{for (i = 1; i < NF; i++) if ($i == "dev") print $(i + 1)}' | sort -u
}

check_ipv6_ra() {
  local iface value pending=()
  [[ -d "$(path /proc/sys/net/ipv6)" ]] || return 0
  while IFS= read -r iface; do
    [[ -n "${iface}" ]] || continue
    value="$(cat "$(path "/proc/sys/net/ipv6/conf/${iface}/accept_ra")" 2>/dev/null)" || value=""
    [[ "${value}" == "2" ]] || pending+=("${iface}")
  done < <(slaac_interfaces)
  if ((${#pending[@]} == 0)); then
    pf_ok "IPv6 router advertisements (no SLAAC interface needs accept_ra = 2)"
  elif ((ACCEPT_RA)); then
    pf_ok "IPv6 router advertisements: the drop-in sets accept_ra = 2 on ${pending[*]}"
  else
    pf_warn "IPv6 forwarding makes the kernel ignore router advertisements on ${pending[*]} (accept_ra is not 2): the IPv6 default route learned by SLAAC expires. Re-run without --no-accept-ra to write net.ipv6.conf.<if>.accept_ra = 2 to ${SYSCTL_FILE}"
  fi
}

check_ports() {
  local from to used=()
  if [[ -z "${PORT_RANGE}" ]]; then
    pf_skip "ports (the Agent listens on no port; --port-range checks the node's forward port range)"
    return 0
  fi
  if ! have ss; then
    pf_warn "ss is missing: the forward port range ${PORT_RANGE} was not checked (apt-get install -y iproute2)"
    return 0
  fi
  from="${PORT_RANGE%-*}"
  to="${PORT_RANGE#*-}"
  mapfile -t used < <({ ss -Htuln 2>/dev/null || true; } |
    awk -v from="${from}" -v to="${to}" '{n = split($5, a, ":"); p = a[n] + 0; if (p >= from && p <= to) print p}' | sort -nu)
  if ((${#used[@]} == 0)); then
    pf_ok "no listener in the forward port range ${PORT_RANGE}"
  else
    pf_warn "ports ${used[*]} of the forward port range ${PORT_RANGE} are in use on this host: forwards on them fail; stop those services or narrow the node's port range in Control"
  fi
}

# free_kb prints the free space, in KiB, of the file system that holds the
# nearest existing directory of $1.
free_kb() {
  local dir="$1"
  while [[ ! -d "${dir}" && "${dir}" != "/" && -n "${dir}" ]]; do
    dir="$(dirname "${dir}")"
  done
  df -Pk "${dir:-/}" 2>/dev/null | awk 'NR == 2 {print $4}'
}

check_disk() {
  local label dir free
  for label in "${LIB_DIR}" "download directory"; do
    dir="$(path "${LIB_DIR}")"
    [[ "${label}" == "${LIB_DIR}" ]] || dir="${TMP_DIR}"
    free="$(free_kb "${dir}")" || free=""
    if [[ ! "${free}" =~ ^[0-9]+$ ]]; then
      pf_warn "cannot read the free space for ${label} (df)"
    elif ((free < MIN_FREE_KB)); then
      pf_fail "only $((free / 1024)) MiB free for ${label}: the Agent needs about $((MIN_FREE_KB / 1024)) MiB" \
        "free space on that file system (apt-get clean; journalctl --vacuum-size=100M)"
    else
      pf_ok "$((free / 1024)) MiB free for ${label}"
    fi
  done
}

# check_control checks Control's https address (online it was read already)
# and the gRPC target the Agent enrolls at, both with TLS verification.
check_control() {
  local args=(-sS -g -o /dev/null --connect-timeout 10 -m 20) rc=0 http2=0 host="${META_GRPC%:*}"
  if [[ -n "${OFFLINE}" ]]; then
    if fetch "${CONTROL_URL}/install/agent.env" /dev/null "${TMP_DIR}/control.headers" 2>/dev/null; then
      pf_ok "Control ${CONTROL_URL} reachable over https"
    else
      pf_warn "cannot reach ${CONTROL_URL} over https: clock skew is not checked; the Agent enrolls over gRPC, checked next"
    fi
  else
    pf_ok "Control ${CONTROL_URL} reachable over https (certificate verified)"
  fi
  if curl -V 2>/dev/null | grep -qw HTTP2; then
    args+=(--http2)
    http2=1
  fi
  curl "${args[@]}" "https://${META_GRPC}/" 2>/dev/null || rc=$?
  case "${rc}" in
    0 | 16 | 52 | 56 | 92)
      pf_ok "gRPC target ${META_GRPC} reachable over TLS (certificate verified)"
      ;;
    6)
      pf_fail "cannot resolve ${host}, the gRPC target the Agent enrolls at" \
        "check DNS on this host, or set agent_install.grpc_target on Control to an address nodes resolve"
      ;;
    7 | 28)
      pf_fail "cannot connect to the gRPC target ${META_GRPC}: the Agent enrolls and connects there" \
        "open the port from this host to Control (grpc.port), or set agent_install.grpc_target on Control"
      ;;
    35)
      if ((http2)); then
        pf_fail "the TLS handshake with the gRPC target ${META_GRPC} failed" \
          "check that the gRPC listener has TLS (grpc.tls_cert_file) and that nothing intercepts the connection"
      else
        pf_warn "could not finish a TLS handshake with the gRPC target ${META_GRPC} (this curl has no HTTP/2, which gRPC needs): it was not checked"
      fi
      ;;
    51 | 58 | 60 | 77 | 83 | 90 | 91)
      pf_fail "the certificate of the gRPC target ${META_GRPC} does not verify: the Agent checks it against the system CAs" \
        "give the gRPC listener a certificate for ${host} from a public CA (grpc.tls_cert_file), or add its CA to this host's trust store (update-ca-certificates)"
      ;;
    *)
      pf_warn "the gRPC target ${META_GRPC} check was inconclusive (curl exit ${rc})"
      ;;
  esac
}

# check_clock compares this host's clock with the Date header of Control's
# answer: certificates stop verifying when they differ by minutes.
check_clock() {
  local headers date control now skew
  headers="${TMP_DIR}/control.headers"
  [[ -n "${OFFLINE}" ]] || headers="${TMP_DIR}/agent.env.headers"
  date="$(grep -i '^date:' "${headers}" 2>/dev/null | tail -n 1 | cut -d ' ' -f 2- | tr -d '\r')" || date=""
  control="$(date -u -d "${date}" +%s 2>/dev/null)" || control=""
  if [[ -z "${date}" || ! "${control}" =~ ^[0-9]+$ ]]; then
    pf_warn "Control sent no readable Date header: the clock skew was not checked"
    return 0
  fi
  now="$(date -u +%s)"
  skew=$((now - control))
  ((skew >= 0)) || skew=$((-skew))
  if ((skew > SKEW_FAIL)); then
    pf_fail "this host's clock is ${skew}s off Control's: TLS certificates and the Agent's certificate do not verify" \
      "synchronize the clock (timedatectl set-ntp true, or chrony/ntpd), then check it with: date -u"
  elif ((skew > SKEW_WARN)); then
    pf_warn "this host's clock is ${skew}s off Control's: synchronize it (timedatectl set-ntp true)"
  else
    pf_ok "clock within ${skew}s of Control's"
  fi
}

preflight() {
  if ((SKIP_PREFLIGHT)); then
    note "preflight checks were skipped (--skip-preflight): the install may fail later or forward nothing"
    return 0
  fi
  info "Preflight checks"
  check_systemd
  check_kernel
  if forward_node; then
    install_nftables
    check_nftables
    check_polkit
    check_firewalls
    check_ipv6_ra
    FORWARD_CHECKED=1
  fi
  check_ports
  check_disk
  check_control
  check_clock
  if ((PREFLIGHT_FAILS > 0)); then
    die "preflight found ${PREFLIGHT_FAILS} problem(s), see above; nothing on this host was changed. Fix them and re-run the command, or add --skip-preflight to install anyway"
  fi
}

# fetch downloads a URL over https, with the response headers in $3 when
# given; curl's URL globbing is off for IPv6 literals. Links from mainland
# China often cut HTTP/2 streams (curl 16, 18, 56, 92), which --retry does
# not retry: a transfer that fails other than with an HTTP error answer is
# tried twice more over HTTP/1.1.
fetch() {
  local args=(-fsSL -g --proto '=https' --retry 3 --connect-timeout 15 -o "$2") rc=0
  [[ -z "${3:-}" ]] || args+=(-D "$3")
  curl "${args[@]}" "$1" || rc=$?
  for _ in 1 2; do
    ((rc != 0 && rc != 22)) || return "${rc}"
    note "downloading $1 failed (curl exit ${rc}); trying again over HTTP/1.1"
    rc=0
    curl "${args[@]}" --http1.1 "$1" || rc=$?
  done
  return "${rc}"
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
  fetch "${CONTROL_URL}/install/agent.env" "${TMP_DIR}/agent.env" "${TMP_DIR}/agent.env.headers" ||
    die "cannot reach ${CONTROL_URL}/install/agent.env: check the address, DNS, the firewall and Control's https certificate (curl -v ${CONTROL_URL}/install/agent.env)"
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

# offline_entry tells whether a name may be in an offline bundle.
offline_entry() {
  case "$1" in
    agent.env | install.sh | install.sh.sig | SHA256SUMS | SHA256SUMS.sig) return 0 ;;
  esac
  [[ "$1" =~ ^anix-agent-linux-[a-z0-9-]+\.zip(\.sig)?$ ]]
}

# load_offline unpacks an offline bundle (a tar.gz of flat files: agent.env,
# the Agent zips with their .sig, SHA256SUMS and SHA256SUMS.sig, install.sh
# and its .sig) into TMP_DIR and reads its agent.env. Nothing in it is
# trusted before verify_offline.
load_offline() {
  local dir="${TMP_DIR}/offline" names name
  info "Reading the offline bundle ${OFFLINE}"
  names="$(tar -tzf "${OFFLINE}" 2>/dev/null)" || die "${OFFLINE} is not a tar.gz offline bundle (make one with: anix-control agent offline-bundle)"
  while IFS= read -r name; do
    name="${name#./}"
    [[ -n "${name}" ]] || continue
    offline_entry "${name}" || die "the offline bundle holds an unexpected entry ${name}: make it with 'anix-control agent offline-bundle'"
  done <<<"${names}"
  mkdir -p "${dir}"
  tar -xzf "${OFFLINE}" -C "${dir}" --no-same-owner --no-same-permissions || die "cannot unpack ${OFFLINE}"
  if [[ -n "$(find "${dir}" -mindepth 1 ! -type f -print -quit)" ]]; then
    die "the offline bundle holds something other than plain files"
  fi
  [[ -f "${dir}/agent.env" ]] || die "the offline bundle has no agent.env (Control's /install/agent.env)"
  read_metadata "${dir}/agent.env"
  [[ -f "${dir}/${ASSET}" ]] || die "the offline bundle has no ${ASSET}: it is not for ${ARCH} (anix-control agent offline-bundle --arch ${ARCH})"
  SOURCE="offline bundle ${OFFLINE}"
}

# verify_signature checks file against a base64 raw Ed25519 signature by
# the official release key; label names the file in messages. With strict
# set, a host that cannot check the signature stops the install.
verify_signature() {
  local file="$1" signature="$2" label="${3:-${ASSET}}" strict="${4:-0}" key="${ANIX_INSTALL_PUBLIC_KEY:-${OFFICIAL_PUBLIC_KEY}}"
  if ! have openssl; then
    ((strict == 0)) || die "openssl is required to verify the offline bundle (apt-get install -y openssl)"
    note "openssl is missing: the release signature of ${label} is not checked (the SHA-256 is)"
    return 0
  fi
  if ! openssl pkeyutl -help 2>&1 | grep -q -- '-rawin'; then
    ((strict == 0)) || die "this OpenSSL cannot verify Ed25519 signatures (OpenSSL 3 needed), which the offline bundle requires; install from the network instead"
    note "this OpenSSL cannot verify Ed25519 signatures (OpenSSL 3 needed): the SHA-256 of ${label} is checked, its signature is not"
    return 0
  fi
  {
    printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'
    printf '%s' "${key}" | base64 -d
  } >"${TMP_DIR}/release-key.der" || die "the release key is not valid base64"
  tr -d ' \t\r\n' <"${signature}" | base64 -d >"${TMP_DIR}/signature.bin" 2>/dev/null || die "the signature of ${label} is not base64"
  openssl pkeyutl -verify -pubin -keyform DER -inkey "${TMP_DIR}/release-key.der" -rawin \
    -in "${file}" -sigfile "${TMP_DIR}/signature.bin" >/dev/null 2>&1 ||
    die "the release signature of ${label} does not verify: it was altered; nothing was installed"
  info "Release signature of ${label} verified"
}

unpack_agent() {
  local zip="$1"
  mkdir -p "${TMP_DIR}/agent"
  if have unzip; then
    unzip -q -o "${zip}" -d "${TMP_DIR}/agent" || die "cannot unpack ${ASSET}"
  else
    python3 -m zipfile -e "${zip}" "${TMP_DIR}/agent" || die "cannot unpack ${ASSET}"
  fi
  [[ -f "${TMP_DIR}/agent/anix-agent" && ! -L "${TMP_DIR}/agent/anix-agent" ]] || die "${ASSET} holds no anix-agent binary"
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
  unpack_agent "${zip}"
}

# verify_offline checks an offline bundle like a download, but every check
# is required: SHA256SUMS signed by the official release key lists the
# Agent zip's digest, and the zip's own signature verifies. The bundle's
# agent.env is not signed and never supplies a digest.
verify_offline() {
  local dir="${TMP_DIR}/offline" zip actual
  zip="${dir}/${ASSET}"
  [[ -f "${dir}/SHA256SUMS" && -f "${dir}/SHA256SUMS.sig" ]] ||
    die "the offline bundle has no SHA256SUMS and SHA256SUMS.sig: it cannot be verified; nothing was installed"
  verify_signature "${dir}/SHA256SUMS" "${dir}/SHA256SUMS.sig" "SHA256SUMS" 1
  EXPECTED_DIGEST="$(awk -v asset="${ASSET}" '$2 == asset || $2 == "*" asset {print tolower($1); exit}' "${dir}/SHA256SUMS")"
  [[ "${EXPECTED_DIGEST}" =~ ^[0-9a-f]{64}$ ]] || die "SHA256SUMS of the offline bundle does not list ${ASSET}; nothing was installed"
  actual="$(sha256sum "${zip}" | awk '{print $1}')"
  [[ "${actual}" == "${EXPECTED_DIGEST}" ]] || die "checksum mismatch for ${ASSET}: expected ${EXPECTED_DIGEST}, got ${actual}; nothing was installed"
  info "Checksum verified (sha256 ${actual})"
  [[ -f "${zip}.sig" ]] || die "the offline bundle has no ${ASSET}.sig; nothing was installed"
  verify_signature "${zip}" "${zip}.sig" "${ASSET}" 1
  info "Installing the AnixOps Agent ${META_VERSION} (${ARCH}) from the offline bundle"
  unpack_agent "${zip}"
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
    note "the Agent ${META_VERSION} cannot migrate a root install's directories (no migrate-paths); this host joins again with the token"
    MIGRATION_STATE="not migrated (the Agent has no migrate-paths)"
    return 0
  fi
  [[ -z "${ROOT}" ]] || args+=(--root "${ROOT}")
  info "Migrating the root-installed Agent's directories to ${STATE_DIR}"
  "${bin}" "${args[@]}" ||
    die "anix-agent migrate-paths could not copy the root install's directories (see above); nothing was started, fix the cause and re-run"
  MIGRATION_STATE="ran anix-agent migrate-paths for a root install (copied or kept: see above; the old directories are left in place)"
}

ensure_users() {
  if ! getent group "${AGENT_USER}" >/dev/null; then
    groupadd --system "${AGENT_USER}"
  fi
  if ! getent passwd "${AGENT_USER}" >/dev/null; then
    useradd --system --gid "${AGENT_USER}" --home-dir "${STATE_DIR}" --no-create-home \
      --shell /usr/sbin/nologin "${AGENT_USER}"
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
    # Forwarding keeps running: the rules are in the kernel.
    systemctl stop "${SERVICE}" >/dev/null 2>&1 || true
  fi
}

install_files() {
  install -d -m 0755 "$(path "${LIB_DIR}")" "$(path "$(dirname "${BIN_LINK}")")"
  install -m 0755 "${TMP_DIR}/agent/anix-agent" "$(path "${LIB_DIR}/anix-agent.new")"
  mv -f "$(path "${LIB_DIR}/anix-agent.new")" "$(path "${LIB_DIR}/anix-agent")"
  ln -sfn "${LIB_DIR}/anix-agent" "$(path "${BIN_LINK}")"
  printf '%s\n' "${META_VERSION}" >"$(path "${LIB_DIR}/.release-version")"

  install -d -m 0750 "$(path "${CONFIG_DIR}")"
  chown "root:${AGENT_USER}" "$(path "${CONFIG_DIR}")"
  install -d -m 0700 "$(path "${STATE_DIR}")"
  chown "${AGENT_USER}:${AGENT_USER}" "$(path "${STATE_DIR}")"
  # Before pki and stream exist: migrate-paths copies only to new places.
  migrate_root_install
  install -d -m 0700 "$(path "${PKI_DIR}")" "$(path "${STREAM_DIR}")" "$(path "${UPGRADE_DIR}")"
  chown "${AGENT_USER}:${AGENT_USER}" "$(path "${STATE_DIR}")" "$(path "${PKI_DIR}")" "$(path "${STREAM_DIR}")" "$(path "${UPGRADE_DIR}")"
}

# render_config writes NODE's configuration. A regular node that relays
# (--forward) gets the forward component on its own stream ("Forward"); a
# forward node's Agent runs it without a core.
render_config() {
  local node_id="${NODE#*-}" forward=""
  if [[ "${NODE}" == proxy-* ]] && ((FORWARD)); then
    forward='
  "Forward": {"Enable": true, "NodeKind": "proxy", "ProxyNodeID": '"${node_id}"'},'
  fi
  cat <<EOF
{
  "Log": {"Level": "info", "Output": ""},${forward}
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
        "CertDir": "${PKI_DIR}"
      },
      "AgentStream": {"StateDir": "${STREAM_DIR}"}
    }
  ]
}
EOF
}

# write_config writes the configuration for NODE, keeping an existing one
# for the same node unless --reset was given; a replaced one is kept as
# config.json.bak.<time>. It carries no token: the identity is enrolled.
write_config() {
  local config
  config="$(path "${CONFIG_FILE}")"
  if [[ -f "${config}" ]] && ((RESET == 0)) && grep -q "\"AgentNode\": \"${NODE}\"" "${config}"; then
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

# existing_node prints the node of this host's configuration, or nothing.
existing_node() {
  local config
  config="$(path "${CONFIG_FILE}")"
  [[ -f "${config}" ]] || return 0
  grep -o '"AgentNode": "[a-z]*-[0-9]*"' "${config}" | head -n 1 | cut -d '"' -f 4 || true
}

# load_existing_node sets NODE from the configuration of an earlier install,
# unless --reset discards it: a re-run knows its node before the join.
load_existing_node() {
  local node
  ((RESET == 0)) || return 0
  node="$(existing_node)"
  if [[ "${node}" =~ ^(proxy|forward)-[1-9][0-9]{0,9}$ ]]; then
    NODE="${node}"
  fi
}

# detect_addresses asks Control for this host's address over IPv4 and IPv6
# (GET /install/ip, as Control's https listener sees it, trusted proxies
# applied): the join hands them to Control, which names the node's host
# after the first and rejoins a reinstalled machine by them. A family that
# does not answer is left out.
detect_addresses() {
  local family address
  PUBLIC_ADDRESSES=()
  for family in 4 6; do
    address="$(curl -fsS -g "-${family}" --proto '=https' --connect-timeout 5 -m 10 "${CONTROL_URL}/install/ip" 2>/dev/null | tr -d '[:space:]')" || address=""
    if [[ "${address}" =~ ^[0-9A-Fa-f:.]{2,45}$ ]]; then
      PUBLIC_ADDRESSES+=("${address}")
    fi
  done
}

# as_agent runs a command as the Agent's user, which owns the identity
# store.
as_agent() {
  if have runuser; then
    runuser -u "${AGENT_USER}" -- "$@"
  else
    setpriv --reuid="${AGENT_USER}" --regid="${AGENT_USER}" --init-groups -- "$@"
  fi
}

# json_string prints a string field of a one-line JSON object.
json_string() {
  local pattern="\"$2\"[[:space:]]*:[[:space:]]*\"([^\"]*)\""
  [[ "$1" =~ ${pattern} ]] && printf '%s' "${BASH_REMATCH[1]}"
}

# join_control exchanges the token for this host's identity: 'anix-agent
# join', run as the Agent's user, writes the key and the certificate into
# the identity store and prints the node it joined as one JSON object. A
# host that already has a valid identity skips it and keeps its node.
join_control() {
  local bin token_file output status=0 args=() address join_command=() created
  rm -f "$(path "${LEGACY_CREDENTIAL_FILE}")"
  if ((RESET)); then
    find "$(path "${PKI_DIR}")" -mindepth 1 -delete
    info "Discarded this host's Agent identity (--reset)"
  elif [[ -n "${NODE}" ]] && enrolled; then
    JOIN_STATE="already joined as ${NODE}; the token was not used (add --reset to join again)"
    [[ -n "${TOKEN}" ]] || JOIN_STATE="already joined as ${NODE}"
    return 0
  fi
  [[ -n "${TOKEN}" ]] || die "this host's Agent has no valid identity: a token (-t) is required; copy the command from Control"
  detect_addresses
  bin="$(path "${LIB_DIR}/anix-agent")"
  token_file="$(path "${TOKEN_FILE}")"
  (umask 077 && printf '%s\n' "${TOKEN}" >"${token_file}")
  chmod 0600 "${token_file}"
  chown "${AGENT_USER}:${AGENT_USER}" "${token_file}"
  args=(join --control "${CONTROL_URL}" --grpc "${META_GRPC}" --token-file "${TOKEN_FILE}" --pki-dir "${PKI_DIR}" --json)
  [[ -z "${NODE_NAME}" ]] || args+=(--name "${NODE_NAME}")
  [[ -z "${CONNECT_HOST}" ]] || args+=(--host "${CONNECT_HOST}")
  for address in "${PUBLIC_ADDRESSES[@]}"; do
    args+=(--address "${address}")
  done
  ((RESET == 0)) || args+=(--reset)
  join_command=("${bin}")
  if have timeout; then
    join_command=(timeout "${TIMEOUT}" "${bin}")
  fi
  if [[ "${TOKEN}" == anixjoin_* ]]; then
    info "Joining the node group at ${CONTROL_URL} (gRPC ${META_GRPC})"
  else
    info "Enrolling the node of the token at ${CONTROL_URL} (gRPC ${META_GRPC})"
  fi
  output="$(as_agent "${join_command[@]}" "${args[@]}" 2>"${TMP_DIR}/join.err")" || status=$?
  rm -f "${token_file}"
  if ((status != 0)); then
    sed 's/^/[anixops]   /' "${TMP_DIR}/join.err" >&2 || true
    if ((status == 124)); then
      printf '[anixops] error: the join did not finish within %ss.\n' "${TIMEOUT}" >&2
    else
      printf '[anixops] error: the join failed (anix-agent join exit %s).\n' "${status}" >&2
    fi
    printf '[anixops]   join_rejected or agent_enrollment_rejected: the token was reset, expired, used or its group is full; copy the current command from Control.\n' >&2
    printf '[anixops]   Otherwise check that this host reaches the gRPC target %s with TLS. The install is kept: re-run the command.\n' "${META_GRPC}" >&2
    exit 1
  fi
  output="$(printf '%s' "${output}" | tr -d '\n')"
  NODE="$(json_string "${output}" node)" || NODE=""
  [[ "${NODE}" =~ ^(proxy|forward)-[1-9][0-9]{0,9}$ ]] || die "anix-agent join answered no node: ${output:0:200}"
  NODE_NAME="$(json_string "${output}" node_name)" || NODE_NAME=""
  JOIN_GROUP="$(json_string "${output}" group)" || JOIN_GROUP=""
  JOIN_APPROVAL="$(json_string "${output}" approval)" || JOIN_APPROVAL=""
  JOIN_SPIFFE="$(json_string "${output}" spiffe_id)" || JOIN_SPIFFE=""
  JOIN_NOT_AFTER="$(json_string "${output}" not_after)" || JOIN_NOT_AFTER=""
  created="created"
  [[ ! "${output}" =~ \"created\"[[:space:]]*:[[:space:]]*false ]] || created="rejoined"
  if [[ -n "${JOIN_GROUP}" ]]; then
    JOIN_STATE="${created} ${NODE} in the node group ${JOIN_GROUP}"
  else
    JOIN_STATE="enrolled as ${NODE}"
  fi
  info "已接入：${NODE_NAME:-${NODE}} (${NODE})"
}

# post_join_preflight runs the forward checks for a host that the join made
# a forward node: before the join its kind was not known. A failure stops
# the script before the Agent starts; the identity is kept, so the same
# command continues once the cause is fixed.
post_join_preflight() {
  if ((SKIP_PREFLIGHT || FORWARD_CHECKED)) || ! forward_node; then
    return 0
  fi
  info "Preflight checks for a forward node (${NODE})"
  PREFLIGHT_FAILS=0
  check_kernel
  install_nftables
  check_nftables
  check_polkit
  check_firewalls
  check_ipv6_ra
  FORWARD_CHECKED=1
  if ((PREFLIGHT_FAILS > 0)); then
    die "this host joined Control as ${NODE}, a forward node, but preflight found ${PREFLIGHT_FAILS} problem(s), see above; the Agent was not started. Fix them and re-run the same command (the identity is kept), or add --skip-preflight"
  fi
}

# cleanup_legacy removes the legacy forward runtime of this host before the
# new Agent starts (forward-sdk.md, section 10): the named nftables tables,
# the clean agent's unit and files, and the gost of an earlier install (its
# unit, binary, directory and account). Nothing else is touched.
cleanup_legacy() {
  local table unit legacy_path
  if have nft; then
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
  if getent passwd "${LEGACY_USER}" >/dev/null; then
    if userdel "${LEGACY_USER}" >/dev/null 2>&1; then
      REMOVED+=("user ${LEGACY_USER}")
    else
      note "cannot delete the user ${LEGACY_USER}: userdel ${LEGACY_USER}"
    fi
  fi
  if getent group "${LEGACY_USER}" >/dev/null; then
    groupdel "${LEGACY_USER}" >/dev/null 2>&1 || note "cannot delete the group ${LEGACY_USER}: groupdel ${LEGACY_USER}"
  fi
}

render_sysctl() {
  local setting
  printf '# Written by the AnixOps installer (agent-install.sh): IP forwarding for\n'
  printf '# the forward drivers of the AnixOps Agent. Re-running the installer rewrites it.\n'
  for setting in "${SYSCTL_SETTINGS[@]}"; do
    printf '%s\n' "${setting/=/ = }"
  done
}

# append_accept_ra adds accept_ra = 2 for the SLAAC interfaces to the new
# drop-in $2: with forwarding on, the kernel ignores router advertisements
# on an interface whose accept_ra is 1. It does so unless --no-accept-ra
# is given, and keeps the lines an earlier run wrote to $1.
append_accept_ra() {
  local old="$1" new="$2" iface lines=()
  if [[ -f "${old}" ]]; then
    mapfile -t lines < <(grep -E '^net\.ipv6\.conf\.[^ =]+\.accept_ra *= *2$' "${old}" || true)
  fi
  if ((ACCEPT_RA)); then
    while IFS= read -r iface; do
      [[ "${iface}" =~ ^[A-Za-z0-9_.@-]{1,15}$ ]] || continue
      # sysctl writes a dot in an interface name as a slash.
      lines+=("net.ipv6.conf.${iface//./\/}.accept_ra = 2")
    done < <(slaac_interfaces)
  fi
  ((${#lines[@]} > 0)) || return 0
  {
    printf '# SLAAC interfaces keep accepting router advertisements with forwarding on.\n'
    printf '%s\n' "${lines[@]}" | sed -E 's/ *= */ = /' | sort -u
  } >>"${new}"
  FORWARDING_NOTE="; accept_ra = 2 on $(printf '%s\n' "${lines[@]}" | sed -E 's/^net\.ipv6\.conf\.([^ =]+)\.accept_ra.*/\1/' | sort -u | tr '\n' ' ' | sed 's/ $//')"
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
  append_accept_ra "${file}" "${file}.new"
  chmod 0644 "${file}.new"
  mv -f "${file}.new" "${file}"
  FORWARDING_STATE="wrote ${SYSCTL_FILE} (${SYSCTL_SETTINGS[*]}${FORWARDING_NOTE})"
  if ! have sysctl; then
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
  # systemd-journal lets 诊断 read the Agent's own log (journalctl -u); a
  # host without the group gets none, since a unit naming a missing group
  # does not start.
  local groups=""
  if getent group systemd-journal >/dev/null; then
    groups="SupplementaryGroups=systemd-journal"
  fi
  cat <<EOF
# ${SERVICE}: the AnixOps Agent. Written by the AnixOps installer
# (agent-install.sh); re-running the installer rewrites it.
[Unit]
Description=AnixOps Agent
Documentation=https://github.com/AnixOps/anix-control/blob/main/docs/guide/agent-onboarding.md
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${LIB_DIR}/anix-agent server -c ${CONFIG_FILE}
Restart=on-failure
RestartSec=5s
User=${AGENT_USER}
Group=${AGENT_USER}
# systemd-journal reads the journal for 诊断's log tail.
${groups}
UMask=0077
LimitNOFILE=1048576
# /run/anixops-agent: the plugins' sockets (ProtectSystem=strict).
RuntimeDirectory=anixops-agent
RuntimeDirectoryMode=0750

# Privileges (H13): nftables and tc over netlink, low ports, and raw
# sockets for ANX-Line's fake-TCP carrier (c3, anx-node and anx-server);
# nothing else.
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
NoNewPrivileges=yes

# Sandbox (forward-sdk.md, section 14).
ProtectSystem=strict
ReadWritePaths=${STATE_DIR}
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

# render_updater_path starts the updater when the Agent writes a request.
render_updater_path() {
  cat <<EOF
# ${UPDATER_PATH}: starts ${UPDATER_SERVICE} when the AnixOps Agent asks
# for an upgrade or a rollback that Control pushed (agent.upgrade, upgrade.v1).
# Written by the AnixOps installer; re-running the installer rewrites it.
[Unit]
Description=AnixOps Agent updater trigger
Documentation=https://github.com/AnixOps/anix-control/blob/main/docs/architecture/forward-sdk.md

[Path]
PathExists=${UPDATER_REQUEST}
Unit=${UPDATER_SERVICE}

[Install]
WantedBy=multi-user.target
EOF
}

# render_updater_service is the root oneshot that applies the request with
# the installed anix-agent: it re-verifies the staged release (size,
# SHA-256, the Ed25519 signature by the key compiled into the installed
# binary), refuses a downgrade other than a rollback to the kept release,
# keeps the running binary as anix-agent.prev, swaps the new one in,
# restarts the Agent, and reinstates the previous one if the new Agent does
# not start. It consumes the request, so the path unit does not fire again.
render_updater_service() {
  cat <<EOF
# ${UPDATER_SERVICE}: applies a Control-pushed Agent upgrade or rollback.
# Written by the AnixOps installer; re-running the installer rewrites it.
[Unit]
Description=AnixOps Agent updater
Documentation=https://github.com/AnixOps/anix-control/blob/main/docs/architecture/forward-sdk.md
ConditionPathExists=${UPDATER_REQUEST}

[Service]
Type=oneshot
ExecStart=${LIB_DIR}/anix-agent upgrade apply --request ${UPDATER_REQUEST}
TimeoutStartSec=300
UMask=0022
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=${LIB_DIR} ${STATE_DIR}
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectClock=yes
ProtectHostname=yes
RestrictAddressFamilies=AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
LockPersonality=yes
SystemCallArchitectures=native
EOF
}

# render_polkit_rule lets the Agent user restart its own unit (诊断's
# service restart), and nothing else.
render_polkit_rule() {
  cat <<EOF
// Written by the AnixOps installer: ${AGENT_USER} may restart ${SERVICE},
// only.
polkit.addRule(function(action, subject) {
  if (action.id != "org.freedesktop.systemd1.manage-units" ||
      subject.user != "${AGENT_USER}") {
    return;
  }
  var unit = action.lookup("unit");
  var verb = action.lookup("verb");
  if (unit == "${SERVICE}" && (verb == "restart" || verb == "try-restart")) {
    return polkit.Result.YES;
  }
});
EOF
}

install_units() {
  install -d -m 0755 "$(path "${UNIT_DIR}")"
  render_agent_unit >"$(path "${UNIT_DIR}/${SERVICE}")"
  render_updater_service >"$(path "${UNIT_DIR}/${UPDATER_SERVICE}")"
  render_updater_path >"$(path "${UNIT_DIR}/${UPDATER_PATH}")"
  chmod 0644 "$(path "${UNIT_DIR}/${SERVICE}")" \
    "$(path "${UNIT_DIR}/${UPDATER_SERVICE}")" "$(path "${UNIT_DIR}/${UPDATER_PATH}")"
  if [[ -d "$(path "$(dirname "${POLKIT_RULE}")")" ]]; then
    render_polkit_rule >"$(path "${POLKIT_RULE}")"
    chmod 0644 "$(path "${POLKIT_RULE}")"
  else
    note "polkit is not installed: 诊断 cannot restart the Agent until it is (apt-get install -y polkitd)"
  fi
  systemctl daemon-reload
  # The updater acts only on a request an Agent with upgrade.v1 writes.
  systemctl enable --now "${UPDATER_PATH}" >/dev/null 2>&1 || note "cannot enable ${UPDATER_PATH}: Control cannot upgrade this Agent until it is"
  systemctl enable "${SERVICE}" >/dev/null
}

# start_agent starts the Agent once its identity and configuration exist.
start_agent() {
  systemctl restart "${SERVICE}"
}

print_summary() {
  local status item spiffe not_after
  status="$(identity_status)"
  spiffe="$(identity_field "${status}" spiffe_id)"
  not_after="$(identity_field "${status}" not_after)"
  info "AnixOps Agent ${META_VERSION} is running (${INSTALL_MODE})"
  if [[ -n "${OFFLINE}" ]]; then
    printf '  source:      %s\n' "${SOURCE}"
  fi
  if [[ -n "${NODE_NAME}" ]]; then
    printf '  node:        %s (%s)\n' "${NODE}" "${NODE_NAME}"
  else
    printf '  node:        %s\n' "${NODE}"
  fi
  printf '  identity:    %s (serial %s, expires %s)\n' "${spiffe:-${JOIN_SPIFFE}}" \
    "$(identity_field "${status}" serial)" "${not_after:-${JOIN_NOT_AFTER}}"
  printf '  control:     %s (gRPC %s)\n' "${CONTROL_URL}" "${META_GRPC}"
  printf '  join:        %s\n' "${JOIN_STATE}"
  printf '  config:      %s\n' "${CONFIG_STATE}"
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
  if [[ "${JOIN_APPROVAL}" == "pending" ]]; then
    info "等待审批：节点组 ${JOIN_GROUP} 要求管理员批准新节点。管理员批准前，Control 不会向 ${NODE} 下发配置和用户。"
  fi
  if ((TOKEN_ON_COMMAND_LINE)); then
    info "The install command, token included, is in this shell's history. Clear it if others use this account (history -c, or delete the line from ~/.bash_history); reset the token in Control if it leaked."
  fi
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
  have flock || return 0
  mkdir -p "$(path "$(dirname "${LOCK_FILE}")")"
  exec 9>"$(path "${LOCK_FILE}")"
  flock -n 9 || die "another installer run is in progress"
}

# require_token stops a first install without a token before anything is
# changed.
require_token() {
  if [[ -z "${TOKEN}" && ! -f "$(path "${CONFIG_FILE}")" ]]; then
    die "this host's Agent has not joined Control: a token (-t) is required (copy the command from Control)"
  fi
}

# Uninstall (forward-sdk.md, section 9, O3). The Agent's own 'uninstall'
# command predates this layout, so the script does the work itself.

# limit_interfaces prints the interfaces the drivers may have shaped: the
# configured LimitInterfaces, and every interface of the host, since the
# installer's configuration names none and the qdisc's handle is the mark.
limit_interfaces() {
  local config dev
  config="$(path "${CONFIG_FILE}")"
  {
    if [[ -f "${config}" ]]; then
      tr -d '\n' <"${config}" | grep -o '"LimitInterfaces"[[:space:]]*:[[:space:]]*\[[^]]*\]' |
        sed 's/^"LimitInterfaces"//' | grep -o '"[^"]*"' | tr -d '"' || true
    fi
    for dev in "$(path /sys/class/net)"/*; do
      [[ -e "${dev}" ]] && basename "${dev}"
    done
  } | grep -E '^[A-Za-z0-9_.@:-]{1,15}$' | sort -u
}

# purge_forwarding removes what the forward drivers created: the nftables
# table inet anixops_fwd when it carries the driver's ownership comment, and
# root HTB qdiscs with the driver's handle. Anything without the mark is
# foreign and left alone.
purge_forwarding() {
  local listing dev qdiscs
  if have nft; then
    # shellcheck disable=SC2086 # "family name" is two words on purpose.
    listing="$(nft list table ${FWD_TABLE} 2>/dev/null)" || listing=""
    if [[ -n "${listing}" ]]; then
      if grep -qF "comment \"${FWD_TABLE_COMMENT}\"" <<<"${listing}"; then
        # shellcheck disable=SC2086
        if nft delete table ${FWD_TABLE}; then
          REMOVED+=("nftables table ${FWD_TABLE}")
        else
          note "cannot delete the nftables table ${FWD_TABLE}: delete it with 'nft delete table ${FWD_TABLE}'"
        fi
      else
        KEPT+=("nftables table ${FWD_TABLE}: it has no '${FWD_TABLE_COMMENT}' comment, so it is not the Agent's")
      fi
    fi
  fi
  if have tc; then
    while IFS= read -r dev; do
      qdiscs="$(tc qdisc show dev "${dev}" 2>/dev/null)" || qdiscs=""
      grep -qE "^qdisc htb ${FWD_TC_HANDLE} root" <<<"${qdiscs}" || continue
      if tc qdisc del dev "${dev}" root; then
        REMOVED+=("tc root qdisc ${FWD_TC_HANDLE} on ${dev}")
      else
        note "cannot delete the tc qdisc ${FWD_TC_HANDLE} on ${dev}: tc qdisc del dev ${dev} root"
      fi
    done < <(limit_interfaces)
  fi
}

# remove_path removes a system path and records it.
remove_path() {
  if [[ -e "$(path "$1")" || -L "$(path "$1")" ]]; then
    rm -rf "$(path "$1")"
    REMOVED+=("$1")
  fi
}

remove_users() {
  if getent passwd "${AGENT_USER}" >/dev/null; then
    if userdel "${AGENT_USER}"; then
      REMOVED+=("user ${AGENT_USER}")
    else
      note "cannot delete the user ${AGENT_USER}: userdel ${AGENT_USER}"
    fi
  fi
  if getent group "${AGENT_USER}" >/dev/null; then
    groupdel "${AGENT_USER}" || note "cannot delete the group ${AGENT_USER}: groupdel ${AGENT_USER}"
  fi
}

uninstall() {
  local unit item link
  require_root
  take_lock
  info "Uninstalling the AnixOps Agent$( ((PURGE)) && printf ' (--purge)')"
  # The updater first, so that nothing replaces the Agent meanwhile, then
  # the Agent, so that it cannot apply its state again.
  for unit in "${UPDATER_PATH}" "${UPDATER_SERVICE}" "${SERVICE}"; do
    if [[ -f "$(path "${UNIT_DIR}/${unit}")" ]]; then
      if have systemctl; then
        systemctl disable --now "${unit}" >/dev/null 2>&1 || true
      fi
      rm -f "$(path "${UNIT_DIR}/${unit}")"
      REMOVED+=("systemd unit ${unit}")
    fi
  done
  remove_path "${POLKIT_RULE}"
  if have systemctl; then
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl reset-failed "${SERVICE}" "${UPDATER_SERVICE}" >/dev/null 2>&1 || true
  fi
  link="$(path "${BIN_LINK}")"
  if [[ -L "${link}" && "$(readlink "${link}")" == "${LIB_DIR}/anix-agent" ]]; then
    rm -f "${link}"
    REMOVED+=("${BIN_LINK}")
  fi
  remove_path "${LIB_DIR}"
  if ((PURGE)); then
    purge_forwarding
    remove_path "${STATE_DIR}"
    remove_path "${CONFIG_DIR}"
    rmdir "$(path /etc/anixops)" 2>/dev/null || true
    remove_path "${SYSCTL_FILE}"
    remove_users
  else
    KEPT+=("${CONFIG_DIR} and ${STATE_DIR}: the identity and configuration, for a later install (uninstall --purge removes them)")
    KEPT+=("the user ${AGENT_USER} and ${SYSCTL_FILE}")
    KEPT+=("the forwarding rules in the nftables table ${FWD_TABLE} and the tc qdiscs ${FWD_TC_HANDLE}: they stay in the kernel until a reboot or uninstall --purge")
  fi
  info "The AnixOps Agent is uninstalled"
  if ((${#REMOVED[@]} == 0)); then
    printf '  removed:     nothing (the Agent was not installed)\n'
  else
    printf '  removed:\n'
    for item in "${REMOVED[@]}"; do
      printf '                 - %s\n' "${item}"
    done
  fi
  if ((${#KEPT[@]} > 0)); then
    printf '  kept:\n'
    for item in "${KEPT[@]}"; do
      printf '                 - %s\n' "${item}"
    done
  fi
  if ((PURGE)) && [[ "$(cat "$(path /proc/sys/net/ipv4/ip_forward)" 2>/dev/null)" == "1" ]]; then
    printf '  forwarding:  IP forwarding stays on until the next boot (sysctl -w net.ipv4.ip_forward=0 turns it off now)\n'
  fi
  printf '  control:     the node still exists in Control: revoke its credentials or delete the node there, which revokes the Agent certificate\n'
  for item in "${NOTES[@]}"; do
    printf '  note:        %s\n' "${item}"
  done
}

main() {
  parse_args "$@"
  if [[ "${ACTION}" == "uninstall" ]]; then
    uninstall
    return 0
  fi
  require_root
  detect_platform
  require_tools
  require_token
  load_existing_node
  take_lock
  TMP_DIR="$(mktemp -d)"
  trap cleanup_tmp EXIT
  trap 'on_error "${LINENO}"' ERR
  # Reading the metadata or the bundle changes nothing; preflight needs it
  # (the gRPC target, Control's clock).
  if [[ -n "${OFFLINE}" ]]; then
    load_offline
  else
    fetch_metadata
    select_source
  fi
  preflight
  if [[ -n "${OFFLINE}" ]]; then
    verify_offline
  else
    download_agent
  fi
  ensure_users
  stop_running_agent
  install_files
  cleanup_legacy
  install_units
  join_control
  write_config
  post_join_preflight
  enable_forwarding
  start_agent
  print_summary
}

if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
  main "$@"
fi
