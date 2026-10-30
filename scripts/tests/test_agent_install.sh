#!/usr/bin/env bash
# shellcheck disable=SC2317 # the fakes are called by the sourced installer.
# Tests of the Agent installer (internal/agentinstall/install.sh) in a fake
# root: systemctl, curl (with Control's /install/ip), nft, tc, sysctl, the
# preflight probes (uname, ss, ip, iptables, pkaction, modinfo; df is real
# unless FAKE_DF_KB is set), the user tools, runuser and chown are shell
# functions, and the Agent is a fake binary whose 'join' writes an identity
# and answers the node it joined. Nothing here touches or reads the host's
# system.

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${REPO_ROOT}/internal/agentinstall/install.sh"
WORK="$(mktemp -d)"
cleanup() { [[ -n "${KEEP_WORK:-}" ]] || find "${WORK}" -depth -delete 2>/dev/null || true; }
trap cleanup EXIT

PASS=0
fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}
ok() {
  PASS=$((PASS + 1))
  printf 'ok - %s\n' "$*"
}

readonly CONTROL="https://ctl.example.com"
readonly TOKEN_A="anixagt_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
readonly TOKEN_B="anixagt_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
readonly JOIN_TOKEN="anixjoin_JJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJJ"
readonly GITHUB="https://github.com/AnixOps/anix-agent/releases/download"

# build_release makes the fake Agent release zip and its .dgst.
build_release() {
  local dir="${WORK}/release"
  mkdir -p "${dir}/content"
  cat >"${dir}/content/anix-agent" <<'EOF'
#!/usr/bin/env bash
# Fake AnixOps Agent: `join` writes the identity record `identity --json`
# prints and answers the node: FAKE_JOIN_NODE (proxy-12) for a join token,
# FAKE_ENROLL_NODE (forward-41) for an enrollment token; FAKE_JOIN=fail
# refuses, FAKE_JOIN=hang never answers. It logs its arguments and the token
# file's mode, never the token.
# `forward sysctl-dropin` and `migrate-paths` follow FAKE_DROPIN,
# FAKE_MIGRATE_HELP and FAKE_MIGRATE; migrate-paths logs its arguments.
case "${1:-}" in
  identity)
    cat "${ANIX_INSTALL_ROOT}/var/lib/anixops-agent/pki/identity.json" 2>/dev/null || printf '[]\n'
    ;;
  join)
    shift
    args="$*"
    token_file=""
    pki=""
    while (($# > 0)); do
      case "$1" in
        --token-file) token_file="$2"; shift 2 ;;
        --pki-dir) pki="$2"; shift 2 ;;
        --control | --grpc | --name | --host | --address) shift 2 ;;
        *) shift ;;
      esac
    done
    printf 'agent join %s (token file mode %s)\n' "${args}" "$(stat -c '%a' "${ANIX_INSTALL_ROOT}${token_file}")" >>"${FAKE_AGENT_LOG}"
    case "${FAKE_JOIN:-ok}" in
      fail)
        printf 'anix-agent join: rpc error: code = PermissionDenied desc = join_rejected: the join token is not usable\n' >&2
        exit 1
        ;;
      hang) sleep 5 ;;
    esac
    token="$(cat "${ANIX_INSTALL_ROOT}${token_file}")"
    case "${token}" in
      anixjoin_*) node="${FAKE_JOIN_NODE:-proxy-12}" name="${FAKE_JOIN_NAME:-hk-01}" group=',"group":"HK"' ;;
      *) node="${FAKE_ENROLL_NODE:-forward-41}" name="relay" group='' ;;
    esac
    printf '[{"node": "%s", "dir": "x", "state": "valid", "spiffe_id": "spiffe://anixops/prod/agent/%s", "serial": "abc123", "not_after": "2026-10-10T00:00:00Z", "trust_bundle_cas": 1}]\n' \
      "${node}" "${node}" >"${ANIX_INSTALL_ROOT}${pki}/identity.json"
    printf '{"node":"%s","node_name":"%s"%s,"created":%s,"approval":"%s","spiffe_id":"spiffe://anixops/prod/agent/%s","not_after":"2026-10-19T00:00:00Z"}\n' \
      "${node}" "${name}" "${group}" "${FAKE_CREATED:-true}" "${FAKE_APPROVAL:-approved}" "${node}"
    ;;
  forward)
    [[ "${FAKE_DROPIN:-1}" == "1" && "${2:-}" == "sysctl-dropin" ]] || exit 1
    printf '# fake agent drop-in\nnet.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\n'
    ;;
  migrate-paths)
    if [[ "${2:-}" == "--help" ]]; then
      exit "${FAKE_MIGRATE_HELP:-0}"
    fi
    printf 'agent %s\n' "$*" >>"${FAKE_AGENT_LOG}"
    exit "${FAKE_MIGRATE:-0}"
    ;;
esac
EOF
  chmod 0755 "${dir}/content/anix-agent"
  printf 'readme\n' >"${dir}/content/README.md"
  (cd "${dir}/content" && python3 -m zipfile -c "${dir}/anix-agent-linux-64.zip" anix-agent README.md)
  printf 'MD5= 00\nSHA2-256= %s\n' "$(sha256sum "${dir}/anix-agent-linux-64.zip" | awk '{print $1}')" >"${dir}/anix-agent-linux-64.zip.dgst"
  RELEASE_ZIP="${dir}/anix-agent-linux-64.zip"
  RELEASE_DGST="${dir}/anix-agent-linux-64.zip.dgst"
}

# new_root prepares an empty fake root with systemd, polkit and a legacy
# forward runtime (the clean agent and the gost of an earlier install).
new_root() {
  ROOT_DIR="${WORK}/root-$1"
  mkdir -p "${ROOT_DIR}/run/systemd/system" "${ROOT_DIR}/etc/systemd/system" "${ROOT_DIR}/etc/polkit-1/rules.d" \
    "${ROOT_DIR}/etc/v2board-forward-agent" "${ROOT_DIR}/usr/local/bin" "${WORK}/state-$1" \
    "${ROOT_DIR}/proc/sys/net/netfilter" "${ROOT_DIR}/proc/sys/net/ipv4" "${ROOT_DIR}/proc/sys/net/ipv6/conf/eth0" \
    "${ROOT_DIR}/sys/class/net/eth0" "${ROOT_DIR}/sys/class/net/eth1" "${ROOT_DIR}/sys/class/net/lo" \
    "${ROOT_DIR}/var/lib/anixops-gost"
  printf '262144\n' >"${ROOT_DIR}/proc/sys/net/netfilter/nf_conntrack_max"
  printf '0\n' >"${ROOT_DIR}/proc/sys/net/ipv4/ip_forward"
  printf '1\n' >"${ROOT_DIR}/proc/sys/net/ipv6/conf/eth0/accept_ra"
  printf '[Unit]\nDescription=clean agent\n' >"${ROOT_DIR}/etc/systemd/system/v2forward-agent.service"
  printf '[Unit]\nDescription=WireGuard gost relay\n' >"${ROOT_DIR}/etc/systemd/system/gost-wg-relay.service"
  printf '[Unit]\nDescription=AnixOps forward gost\n' >"${ROOT_DIR}/etc/systemd/system/anixops-gost.service"
  printf '{}\n' >"${ROOT_DIR}/var/lib/anixops-gost/gost.json"
  printf 'panel_url: x\n' >"${ROOT_DIR}/etc/v2board-forward-agent/config.yaml"
  printf '#!/bin/sh\n' >"${ROOT_DIR}/usr/local/bin/v2forward-agent"
  STATE="${WORK}/state-$1"
  printf 'inet v2b_forward\nip anixops_forward\ninet filter\n' >"${STATE}/nft-tables"
  : >"${STATE}/nft-owned"
  : >"${STATE}/ss"
  printf 'anixops-gost\n' >"${STATE}/passwd"
  printf 'systemd-journal\nanixops-gost\n' >"${STATE}/group"
  : >"${STATE}/log"
}

metadata() {
  {
    printf 'format 1\nagent_version v4.2.0\ngrpc_target ctl.example.com:50051\n'
    printf 'asset amd64 anix-agent-linux-64.zip\nasset arm64 anix-agent-linux-arm64-v8a.zip\n'
    printf 'source github %s\n' "${GITHUB}"
    printf '%s\n' "$@"
  } >"${STATE}/agent.env"
}

# run_installer runs the script's main in a subshell with the fakes and
# records its output and status.
run_installer() {
  set +e
  (
    set -Eeuo pipefail
    export ANIX_INSTALL_ROOT="${ROOT_DIR}"
    export FAKE_AGENT_LOG="${STATE}/log"
    # shellcheck source=/dev/null
    source "${SCRIPT}"
    id() { if [[ "${1:-}" == "-u" ]]; then printf '0\n'; else command id "$@"; fi; }
    uname() {
      case "${1:-}" in
        -s) printf 'Linux\n' ;;
        -m) printf '%s\n' "${FAKE_ARCH:-x86_64}" ;;
        -r) printf '%s\n' "${FAKE_KERNEL:-6.1.0-18-amd64}" ;;
        *) command uname "$@" ;;
      esac
    }
    # FAKE_MISSING lists commands the host does not have.
    have() {
      [[ " ${FAKE_MISSING:-} " != *" $1 "* ]] || return 1
      command -v "$1" >/dev/null 2>&1
    }
    pkaction() { printf 'pkaction version %s\n' "${FAKE_POLKIT:-122}"; }
    # The package manager installs nftables unless FAKE_PKG_FAIL: nft is
    # then no longer missing.
    apt-get() {
      printf 'apt-get %s\n' "$*" >>"${FAKE_AGENT_LOG}"
      [[ "${FAKE_PKG_FAIL:-0}" == "0" ]] || return 100
      [[ "$*" != *"install -y nftables"* ]] || FAKE_MISSING="${FAKE_MISSING//nft/}"
    }
    modinfo() { [[ "${FAKE_MODINFO:-0}" == "1" ]]; }
    ss() { cat "${STATE}/ss"; }
    ip() {
      local iface
      [[ "$*" == "-6 route show default proto ra" ]] || return 1
      for iface in ${FAKE_RA:-}; do
        printf 'default via fe80::1 dev %s proto ra metric 100 expires 1790sec pref medium\n' "${iface}"
      done
    }
    iptables() { [[ "$*" == "-S FORWARD" ]] && printf -- '-P FORWARD %s\n' "${FAKE_FORWARD_POLICY:-ACCEPT}"; }
    df() {
      if [[ -n "${FAKE_DF_KB:-}" ]]; then
        printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/fake 100000000 1 %s 1%% /\n' "${FAKE_DF_KB}"
      else
        command df "$@"
      fi
    }
    tc() {
      printf 'tc %s\n' "$*" >>"${STATE}/log"
      case "$1 $2" in
        "qdisc show") cat "${STATE}/tc-$4" 2>/dev/null || true ;;
        "qdisc del") rm -f "${STATE}/tc-$4" ;;
      esac
    }
    userdel() { printf 'userdel %s\n' "$*" >>"${STATE}/log"; grep -vx "$1" "${STATE}/passwd" >"${STATE}/passwd.new" || true; mv "${STATE}/passwd.new" "${STATE}/passwd"; }
    groupdel() { printf 'groupdel %s\n' "$*" >>"${STATE}/log"; grep -vx "$1" "${STATE}/group" >"${STATE}/group.new" || true; mv "${STATE}/group.new" "${STATE}/group"; }
    sleep() { :; }
    chown() { printf 'chown %s\n' "$*" >>"${STATE}/log"; }
    # The join runs as the Agent's user: the fake runs it as it is.
    runuser() {
      printf 'runuser %s\n' "$*" >>"${STATE}/log"
      while (($# > 0)) && [[ "$1" != "--" ]]; do shift; done
      shift
      "$@"
    }
    getent() {
      case "$1" in
        passwd) grep -qx "$2" "${STATE}/passwd" ;;
        group) grep -qx "$2" "${STATE}/group" ;;
      esac
    }
    groupadd() { printf 'groupadd %s\n' "$*" >>"${STATE}/log"; printf '%s\n' "${*: -1}" >>"${STATE}/group"; }
    useradd() { printf 'useradd %s\n' "$*" >>"${STATE}/log"; printf '%s\n' "${*: -1}" >>"${STATE}/passwd"; }
    usermod() { printf 'usermod %s\n' "$*" >>"${STATE}/log"; }
    nft() {
      printf 'nft %s\n' "$*" >>"${STATE}/log"
      if [[ "$1" == "--version" ]]; then
        printf 'nftables v%s (Lester Gooch #5)\n' "${FAKE_NFT:-1.0.6}"
        return 0
      fi
      local verb="$1" family="$3" name="$4"
      grep -qx "${family} ${name}" "${STATE}/nft-tables" || return 1
      if [[ "${verb}" == "list" ]]; then
        printf 'table %s %s {\n' "${family}" "${name}"
        if grep -qx "${family} ${name}" "${STATE}/nft-owned"; then
          printf '\tcomment "anixops-forward-driver v1"\n'
        fi
        printf '}\n'
      fi
      if [[ "${verb}" == "delete" ]]; then
        grep -vx "${family} ${name}" "${STATE}/nft-tables" >"${STATE}/nft-tables.new" || true
        mv "${STATE}/nft-tables.new" "${STATE}/nft-tables"
      fi
    }
    sysctl() {
      printf 'sysctl %s\n' "$*" >>"${STATE}/log"
      [[ "${FAKE_SYSCTL:-ok}" == "ok" ]]
    }
    systemctl() {
      case "$1" in
        --version)
          printf 'systemd %s (%s.1-1)\n+PAM +AUDIT\n' "${FAKE_SYSTEMD:-252}" "${FAKE_SYSTEMD:-252}"
          return 0
          ;;
        is-active) [[ " ${FAKE_ACTIVE:-} " == *" ${*: -1} "* ]]; return ;;
      esac
      printf 'systemctl %s\n' "$*" >>"${STATE}/log"
      return 0
    }
    # The fake Control answers with a Date FAKE_SKEW seconds off this
    # clock; the gRPC target answers curl with FAKE_GRPC_RC; /install/ip
    # answers the address of the family, unless FAKE_IP is down; with
    # FAKE_H2_CUT the zip download breaks unless asked over HTTP/1.1.
    curl() {
      local out="" url="" headers="" family="" http11=0
      while (($# > 0)); do
        case "$1" in
          -V)
            if [[ "${FAKE_HTTP2:-1}" == "1" ]]; then printf 'Features: alt-svc HTTP2 HTTPS-proxy IPv6\n'; else printf 'Features: IPv6\n'; fi
            return 0
            ;;
          -o) out="$2"; shift 2 ;;
          -D) headers="$2"; shift 2 ;;
          -4 | -6) family="${1#-}"; shift ;;
          --http1.1) http11=1; shift ;;
          --proto | --retry | --connect-timeout | -m) shift 2 ;;
          -*) shift ;;
          *) url="$1"; shift ;;
        esac
      done
      printf 'curl %s\n' "${url}" >>"${STATE}/log"
      if [[ "${FAKE_H2_CUT:-0}" == "1" && "${http11}" == "0" && "${url}" == *.zip ]]; then
        return 18
      fi
      if [[ "${url}" == "${CONTROL}/install/ip" ]]; then
        [[ "${FAKE_IP:-up}" == "up" ]] || return 7
        if [[ "${family}" == "4" ]]; then printf '203.0.113.10\n'; else printf '2001:db8::10\n'; fi
        return 0
      fi
      if [[ "${url}" == "https://ctl.example.com:50051/" ]]; then
        return "${FAKE_GRPC_RC:-0}"
      fi
      if [[ "${url}" == "${CONTROL}/install/agent.env" ]]; then
        [[ "${FAKE_CONTROL_DOWN:-0}" == "0" ]] || return 7
        if [[ -n "${headers}" ]]; then
          printf 'HTTP/2 200\r\n' >"${headers}"
          [[ "${FAKE_NO_DATE:-0}" == "1" ]] ||
            printf 'date: %s\r\n' "$(date -u -d "@$(($(date +%s) + ${FAKE_SKEW:-0}))" '+%a, %d %b %Y %H:%M:%S GMT')" >>"${headers}"
        fi
      fi
      local source=""
      case "${url}" in
        "${CONTROL}/install/agent.env") source="${STATE}/agent.env" ;;
        */v4.2.0/anix-agent-linux-64.zip) source="${FAKE_ZIP:-${RELEASE_ZIP}}" ;;
        "${GITHUB}/v4.2.0/anix-agent-linux-64.zip.dgst") source="${RELEASE_DGST}" ;;
        */v4.2.0/anix-agent-linux-64.zip.sig) source="${FAKE_SIG:-}" ;;
      esac
      [[ -n "${source}" && -f "${source}" ]] || return 22
      cp "${source}" "${out}"
    }
    main "$@"
  ) >"${STATE}/out" 2>&1
  STATUS=$?
  set -e
}

expect_status() {
  [[ "${STATUS}" == "$1" ]] || { cat "${STATE}/out" >&2; fail "$2: exit ${STATUS}, want $1"; }
}

expect_out() {
  grep -qF -- "$1" "${STATE}/out" || { cat "${STATE}/out" >&2; fail "output lacks: $1"; }
}

build_release

# 1. Fresh install with a node's enrollment token: user, binary, sandboxed
# unit, the join run as the Agent's user before the Agent starts, a config
# for the node the join named without a token or an API key, the token file
# 0600 and gone, the legacy runtime removed.
new_root fresh
metadata
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "fresh install"
[[ -x "${ROOT_DIR}/usr/lib/anixops-agent/anix-agent" ]] || fail "binary not installed"
[[ "$(readlink "${ROOT_DIR}/usr/local/bin/anix-agent")" == "/usr/lib/anixops-agent/anix-agent" ]] || fail "anix-agent link"
config="${ROOT_DIR}/etc/anixops/agent/config.json"
python3 -m json.tool "${config}" >/dev/null || fail "config is not JSON"
grep -qF '"AgentNode": "forward-41"' "${config}" || fail "config lacks the node the join named"
grep -qF '"NodeID": 41' "${config}" || fail "config lacks the node id"
grep -qF '"GRPCHost": "ctl.example.com:50051"' "${config}" || fail "config lacks the gRPC target"
grep -qF '"CertDir": "/var/lib/anixops-agent/pki"' "${config}" || fail "config lacks the identity store"
! grep -q 'ApiKey\|EnrollCredentialFile\|anixagt_' "${config}" || fail "config must carry no API key and no token"
[[ "$(stat -c '%a' "${config}")" == "640" ]] || fail "config mode"
join_line="$(grep '^agent join ' "${STATE}/log")" || fail "the join did not run"
for word in "--control ${CONTROL}" "--grpc ctl.example.com:50051" "--token-file /var/lib/anixops-agent/join.token" \
  "--pki-dir /var/lib/anixops-agent/pki" "--json" "--address 203.0.113.10 --address 2001:db8::10" "(token file mode 600)"; do
  [[ "${join_line}" == *"${word}"* ]] || fail "join lacks ${word}: ${join_line}"
done
[[ "${join_line}" != *"--reset"* && "${join_line}" != *"--name"* && "${join_line}" != *"--host"* ]] || fail "join got flags it was not given: ${join_line}"
grep -q '^runuser -u anixops-agent -- timeout 180 .*/usr/lib/anixops-agent/anix-agent join ' "${STATE}/log" || fail "the join must run as anixops-agent, bounded by --timeout: $(grep runuser "${STATE}/log")"
grep -qF "chown anixops-agent:anixops-agent ${ROOT_DIR}/var/lib/anixops-agent/join.token" "${STATE}/log" || fail "the token file is not the Agent's"
[[ ! -e "${ROOT_DIR}/var/lib/anixops-agent/join.token" ]] || fail "token file left behind"
[[ "$(grep -n '^agent join ' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n '^systemctl restart anix-agent.service' "${STATE}/log" | cut -d: -f1)" ]] ||
  fail "the join must finish before the Agent starts"
[[ "$(stat -c '%a' "${ROOT_DIR}/var/lib/anixops-agent")" == "700" ]] || fail "state dir mode"
unit="${ROOT_DIR}/etc/systemd/system/anix-agent.service"
for line in "User=anixops-agent" "SupplementaryGroups=systemd-journal" "AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW" \
  "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW" "NoNewPrivileges=yes" "ProtectSystem=strict" \
  "ReadWritePaths=/var/lib/anixops-agent" "RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX" \
  "ExecStart=/usr/lib/anixops-agent/anix-agent server -c /etc/anixops/agent/config.json"; do
  grep -qxF "${line}" "${unit}" || fail "agent unit lacks ${line}"
done
! grep -qF 'anixops-gost' "${ROOT_DIR}/etc/polkit-1/rules.d/50-anixops-agent.rules" || fail "the polkit rule still names gost"
grep -qF 'unit == "anix-agent.service" && (verb == "restart" || verb == "try-restart")' \
  "${ROOT_DIR}/etc/polkit-1/rules.d/50-anixops-agent.rules" || fail "polkit rule lets the Agent restart itself"
updater="${ROOT_DIR}/etc/systemd/system/anixops-agent-updater.service"
for line in "Type=oneshot" "ExecStart=/usr/lib/anixops-agent/anix-agent upgrade apply --request /var/lib/anixops-agent/upgrade/request.json" \
  "ProtectSystem=strict" "ReadWritePaths=/usr/lib/anixops-agent /var/lib/anixops-agent"; do
  grep -qxF "${line}" "${updater}" || fail "updater unit lacks ${line}"
done
! grep -q '^User=' "${updater}" || fail "the updater runs as root"
grep -qxF "PathExists=/var/lib/anixops-agent/upgrade/request.json" "${ROOT_DIR}/etc/systemd/system/anixops-agent-updater.path" || fail "updater path unit"
grep -qF 'systemctl enable --now anixops-agent-updater.path' "${STATE}/log" || fail "updater path not enabled"
[[ "$(stat -c '%a' "${ROOT_DIR}/var/lib/anixops-agent/upgrade")" == "700" ]] || fail "upgrade dir mode"
grep -qF 'useradd --system --gid anixops-agent --home-dir' "${STATE}/log" || fail "agent user not created"
! grep -q 'anixops-gost' "${ROOT_DIR}/etc/systemd/system/anix-agent.service" || fail "the agent unit still names gost"
for gone in etc/systemd/system/anixops-gost.service var/lib/anixops-gost; do
  [[ ! -e "${ROOT_DIR}/${gone}" ]] || fail "the earlier install's gost was not retired: ${gone}"
done
grep -qF 'systemctl disable --now anixops-gost.service' "${STATE}/log" || fail "the earlier gost was not stopped"
! grep -qx anixops-gost "${STATE}/passwd" || fail "the gost user was not removed"
! grep -qx anixops-gost "${STATE}/group" || fail "the gost group was not removed"
grep -qF 'systemctl enable anix-agent.service' "${STATE}/log" || fail "service not enabled"
[[ "$(cat "${STATE}/nft-tables")" == "inet filter" ]] || fail "legacy tables left or foreign table removed: $(cat "${STATE}/nft-tables")"
[[ ! -e "${ROOT_DIR}/etc/systemd/system/v2forward-agent.service" && ! -e "${ROOT_DIR}/etc/v2board-forward-agent" && ! -e "${ROOT_DIR}/usr/local/bin/v2forward-agent" ]] ||
  fail "clean agent not removed"
[[ -f "${ROOT_DIR}/etc/systemd/system/gost-wg-relay.service" ]] || fail "the WireGuard gost relay must not be touched"
expect_out "nftables table inet v2b_forward"
expect_out "nftables table ip anixops_forward"
expect_out "systemd unit v2forward-agent.service"
expect_out "AnixOps Agent v4.2.0 is running (install)"
expect_out "已接入：relay (forward-41)"
expect_out "join:        enrolled as forward-41"
expect_out "node:        forward-41 (relay)"
sysctl_file="${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf"
grep -qxF 'net.ipv4.ip_forward = 1' "${sysctl_file}" || fail "sysctl file lacks IPv4 forwarding: a forward node joined"
grep -qxF 'net.ipv6.conf.all.forwarding = 1' "${sysctl_file}" || fail "sysctl file lacks IPv6 forwarding"
[[ "$(stat -c '%a' "${sysctl_file}")" == "644" ]] || fail "sysctl file mode"
grep -qxF "sysctl -e -q -p ${sysctl_file}" "${STATE}/log" || fail "forwarding not applied"
[[ "$(head -n 1 "${sysctl_file}")" == "# fake agent drop-in" ]] || fail "the Agent's drop-in was not used"
grep -qxF "RuntimeDirectory=anixops-agent" "${unit}" || fail "agent unit lacks RuntimeDirectory"
grep -qxF "RuntimeDirectoryMode=0750" "${unit}" || fail "agent unit lacks RuntimeDirectoryMode"
! grep -q '^agent migrate-paths' "${STATE}/log" || fail "a fresh install ran migrate-paths"
[[ "$(grep -n '^sysctl ' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n '^systemctl restart anix-agent.service' "${STATE}/log" | cut -d: -f1)" ]] ||
  fail "forwarding must be on before the Agent starts"
expect_out "forwarding:  wrote /etc/sysctl.d/90-anixops-forward.conf (net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1); applied"
expect_out "serial abc123"
expect_out "Preflight checks for a forward node (forward-41)"
expect_out "token included, is in this shell's history"
! grep -qF "${TOKEN_A}" "${STATE}/out" || fail "the token was printed"
! grep -qF "${TOKEN_A}" "${STATE}/log" || fail "the token reached a log"
grep -qF "curl ${GITHUB}/v4.2.0/anix-agent-linux-64.zip.dgst" "${STATE}/log" || fail "checksum not from GitHub"
grep -qF "curl ${CONTROL}/install/ip" "${STATE}/log" || fail "the addresses were not asked"
ok "fresh install with an enrollment token"

# 2. The same command again: upgrade in place, identity kept, token unused.
: >"${STATE}/log"
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "re-run"
expect_out "(upgrade)"
expect_out "already joined as forward-41; the token was not used (add --reset to join again)"
expect_out "kept /etc/anixops/agent/config.json"
expect_out "legacy:      nothing to remove"
! grep -q '^agent join' "${STATE}/log" || fail "re-run joined again"
grep -qF 'systemctl stop anix-agent.service' "${STATE}/log" || fail "re-run did not stop the old Agent first"
ok "re-run is an in-place upgrade"

# 3. Another token keeps the node unless --reset; with it the host joins
# again and the old config is kept as a backup.
: >"${STATE}/log"
run_installer -u "${CONTROL}" -t "${TOKEN_B}"
expect_status 0 "another token without --reset"
expect_out "already joined as forward-41; the token was not used"
! grep -q '^agent join' "${STATE}/log" || fail "a valid identity joined again without --reset"
FAKE_ENROLL_NODE=forward-42 run_installer -u "${CONTROL}" -t "${TOKEN_B}" --reset
expect_status 0 "reset"
grep -qF '"AgentNode": "forward-42"' "${config}" || fail "reset did not rewrite the config"
compgen -G "${config}.bak.*" >/dev/null || fail "reset kept no backup of the config"
expect_out "Discarded this host's Agent identity"
grep -q '^agent join .*--reset' "${STATE}/log" || fail "the join was not told --reset"
ok "--reset joins again"

# 4. A checksum mismatch installs nothing.
new_root mismatch
metadata
printf 'not the release\n' >"${WORK}/bad.zip"
FAKE_ZIP="${WORK}/bad.zip" run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "checksum mismatch"
expect_out "checksum mismatch"
[[ ! -e "${ROOT_DIR}/usr/lib/anixops-agent" && ! -e "${ROOT_DIR}/etc/systemd/system/anix-agent.service" ]] || fail "a bad download changed the host"
[[ -f "${ROOT_DIR}/etc/systemd/system/v2forward-agent.service" ]] || fail "a bad download removed the legacy runtime"
! grep -q '^agent join' "${STATE}/log" || fail "a bad download joined Control"
ok "checksum mismatch installs nothing"

# 5. Mirrors: Control's digest is used and the cn mirror's never fetched.
new_root mirror
metadata "source cn https://mirror.example.cn/anix-agent" "source control ${CONTROL}/install/agent" \
  "sha256 anix-agent-linux-64.zip $(sha256sum "${RELEASE_ZIP}" | awk '{print $1}')"
FAKE_ENROLL_NODE=proxy-7 run_installer -u "${CONTROL}" -t "${TOKEN_A}" --mirror cn
expect_status 0 "cn mirror"
grep -qF 'curl https://mirror.example.cn/anix-agent/v4.2.0/anix-agent-linux-64.zip' "${STATE}/log" || fail "cn mirror not used"
[[ ! -e "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ]] || fail "a proxy node got IP forwarding without --forward"
! grep -q '"Forward"' "${ROOT_DIR}/etc/anixops/agent/config.json" || fail "a proxy node got the forward component without --forward"
! grep -q '^sysctl ' "${STATE}/log" || fail "sysctl ran for a proxy node without --forward"
expect_out "forwarding:  unchanged (a proxy node; --forward turns IP forwarding on)"
! grep -q '\.dgst' "${STATE}/log" || fail "a .dgst was fetched although Control published the digest"
! grep -q 'Preflight checks for a forward node' "${STATE}/out" || fail "a proxy node got the forward checks"
new_root fallback
metadata
run_installer -u "${CONTROL}" -t "${TOKEN_A}" --mirror cn
expect_status 0 "cn fallback"
expect_out "the cn mirror is not set up on Control; downloading from GitHub releases"
ok "mirror selection and trusted digests"

# 5a. A download whose HTTP/2 stream is cut is tried again over HTTP/1.1.
new_root h2cut
metadata
FAKE_H2_CUT=1 run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "HTTP/2 stream cut"
expect_out "(curl exit 18); trying again over HTTP/1.1"
ok "a cut HTTP/2 download is retried over HTTP/1.1"

# 5b. IP forwarding: a proxy node with --forward gets it; a sysctl that
# cannot apply leaves the file for the next boot and only notes it.
new_root proxyforward
metadata
FAKE_ENROLL_NODE=proxy-7 run_installer -u "${CONTROL}" -t "${TOKEN_A}" --forward
expect_status 0 "proxy --forward"
grep -qxF 'net.ipv4.ip_forward = 1' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "--forward wrote no sysctl file"
expect_out "; applied"
grep -qF '"Forward": {"Enable": true, "NodeKind": "proxy", "ProxyNodeID": 7},' "${ROOT_DIR}/etc/anixops/agent/config.json" ||
  fail "a relay's config lacks the forward component on its own stream"
new_root sysctlfail
metadata
FAKE_DROPIN=0 FAKE_SYSCTL=fail run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "sysctl failure is not fatal"
[[ -f "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ]] || fail "sysctl failure removed the file"
grep -qF 'Written by the AnixOps installer' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ||
  fail "an Agent without sysctl-dropin did not get the installer's drop-in"
grep -qxF 'net.ipv6.conf.all.forwarding = 1' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "installer drop-in lacks IPv6"
expect_out "sysctl could not apply /etc/sysctl.d/90-anixops-forward.conf now"
expect_out "; applies at the next boot"
ok "IP forwarding for forward nodes"

# 5c. Switching from anix-agent's root install: migrate-paths copies its
# directories, owned by anixops-agent, before the join and the new unit.
root_switch() {
  new_root "$1"
  metadata
  printf '[Unit]\nDescription=anix-agent (root)\n[Service]\nExecStart=/usr/local/bin/anix-agent server\n' \
    >"${ROOT_DIR}/etc/systemd/system/anix-agent.service"
}
root_switch rootunit
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "switch from a root unit"
grep -qxF "agent migrate-paths --chown anixops-agent --root ${ROOT_DIR}" "${STATE}/log" || fail "migrate-paths not run: $(grep agent "${STATE}/log")"
[[ "$(grep -n '^agent migrate-paths' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n '^agent join' "${STATE}/log" | cut -d: -f1)" ]] ||
  fail "migrate-paths must run before the join"
grep -qxF "User=anixops-agent" "${ROOT_DIR}/etc/systemd/system/anix-agent.service" || fail "the root unit was not replaced"
expect_out "migration:   ran anix-agent migrate-paths for a root install"
new_root rootdirs
metadata
mkdir -p "${ROOT_DIR}/var/lib/anixops/plugins"
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "switch with the root install's directories"
grep -q '^agent migrate-paths --chown anixops-agent' "${STATE}/log" || fail "old directories did not trigger migrate-paths"
root_switch rootfail
FAKE_MIGRATE=1 run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "migrate-paths failure"
expect_out "anix-agent migrate-paths could not copy"
! grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the Agent started after a failed migration"
! grep -q '^agent join' "${STATE}/log" || fail "the host joined after a failed migration"
root_switch rootold
FAKE_MIGRATE_HELP=1 run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "an Agent without migrate-paths"
expect_out "cannot migrate a root install's directories"
! grep -q '^agent migrate-paths' "${STATE}/log" || fail "migrate-paths ran on an Agent without it"
ok "switching from a root install"

# 6. Signatures: a valid release signature verifies, a wrong one aborts.
if openssl pkeyutl -help 2>&1 | grep -q -- '-rawin'; then
  openssl genpkey -algorithm ED25519 -out "${WORK}/key.pem" 2>/dev/null
  public="$(openssl pkey -in "${WORK}/key.pem" -pubout -outform DER | tail -c 32 | base64 -w0)"
  openssl pkeyutl -sign -inkey "${WORK}/key.pem" -rawin -in "${RELEASE_ZIP}" -out "${WORK}/zip.sig.bin"
  base64 -w0 "${WORK}/zip.sig.bin" >"${WORK}/zip.sig"
  new_root signed
  metadata
  ANIX_INSTALL_PUBLIC_KEY="${public}" FAKE_SIG="${WORK}/zip.sig" run_installer -u "${CONTROL}" -t "${TOKEN_A}"
  expect_status 0 "signed release"
  expect_out "Release signature of anix-agent-linux-64.zip verified"
  new_root badsig
  metadata
  FAKE_SIG="${WORK}/zip.sig" run_installer -u "${CONTROL}" -t "${TOKEN_A}"
  expect_status 1 "signature by another key"
  expect_out "does not verify"
  [[ ! -e "${ROOT_DIR}/usr/lib/anixops-agent" ]] || fail "a bad signature changed the host"

  # The verification recipe in the script's header checks the script.
  openssl pkeyutl -sign -inkey "${WORK}/key.pem" -rawin -in "${SCRIPT}" -out "${WORK}/script.sig.bin"
  base64 -w0 "${WORK}/script.sig.bin" >"${WORK}/install.sh.sig"
  { printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; printf '%s' "${public}" | base64 -d; } >"${WORK}/key.der"
  base64 -d "${WORK}/install.sh.sig" >"${WORK}/install.sh.sig.bin"
  openssl pkeyutl -verify -pubin -keyform DER -inkey "${WORK}/key.der" -rawin -in "${SCRIPT}" -sigfile "${WORK}/install.sh.sig.bin" >/dev/null ||
    fail "the documented verification recipe fails"
  ok "release signatures"

  # Offline bundles: verified like a download, but every signature is
  # required; nothing is downloaded.
  make_bundle() {
    local dir="${WORK}/bundle-$1" zip="${2:-${RELEASE_ZIP}}"
    rm -rf "${dir}"
    mkdir -p "${dir}"
    cp "${STATE}/agent.env" "${dir}/agent.env"
    cp "${zip}" "${dir}/anix-agent-linux-64.zip"
    cp "${SCRIPT}" "${dir}/install.sh"
    (cd "${dir}" && sha256sum anix-agent-linux-64.zip >SHA256SUMS)
    for file in anix-agent-linux-64.zip SHA256SUMS; do
      openssl pkeyutl -sign -inkey "${WORK}/key.pem" -rawin -in "${dir}/${file}" -out "${WORK}/bundle.sig.bin"
      base64 -w0 "${WORK}/bundle.sig.bin" >"${dir}/${file}.sig"
    done
    BUNDLE_DIR="${dir}"
  }
  pack_bundle() { tar -czf "${WORK}/$1.tar.gz" -C "${BUNDLE_DIR}" .; BUNDLE="${WORK}/$1.tar.gz"; }

  new_root offline
  metadata
  make_bundle good
  pack_bundle good
  ANIX_INSTALL_PUBLIC_KEY="${public}" run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 0 "offline install"
  expect_out "Release signature of SHA256SUMS verified"
  expect_out "Release signature of anix-agent-linux-64.zip verified"
  expect_out "source:      offline bundle ${BUNDLE}"
  [[ -x "${ROOT_DIR}/usr/lib/anixops-agent/anix-agent" ]] || fail "offline install installed no Agent"
  ! grep -q '\.zip\|\.dgst' "${STATE}/log" || fail "an offline install downloaded: $(grep curl "${STATE}/log")"
  grep -qF 'curl https://ctl.example.com:50051/' "${STATE}/log" || fail "offline preflight did not check the gRPC target"
  grep -q '^agent join ' "${STATE}/log" || fail "an offline install did not join"

  offline_case() {
    local name="$1" message="$2"
    new_root "offline-${name}"
    metadata
    ANIX_INSTALL_PUBLIC_KEY="${public}" run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
    expect_status 1 "offline ${name}"
    expect_out "${message}"
    [[ ! -e "${ROOT_DIR}/usr/lib/anixops-agent" ]] || fail "offline ${name}: the host changed"
  }
  make_bundle badsums
  printf '%s  anix-agent-linux-64.zip\n' "$(printf 'x' | sha256sum | awk '{print $1}')" >"${BUNDLE_DIR}/SHA256SUMS"
  pack_bundle badsums
  offline_case badsums "the release signature of SHA256SUMS does not verify"
  make_bundle badzip
  printf 'not the release\n' >"${BUNDLE_DIR}/anix-agent-linux-64.zip"
  pack_bundle badzip
  offline_case badzip "checksum mismatch for anix-agent-linux-64.zip"
  make_bundle badsig
  cp "${BUNDLE_DIR}/SHA256SUMS.sig" "${BUNDLE_DIR}/anix-agent-linux-64.zip.sig"
  pack_bundle badsig
  offline_case badsig "the release signature of anix-agent-linux-64.zip does not verify"
  make_bundle nosig
  rm -f "${BUNDLE_DIR}/anix-agent-linux-64.zip.sig"
  pack_bundle nosig
  offline_case nosig "the offline bundle has no anix-agent-linux-64.zip.sig"
  make_bundle nosums
  rm -f "${BUNDLE_DIR}/SHA256SUMS.sig"
  pack_bundle nosums
  offline_case nosums "the offline bundle has no SHA256SUMS and SHA256SUMS.sig"
  make_bundle evil
  mkdir -p "${BUNDLE_DIR}/etc" && printf 'x\n' >"${BUNDLE_DIR}/etc/passwd"
  pack_bundle evil
  offline_case evil "the offline bundle holds an unexpected entry etc/"
  printf 'not a tarball\n' >"${WORK}/plain.tar.gz"
  BUNDLE="${WORK}/plain.tar.gz"
  offline_case plain "is not a tar.gz offline bundle"
  BUNDLE="${WORK}/good.tar.gz"
  new_root offline-arch
  metadata
  FAKE_ARCH=aarch64 ANIX_INSTALL_PUBLIC_KEY="${public}" run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 1 "offline bundle for another architecture"
  expect_out "it is not for arm64"
  new_root offline-otherkey
  metadata
  run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 1 "offline bundle signed by another key"
  expect_out "the release signature of SHA256SUMS does not verify"
  new_root offline-noopenssl
  metadata
  FAKE_MISSING=openssl run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 1 "offline without openssl"
  expect_out "missing tools: openssl"
  new_root offline-nohttps
  metadata
  FAKE_CONTROL_DOWN=1 FAKE_IP=down ANIX_INSTALL_PUBLIC_KEY="${public}" run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 0 "offline install without Control's https"
  expect_out "cannot reach https://ctl.example.com over https"
  ! grep -q '^agent join .*--address' "${STATE}/log" || fail "addresses passed although Control's https did not answer"
  new_root offline-nogrpc
  metadata
  FAKE_GRPC_RC=7 ANIX_INSTALL_PUBLIC_KEY="${public}" run_installer -u "${CONTROL}" -t "${TOKEN_A}" --offline "${BUNDLE}"
  expect_status 1 "offline install without the gRPC target"
  expect_out "cannot connect to the gRPC target"
  ok "offline bundles"
else
  printf 'skip - release signatures (OpenSSL without -rawin)\n'
fi

# 7. A join that fails or never answers stops before the Agent starts, says
# why, keeps the install and removes the token file.
new_root joinfail
metadata
FAKE_JOIN=fail run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 1 "join refused"
expect_out "join_rejected: the join token is not usable"
expect_out "the join failed (anix-agent join exit 1)"
expect_out "copy the current command from Control"
[[ -x "${ROOT_DIR}/usr/lib/anixops-agent/anix-agent" ]] || fail "a failed join removed the install"
[[ ! -e "${ROOT_DIR}/var/lib/anixops-agent/join.token" ]] || fail "a failed join left the token file"
[[ ! -e "${ROOT_DIR}/etc/anixops/agent/config.json" ]] || fail "a failed join wrote a config"
! grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the Agent started without an identity"
new_root joinhang
metadata
FAKE_JOIN=hang run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}" --timeout 1
expect_status 1 "join timeout"
expect_out "the join did not finish within 1s"
[[ ! -e "${ROOT_DIR}/var/lib/anixops-agent/join.token" ]] || fail "a timed-out join left the token file"
ok "a failed join"

# 8. Joining a node group: --name and --host reach the join; the node the
# join created gets the config; a pending node is told so; a join token
# from the environment leaves nothing in the history.
new_root group
metadata
run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}" --name hk-01 --host hk.nodes.example.com
expect_status 0 "group join"
grep -q '^agent join .*--name hk-01 --host hk.nodes.example.com --address 203.0.113.10 --address 2001:db8::10' "${STATE}/log" ||
  fail "join arguments: $(grep '^agent join' "${STATE}/log")"
grep -qF '"AgentNode": "proxy-12"' "${ROOT_DIR}/etc/anixops/agent/config.json" || fail "config lacks the joined node"
grep -qF '"NodeID": 12' "${ROOT_DIR}/etc/anixops/agent/config.json" || fail "config lacks the joined node's id"
expect_out "已接入：hk-01 (proxy-12)"
expect_out "join:        created proxy-12 in the node group HK"
expect_out "forwarding:  unchanged (a proxy node; --forward turns IP forwarding on)"
expect_out "token included, is in this shell's history"
! grep -q '等待审批' "${STATE}/out" || fail "an approved node was told to wait"
! grep -qF "${JOIN_TOKEN}" "${STATE}/out" || fail "the join token was printed"
: >"${STATE}/log"
run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 0 "group join re-run"
expect_out "already joined as proxy-12; the token was not used"
! grep -q '^agent join' "${STATE}/log" || fail "a re-run joined the group again"
new_root group-pending
metadata
FAKE_APPROVAL=pending FAKE_CREATED=false ANIX_AGENT_TOKEN="${JOIN_TOKEN}" run_installer -u "${CONTROL}"
expect_status 0 "pending join"
expect_out "join:        rejoined proxy-12 in the node group HK"
expect_out "等待审批：节点组 HK 要求管理员批准新节点"
! grep -q "shell's history" "${STATE}/out" || fail "a token from the environment is not in the history"
new_root group-noip
metadata
FAKE_IP=down run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 0 "join without /install/ip"
! grep -q '^agent join .*--address' "${STATE}/log" || fail "addresses passed although /install/ip did not answer"
ok "joining a node group"

# 8b. A forward group: the host turns out a forward node after the join, so
# the forward checks run then, and a failure stops before the Agent starts;
# the same command continues once fixed, without joining again.
new_root group-forward
metadata
FAKE_JOIN_NODE=forward-7 run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 0 "forward group join"
expect_out "Preflight checks for a forward node (forward-7)"
grep -qxF 'net.ipv4.ip_forward = 1' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "a forward group's node got no IP forwarding"
new_root group-forward-fail
metadata
FAKE_JOIN_NODE=forward-7 FAKE_NFT=0.9.6 run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 1 "forward group join on a host that cannot forward"
expect_out "preflight: FAIL  nftables 0.9.6 is older than 0.9.7"
expect_out "this host joined Control as forward-7, a forward node, but preflight found 1 problem(s)"
! grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the Agent started on a host that cannot forward"
: >"${STATE}/log"
FAKE_JOIN_NODE=forward-7 run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 0 "re-run after the fix"
! grep -q '^agent join' "${STATE}/log" || fail "the fixed host joined again"
grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the fixed host's Agent did not start"
# Without nft, the join's forward checks install nftables first and the
# Agent starts (the newnb-1488 case stopped after the join).
new_root group-forward-no-nft
metadata
FAKE_JOIN_NODE=forward-7 FAKE_MISSING=nft run_installer -u "${CONTROL}" -t "${JOIN_TOKEN}"
expect_status 0 "forward group join without nft"
expect_out "installed nftables (the nftables forward driver needs nft)"
grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the Agent did not start after nftables was installed"
ok "a forward group's node"

# 9. Arguments and platforms.
new_root args
metadata
long_name="$(printf 'n%.0s' {1..65})"
for case in \
  "2|--node is gone: the token names its node or its node group|-u ${CONTROL} --node forward-41 -t ${TOKEN_A}" \
  "2|--group is gone|-u ${CONTROL} --group edge -t ${JOIN_TOKEN}" \
  "2|--offline pkg.tar: no such file|-u ${CONTROL} -t ${TOKEN_A} --offline pkg.tar" \
  "2|unknown argument to uninstall: --bogus|uninstall --bogus" \
  "2|--port-range must be FROM-TO|-u ${CONTROL} -t ${TOKEN_A} --port-range 30000-20000" \
  "2|--port-range must be FROM-TO|-u ${CONTROL} -t ${TOKEN_A} --port-range 1-70000" \
  "2|-u (--control) is required|-t ${TOKEN_A}" \
  "2|-u (--control) must be https|--control http://ctl.example.com -t ${TOKEN_A}" \
  "2|is not an AnixOps join token (anixjoin_...) or enrollment token (anixagt_...)|-u ${CONTROL} -t secret" \
  "2|is not an AnixOps join token|-u ${CONTROL} -t anixjoin_short" \
  "2|--mirror must be control, cn or github|-u ${CONTROL} -t ${TOKEN_A} --mirror ftp" \
  "2|--reset needs a token (-t)|-u ${CONTROL} --reset" \
  "2|--name must be at most 64 characters|-u ${CONTROL} -t ${JOIN_TOKEN} --name ${long_name}" \
  "2|--host must be a host name or an IP address|-u ${CONTROL} -t ${JOIN_TOKEN} --host a/b" \
  "2|-t needs a value|-u ${CONTROL} -t" \
  "2|unknown argument|-u ${CONTROL} --bogus" \
  "1|has not joined Control: a token (-t) is required|--control=${CONTROL}"; do
  IFS='|' read -r want message args <<<"${case}"
  # shellcheck disable=SC2086 # the case's arguments are words.
  run_installer ${args}
  expect_status "${want}" "${args}"
  expect_out "${message}"
done
FAKE_ARCH=riscv64 run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "riscv64"
expect_out "unsupported architecture riscv64"
FAKE_MISSING="runuser setpriv" run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "no runuser"
expect_out "missing tools: util-linux"
rm -rf "${ROOT_DIR}/run/systemd"
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "no systemd"
expect_out "systemd is required"
mkdir -p "${ROOT_DIR}/sbin" && : >"${ROOT_DIR}/sbin/openrc-run"
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 1 "OpenRC"
expect_out "OpenRC is not supported"
ok "arguments and platforms"

# 10. Preflight: a healthy forward node passes every check. Before a fresh
# join the forward checks run up front only with --forward.
new_root pfok
metadata
run_installer -u "${CONTROL}" -t "${TOKEN_A}" --forward
expect_status 0 "healthy preflight"
for line in "preflight: ok    systemd 252" "preflight: ok    Linux 6.1.0-18-amd64" "preflight: ok    nftables 1.0.6" \
  "preflight: ok    conntrack (nf_conntrack loaded)" "preflight: ok    polkit 122" \
  "preflight: ok    no firewall manager drops forwarded traffic" "preflight: ok    IPv6 router advertisements (no SLAAC interface" \
  "preflight: skip  ports" "MiB free for /usr/lib/anixops-agent" "MiB free for download directory" \
  "preflight: ok    Control https://ctl.example.com reachable over https (certificate verified)" \
  "preflight: ok    gRPC target ctl.example.com:50051 reachable over TLS (certificate verified)" "preflight: ok    clock within"; do
  expect_out "${line}"
done
! grep -q 'preflight: FAIL\|note: preflight' "${STATE}/out" || fail "a healthy host got a preflight warning: $(grep preflight "${STATE}/out")"
! grep -q 'Preflight checks for a forward node' "${STATE}/out" || fail "the forward checks ran twice"
[[ "$(grep -n '^curl https://ctl.example.com:50051/' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n 'anix-agent-linux-64.zip$' "${STATE}/log" | head -n 1 | cut -d: -f1)" ]] ||
  fail "preflight must run before the download"
ok "preflight passes on a healthy forward node"

# pf_case NAME STATUS MESSAGE "VAR=value ..." [installer arguments]: one
# preflight check in a fresh root, a forward node declared up front. A
# failure must leave the host unchanged.
pf_case() {
  local name="$1" want="$2" message="$3" envs="$4" assignment
  shift 4
  new_root "pf-${name}"
  metadata
  for assignment in ${envs}; do export "${assignment?}"; done
  if (($# == 0)); then
    set -- -u "${CONTROL}" -t "${TOKEN_A}" --forward
  fi
  run_installer "$@"
  for assignment in ${envs}; do unset "${assignment%%=*}"; done
  expect_status "${want}" "preflight ${name}"
  expect_out "${message}"
  if [[ "${want}" != "0" ]]; then
    expect_out "nothing on this host was changed"
    [[ ! -e "${ROOT_DIR}/usr/lib/anixops-agent" && ! -e "${ROOT_DIR}/etc/systemd/system/anix-agent.service" ]] ||
      fail "preflight ${name}: a failed check changed the host"
    [[ -f "${ROOT_DIR}/etc/systemd/system/v2forward-agent.service" ]] || fail "preflight ${name}: a failed check removed the legacy runtime"
    ! grep -q 'anix-agent-linux-64.zip$' "${STATE}/log" || fail "preflight ${name}: the Agent was downloaded after a failed check"
    ! grep -q '^agent join' "${STATE}/log" || fail "preflight ${name}: the host joined after a failed check"
  fi
}

pf_case systemd-old 1 "preflight: FAIL  systemd 239 is older than 240" "FAKE_SYSTEMD=239"
pf_case systemd-245 0 "preflight: ok    systemd 245" "FAKE_SYSTEMD=245"
pf_case kernel-old 1 "preflight: FAIL  Linux 5.4.0-150-generic is older than 5.10" "FAKE_KERNEL=5.4.0-150-generic"
pf_case kernel-old-proxy 0 "Linux 5.4.0-150-generic is older than 5.10: this node cannot forward with nftables later" \
  "FAKE_KERNEL=5.4.0-150-generic FAKE_ENROLL_NODE=proxy-7" -u "${CONTROL}" -t "${TOKEN_A}"
pf_case nft-missing 1 "preflight: FAIL  nft is missing" "FAKE_MISSING=nft FAKE_PKG_FAIL=1"
expect_out "fix: apt-get install -y nftables"
pf_case nft-installed 0 "installed nftables (the nftables forward driver needs nft)" "FAKE_MISSING=nft"
grep -q '^apt-get install -y nftables' "${STATE}/log" || fail "nftables was not installed with apt-get"
pf_case nft-old 1 "preflight: FAIL  nftables 0.9.6 is older than 0.9.7" "FAKE_NFT=0.9.6"
pf_case tc-missing 0 "tc is missing: bandwidth limits are off" "FAKE_MISSING=tc"
new_root pf-conntrack
metadata
rm -f "${ROOT_DIR}/proc/sys/net/netfilter/nf_conntrack_max"
FAKE_MODINFO=1 run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "conntrack as a module"
expect_out "preflight: ok    conntrack (nf_conntrack loads on first use)"
rm -f "${ROOT_DIR}/proc/sys/net/netfilter/nf_conntrack_max"
run_installer -u "${CONTROL}"
expect_status 0 "conntrack missing"
expect_out "conntrack (nf_conntrack) is neither loaded nor installed"
pf_case polkit-old 0 "polkit 0.105 reads no rules from /etc/polkit-1/rules.d" "FAKE_POLKIT=0.105"
pf_case polkit-missing 0 "polkit is not installed" "FAKE_MISSING=pkaction"
pf_case firewalld 0 "firewalld is active" "FAKE_ACTIVE=firewalld"
pf_case docker-drop 0 "the iptables FORWARD policy is DROP" "FAKE_FORWARD_POLICY=DROP"
new_root pf-ufw
metadata
mkdir -p "${ROOT_DIR}/etc/ufw" "${ROOT_DIR}/etc/default"
printf 'ENABLED=yes\n' >"${ROOT_DIR}/etc/ufw/ufw.conf"
printf 'DEFAULT_FORWARD_POLICY="DROP"\n' >"${ROOT_DIR}/etc/default/ufw"
run_installer -u "${CONTROL}" -t "${TOKEN_A}" --forward
expect_status 0 "ufw"
expect_out 'ufw is active with DEFAULT_FORWARD_POLICY="DROP"'
pf_case disk 1 "preflight: FAIL  only 0 MiB free for /usr/lib/anixops-agent" "FAKE_DF_KB=1000"
pf_case grpc-refused 1 "preflight: FAIL  cannot connect to the gRPC target ctl.example.com:50051" "FAKE_GRPC_RC=7"
pf_case grpc-dns 1 "preflight: FAIL  cannot resolve ctl.example.com" "FAKE_GRPC_RC=6"
pf_case grpc-cert 1 "preflight: FAIL  the certificate of the gRPC target ctl.example.com:50051 does not verify" "FAKE_GRPC_RC=60"
pf_case grpc-tls 1 "preflight: FAIL  the TLS handshake with the gRPC target" "FAKE_GRPC_RC=35"
pf_case grpc-tls-nohttp2 0 "this curl has no HTTP/2" "FAKE_GRPC_RC=35 FAKE_HTTP2=0"
pf_case skew-warn 0 "this host's clock is 6" "FAKE_SKEW=-65"
pf_case skew-fail 1 "preflight: FAIL  this host's clock is 4" "FAKE_SKEW=420"
expect_out "fix: synchronize the clock"
pf_case no-date 0 "Control sent no readable Date header" "FAKE_NO_DATE=1"
pf_case two-fails 1 "preflight found 2 problem(s)" "FAKE_KERNEL=4.19.0 FAKE_GRPC_RC=7"
new_root pf-ports
metadata
printf 'tcp LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*\ntcp LISTEN 0 4096 [::]:20005 [::]:*\nudp UNCONN 0 0 127.0.0.1:20009 0.0.0.0:*\n' >"${STATE}/ss"
run_installer -u "${CONTROL}" -t "${TOKEN_A}" --port-range 20000-20010
expect_status 0 "ports in use"
expect_out "ports 20005 20009 of the forward port range 20000-20010 are in use"
run_installer -u "${CONTROL}" --port-range 30000-30010
expect_status 0 "free port range"
expect_out "preflight: ok    no listener in the forward port range 30000-30010"
pf_case skip 0 "preflight checks were skipped (--skip-preflight)" "FAKE_KERNEL=4.19.0" \
  -u "${CONTROL}" -t "${TOKEN_A}" --skip-preflight
[[ -x "${ROOT_DIR}/usr/lib/anixops-agent/anix-agent" ]] || fail "--skip-preflight did not install"
! grep -q 'preflight: FAIL' "${STATE}/out" || fail "--skip-preflight still ran the checks"
! grep -q 'Preflight checks for a forward node' "${STATE}/out" || fail "--skip-preflight ran the forward checks after the join"
ok "preflight checks pass, warn and fail with a fix"

# 11. IPv6 SLAAC interfaces: accept_ra = 2 by default, a warning with
# --no-accept-ra, and kept by a later run with it.
pf_case slaac 0 "ignore router advertisements on eth0" "FAKE_RA=eth0" -u "${CONTROL}" -t "${TOKEN_A}" --forward --no-accept-ra
! grep -q 'accept_ra' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "accept_ra written with --no-accept-ra"
FAKE_RA=eth0 run_installer -u "${CONTROL}"
expect_status 0 "default accept_ra"
expect_out "the drop-in sets accept_ra = 2 on eth0"
grep -qxF 'net.ipv6.conf.eth0.accept_ra = 2' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "the default wrote no accept_ra"
expect_out "accept_ra = 2 on eth0); applied"
FAKE_RA=eth0 run_installer -u "${CONTROL}" --no-accept-ra
expect_status 0 "re-run keeps accept_ra"
[[ "$(grep -c 'accept_ra = 2' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf")" == "1" ]] || fail "a re-run lost or doubled accept_ra"
FAKE_RA=eth0 run_installer -u "${CONTROL}" --accept-ra
expect_status 0 "the old --accept-ra flag still works"
ok "IPv6 accept_ra for SLAAC interfaces"

# 12. Uninstall keeps the identity; --purge removes everything the Agent
# owns and nothing it does not.
new_root uninstall
metadata
run_installer -u "${CONTROL}" -t "${TOKEN_A}"
expect_status 0 "install before uninstall"
printf 'inet anixops_fwd\n' >>"${STATE}/nft-tables"
printf 'inet anixops_fwd\n' >"${STATE}/nft-owned"
printf 'qdisc htb af00: root refcnt 2 r2q 10 default 0 direct_packets_stat 0\n' >"${STATE}/tc-eth0"
printf 'qdisc htb 1: root refcnt 2 r2q 10 default 0x10\n' >"${STATE}/tc-eth1"
: >"${STATE}/log"
run_installer uninstall
expect_status 0 "uninstall"
for gone in etc/systemd/system/anix-agent.service usr/lib/anixops-agent \
  usr/local/bin/anix-agent etc/polkit-1/rules.d/50-anixops-agent.rules \
  etc/systemd/system/anixops-agent-updater.service etc/systemd/system/anixops-agent-updater.path; do
  [[ ! -e "${ROOT_DIR}/${gone}" && ! -L "${ROOT_DIR}/${gone}" ]] || fail "uninstall left ${gone}"
done
for kept in etc/anixops/agent/config.json var/lib/anixops-agent/pki/identity.json etc/sysctl.d/90-anixops-forward.conf; do
  [[ -e "${ROOT_DIR}/${kept}" ]] || fail "uninstall without --purge removed ${kept}"
done
grep -qx 'inet anixops_fwd' "${STATE}/nft-tables" || fail "uninstall without --purge removed the forwarding table"
[[ -f "${STATE}/tc-eth0" ]] || fail "uninstall without --purge removed the qdisc"
grep -qx anixops-agent "${STATE}/passwd" || fail "uninstall without --purge removed the user"
[[ "$(grep -n 'systemctl disable --now anixops-agent-updater.path' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n 'systemctl disable --now anix-agent.service' "${STATE}/log" | cut -d: -f1)" ]] ||
  fail "the updater must stop before the Agent"
expect_out "they stay in the kernel until a reboot or uninstall --purge"
expect_out "revoke its credentials or delete the node there"
run_installer -u "${CONTROL}"
expect_status 0 "re-install after uninstall"
expect_out "already joined as forward-41"
ok "uninstall keeps the identity for a re-install"

: >"${STATE}/log"
printf '1\n' >"${ROOT_DIR}/proc/sys/net/ipv4/ip_forward"
run_installer uninstall --purge
expect_status 0 "uninstall --purge"
for gone in etc/anixops var/lib/anixops-agent etc/sysctl.d/90-anixops-forward.conf usr/lib/anixops-agent; do
  [[ ! -e "${ROOT_DIR}/${gone}" ]] || fail "uninstall --purge left ${gone}"
done
! grep -qx 'inet anixops_fwd' "${STATE}/nft-tables" || fail "--purge kept the driver's table"
grep -qx 'inet filter' "${STATE}/nft-tables" || fail "--purge removed a foreign table"
[[ ! -f "${STATE}/tc-eth0" ]] || fail "--purge kept the driver's qdisc"
[[ -f "${STATE}/tc-eth1" ]] || fail "--purge removed a foreign qdisc"
! grep -q 'tc qdisc del dev eth1' "${STATE}/log" || fail "--purge touched eth1's qdisc"
# systemd-journal is the host's group: --purge leaves it.
[[ ! -s "${STATE}/passwd" && "$(cat "${STATE}/group")" == "systemd-journal" ]] ||
  fail "--purge kept users or groups: $(cat "${STATE}/passwd" "${STATE}/group")"
expect_out "nftables table inet anixops_fwd"
expect_out "tc root qdisc af00: on eth0"
expect_out "IP forwarding stays on until the next boot"
printf '0\n' >"${ROOT_DIR}/proc/sys/net/ipv4/ip_forward"
run_installer uninstall --purge
expect_status 0 "uninstall on a clean host"
expect_out "removed:     nothing"
! grep -q 'IP forwarding stays on' "${STATE}/out" || fail "a host without forwarding was told it stays on"
new_root foreign
printf 'inet anixops_fwd\n' >>"${STATE}/nft-tables"
run_installer uninstall --purge
expect_status 0 "purge with a foreign anixops_fwd"
grep -qx 'inet anixops_fwd' "${STATE}/nft-tables" || fail "--purge deleted a table without the ownership comment"
expect_out "it has no 'anixops-forward-driver v1' comment"
ok "uninstall --purge removes only what the Agent owns"

printf 'agent installer: %d checks passed\n' "${PASS}"
