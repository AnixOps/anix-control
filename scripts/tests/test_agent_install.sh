#!/usr/bin/env bash
# shellcheck disable=SC2317 # the fakes are called by the sourced installer.
# Tests of the Agent installer (internal/agentinstall/install.sh) in a fake
# root: systemctl, curl, nft, sysctl, the user tools and chown are shell functions,
# the Agent is a fake binary that "enrolls" when systemctl restarts it.
# Nothing here touches the host's system.

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
readonly GITHUB="https://github.com/AnixOps/anix-agent/releases/download"

# build_release makes the fake Agent release zip and its .dgst.
build_release() {
  local dir="${WORK}/release"
  mkdir -p "${dir}/content"
  cat >"${dir}/content/anix-agent" <<'EOF'
#!/usr/bin/env bash
# Fake AnixOps Agent: `identity --json` prints what the fake systemctl
# recorded when it "enrolled".
# `forward sysctl-dropin` and `migrate-paths` follow FAKE_DROPIN,
# FAKE_MIGRATE_HELP and FAKE_MIGRATE; migrate-paths logs its arguments.
case "${1:-}" in
  identity)
    cat "${ANIX_INSTALL_ROOT}/var/lib/anixops-agent/pki/identity.json" 2>/dev/null || printf '[]\n'
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
# forward runtime.
new_root() {
  ROOT_DIR="${WORK}/root-$1"
  mkdir -p "${ROOT_DIR}/run/systemd/system" "${ROOT_DIR}/etc/systemd/system" "${ROOT_DIR}/etc/polkit-1/rules.d" \
    "${ROOT_DIR}/etc/v2board-forward-agent" "${ROOT_DIR}/usr/local/bin" "${WORK}/state-$1"
  printf '[Unit]\nDescription=clean agent\n' >"${ROOT_DIR}/etc/systemd/system/v2forward-agent.service"
  printf '[Unit]\nDescription=WireGuard gost relay\n' >"${ROOT_DIR}/etc/systemd/system/gost-wg-relay.service"
  printf 'panel_url: x\n' >"${ROOT_DIR}/etc/v2board-forward-agent/config.yaml"
  printf '#!/bin/sh\n' >"${ROOT_DIR}/usr/local/bin/v2forward-agent"
  STATE="${WORK}/state-$1"
  printf 'inet v2b_forward\nip anixops_forward\ninet filter\n' >"${STATE}/nft-tables"
  : >"${STATE}/passwd"
  : >"${STATE}/group"
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
    uname() { case "${1:-}" in -s) printf 'Linux\n' ;; -m) printf '%s\n' "${FAKE_ARCH:-x86_64}" ;; *) command uname "$@" ;; esac; }
    sleep() { :; }
    chown() { printf 'chown %s\n' "$*" >>"${STATE}/log"; }
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
      local verb="$1" family="$3" name="$4"
      grep -qx "${family} ${name}" "${STATE}/nft-tables" || return 1
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
      printf 'systemctl %s\n' "$*" >>"${STATE}/log"
      if [[ "$1" == "restart" && "$2" == "anix-agent.service" ]]; then
        # The fake Agent enrolls with the credential file, then removes it.
        local credential="${ROOT_DIR}/var/lib/anixops-agent/enroll.credential" node
        if [[ -f "${credential}" ]]; then
          stat -c '%a' "${credential}" >"${STATE}/credential-mode"
          node="$(grep -o '"AgentNode": "[a-z]*-[0-9]*"' "${ROOT_DIR}/etc/anixops/agent/config.json" | cut -d '"' -f 4)"
          if [[ "${FAKE_ENROLL:-1}" == "1" ]]; then
            printf '[{"node": "%s", "dir": "x", "state": "valid", "spiffe_id": "spiffe://anixops/prod/agent/%s", "serial": "abc123", "not_after": "2026-10-10T00:00:00Z", "trust_bundle_cas": 1}]\n' \
              "${node}" "${node}" >"${ROOT_DIR}/var/lib/anixops-agent/pki/identity.json"
            rm -f "${credential}"
          fi
          printf 'enrolled-with %s\n' "$(cat "${credential}" 2>/dev/null || printf 'consumed')" >>"${STATE}/log"
        fi
      fi
      return 0
    }
    curl() {
      local out="" url=""
      while (($# > 0)); do
        case "$1" in
          -o) out="$2"; shift 2 ;;
          --proto | --retry | --connect-timeout) shift 2 ;;
          -*) shift ;;
          *) url="$1"; shift ;;
        esac
      done
      printf 'curl %s\n' "${url}" >>"${STATE}/log"
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

# 1. Fresh install: user, binary, sandboxed unit, config without an API key,
# credential 0600 consumed by the Agent, legacy runtime removed.
new_root fresh
metadata
run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 0 "fresh install"
[[ -x "${ROOT_DIR}/usr/lib/anixops-agent/anix-agent" ]] || fail "binary not installed"
[[ "$(readlink "${ROOT_DIR}/usr/local/bin/anix-agent")" == "/usr/lib/anixops-agent/anix-agent" ]] || fail "anix-agent link"
config="${ROOT_DIR}/etc/anixops/agent/config.json"
python3 -m json.tool "${config}" >/dev/null || fail "config is not JSON"
grep -qF '"AgentNode": "forward-41"' "${config}" || fail "config lacks the node"
grep -qF '"NodeID": 41' "${config}" || fail "config lacks the node id"
grep -qF '"GRPCHost": "ctl.example.com:50051"' "${config}" || fail "config lacks the gRPC target"
grep -qF '"EnrollCredentialFile": "/var/lib/anixops-agent/enroll.credential"' "${config}" || fail "config lacks the credential file"
! grep -q 'ApiKey' "${config}" || fail "config must not carry an API key"
[[ "$(stat -c '%a' "${config}")" == "640" ]] || fail "config mode"
[[ "$(cat "${STATE}/credential-mode")" == "600" ]] || fail "credential mode was $(cat "${STATE}/credential-mode")"
[[ ! -e "${ROOT_DIR}/var/lib/anixops-agent/enroll.credential" ]] || fail "credential left behind"
[[ "$(stat -c '%a' "${ROOT_DIR}/var/lib/anixops-agent")" == "700" ]] || fail "state dir mode"
[[ "$(stat -c '%a' "${ROOT_DIR}/var/lib/anixops-gost")" == "750" ]] || fail "gost dir mode"
unit="${ROOT_DIR}/etc/systemd/system/anix-agent.service"
for line in "User=anixops-agent" "SupplementaryGroups=anixops-gost" "AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE" \
  "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE" "NoNewPrivileges=yes" "ProtectSystem=strict" \
  "ReadWritePaths=/var/lib/anixops-agent /var/lib/anixops-gost" "RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_UNIX" \
  "ExecStart=/usr/lib/anixops-agent/anix-agent server -c /etc/anixops/agent/config.json"; do
  grep -qxF "${line}" "${unit}" || fail "agent unit lacks ${line}"
done
diff -u "${REPO_ROOT}/contracts/forward/v1/gost/anixops-gost.service" "${ROOT_DIR}/etc/systemd/system/anixops-gost.service" >/dev/null ||
  fail "gost unit differs from the contract"
grep -qF '"anixops-gost.service"' "${ROOT_DIR}/etc/polkit-1/rules.d/50-anixops-agent.rules" || fail "polkit rule"
grep -qF 'useradd --system --gid anixops-agent --groups anixops-gost' "${STATE}/log" || fail "agent user not in the gost group"
grep -qF 'systemctl enable anix-agent.service' "${STATE}/log" || fail "service not enabled"
[[ "$(cat "${STATE}/nft-tables")" == "inet filter" ]] || fail "legacy tables left or foreign table removed: $(cat "${STATE}/nft-tables")"
[[ ! -e "${ROOT_DIR}/etc/systemd/system/v2forward-agent.service" && ! -e "${ROOT_DIR}/etc/v2board-forward-agent" && ! -e "${ROOT_DIR}/usr/local/bin/v2forward-agent" ]] ||
  fail "clean agent not removed"
[[ -f "${ROOT_DIR}/etc/systemd/system/gost-wg-relay.service" ]] || fail "the WireGuard gost relay must not be touched"
expect_out "nftables table inet v2b_forward"
expect_out "nftables table ip anixops_forward"
expect_out "systemd unit v2forward-agent.service"
expect_out "AnixOps Agent v4.2.0 is running (install)"
sysctl_file="${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf"
grep -qxF 'net.ipv4.ip_forward = 1' "${sysctl_file}" || fail "sysctl file lacks IPv4 forwarding"
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
! grep -qF "${TOKEN_A}" "${STATE}/out" || fail "the token was printed"
grep -qF "curl ${GITHUB}/v4.2.0/anix-agent-linux-64.zip.dgst" "${STATE}/log" || fail "checksum not from GitHub"
ok "fresh install"

# 2. The same command again: upgrade in place, identity kept, token unused.
: >"${STATE}/log"
rm -f "${STATE}/credential-mode"
run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 0 "re-run"
expect_out "(upgrade)"
expect_out "already enrolled as forward-41; the token was not used"
expect_out "kept /etc/anixops/agent/config.json"
expect_out "legacy:      nothing to remove"
[[ ! -e "${STATE}/credential-mode" ]] || fail "re-run wrote the credential"
grep -qF 'systemctl stop anix-agent.service' "${STATE}/log" || fail "re-run did not stop the old Agent first"
ok "re-run is an in-place upgrade"

# 3. Another node needs --reset; with it the host enrolls again.
run_installer --control "${CONTROL}" --node forward-42 --token "${TOKEN_B}"
expect_status 1 "other node without --reset"
expect_out "re-run with --reset"
run_installer --control "${CONTROL}" --node forward-42 --token "${TOKEN_B}" --reset
expect_status 0 "reset"
grep -qF '"AgentNode": "forward-42"' "${config}" || fail "reset did not rewrite the config"
compgen -G "${config}.bak.*" >/dev/null || fail "reset kept no backup of the config"
expect_out "Discarded this host's Agent identity"
ok "--reset switches the node"

# 4. A checksum mismatch installs nothing.
new_root mismatch
metadata
printf 'not the release\n' >"${WORK}/bad.zip"
FAKE_ZIP="${WORK}/bad.zip" run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 1 "checksum mismatch"
expect_out "checksum mismatch"
[[ ! -e "${ROOT_DIR}/usr/lib/anixops-agent" && ! -e "${ROOT_DIR}/etc/systemd/system/anix-agent.service" ]] || fail "a bad download changed the host"
[[ -f "${ROOT_DIR}/etc/systemd/system/v2forward-agent.service" ]] || fail "a bad download removed the legacy runtime"
ok "checksum mismatch installs nothing"

# 5. Mirrors: Control's digest is used and the cn mirror's never fetched.
new_root mirror
metadata "source cn https://mirror.example.cn/anix-agent" "source control ${CONTROL}/install/agent" \
  "sha256 anix-agent-linux-64.zip $(sha256sum "${RELEASE_ZIP}" | awk '{print $1}')"
run_installer --control "${CONTROL}" --node proxy-7 --token "${TOKEN_A}" --mirror cn
expect_status 0 "cn mirror"
grep -qF 'curl https://mirror.example.cn/anix-agent/v4.2.0/anix-agent-linux-64.zip' "${STATE}/log" || fail "cn mirror not used"
[[ ! -e "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ]] || fail "a proxy node got IP forwarding without --forward"
! grep -q '^sysctl ' "${STATE}/log" || fail "sysctl ran for a proxy node without --forward"
expect_out "forwarding:  unchanged (a proxy node; --forward turns IP forwarding on)"
! grep -q '\.dgst' "${STATE}/log" || fail "a .dgst was fetched although Control published the digest"
new_root fallback
metadata
run_installer --control "${CONTROL}" --node proxy-7 --token "${TOKEN_A}" --mirror cn
expect_status 0 "cn fallback"
expect_out "the cn mirror is not set up on Control; downloading from GitHub releases"
ok "mirror selection and trusted digests"

# 5b. IP forwarding: a proxy node with --forward gets it; a sysctl that
# cannot apply leaves the file for the next boot and only notes it.
new_root proxyforward
metadata
run_installer --control "${CONTROL}" --node proxy-7 --token "${TOKEN_A}" --forward
expect_status 0 "proxy --forward"
grep -qxF 'net.ipv4.ip_forward = 1' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "--forward wrote no sysctl file"
expect_out "; applied"
new_root sysctlfail
metadata
FAKE_DROPIN=0 FAKE_SYSCTL=fail run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 0 "sysctl failure is not fatal"
[[ -f "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ]] || fail "sysctl failure removed the file"
grep -qF 'Written by the AnixOps installer' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" ||
  fail "an Agent without sysctl-dropin did not get the installer's drop-in"
grep -qxF 'net.ipv6.conf.all.forwarding = 1' "${ROOT_DIR}/etc/sysctl.d/90-anixops-forward.conf" || fail "installer drop-in lacks IPv6"
expect_out "sysctl could not apply /etc/sysctl.d/90-anixops-forward.conf now"
expect_out "; applies at the next boot"
ok "IP forwarding for forward nodes"

# 5c. Switching from anix-agent's root install: migrate-paths copies its
# directories, owned by anixops-agent, before the new unit starts.
root_switch() {
  new_root "$1"
  metadata
  printf '[Unit]\nDescription=anix-agent (root)\n[Service]\nExecStart=/usr/local/bin/anix-agent server\n' \
    >"${ROOT_DIR}/etc/systemd/system/anix-agent.service"
}
root_switch rootunit
run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 0 "switch from a root unit"
grep -qxF "agent migrate-paths --chown anixops-agent --root ${ROOT_DIR}" "${STATE}/log" || fail "migrate-paths not run: $(grep agent "${STATE}/log")"
[[ "$(grep -n '^agent migrate-paths' "${STATE}/log" | cut -d: -f1)" -lt "$(grep -n '^systemctl restart anix-agent.service' "${STATE}/log" | cut -d: -f1)" ]] ||
  fail "migrate-paths must run before the new unit starts"
grep -qxF "User=anixops-agent" "${ROOT_DIR}/etc/systemd/system/anix-agent.service" || fail "the root unit was not replaced"
expect_out "migration:   copied the root install's directories to /var/lib/anixops-agent"
new_root rootdirs
metadata
mkdir -p "${ROOT_DIR}/var/lib/anixops/plugins"
run_installer --control "${CONTROL}" --node proxy-7 --token "${TOKEN_A}"
expect_status 0 "switch with the root install's directories"
grep -q '^agent migrate-paths --chown anixops-agent' "${STATE}/log" || fail "old directories did not trigger migrate-paths"
root_switch rootfail
FAKE_MIGRATE=1 run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 1 "migrate-paths failure"
expect_out "anix-agent migrate-paths could not copy"
! grep -q '^systemctl restart anix-agent.service' "${STATE}/log" || fail "the Agent started after a failed migration"
root_switch rootold
FAKE_MIGRATE_HELP=1 run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
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
  ANIX_INSTALL_PUBLIC_KEY="${public}" FAKE_SIG="${WORK}/zip.sig" run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
  expect_status 0 "signed release"
  expect_out "Release signature verified"
  new_root badsig
  metadata
  FAKE_SIG="${WORK}/zip.sig" run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
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
else
  printf 'skip - release signatures (OpenSSL without -rawin)\n'
fi

# 7. Enrollment that never completes fails with a hint and keeps the install.
new_root stuck
metadata
FAKE_ENROLL=0 run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}" --timeout 4
expect_status 1 "enrollment timeout"
expect_out "did not enroll within 4s"
expect_out "journalctl -u anix-agent.service"
ok "enrollment timeout"

# 8. Arguments and platforms.
new_root args
metadata
for case in \
  "2|node-group tokens are not supported|--control ${CONTROL} --node forward-41 --token ${TOKEN_A} --group edge" \
  "2|offline packages are not part of this installer release|--control ${CONTROL} --node forward-41 --offline pkg.tar" \
  "2|uninstall is not part of this installer release|uninstall" \
  "2|--control is required|--node forward-41 --token ${TOKEN_A}" \
  "2|--control must be https|--control http://ctl.example.com --node forward-41 --token ${TOKEN_A}" \
  "2|--node must be proxy-<id> or forward-<id>|--control ${CONTROL} --node module-1 --token ${TOKEN_A}" \
  "2|is not an AnixOps enrollment token|--control ${CONTROL} --node forward-41 --token secret" \
  "2|--mirror must be control, cn or github|--control ${CONTROL} --node forward-41 --token ${TOKEN_A} --mirror ftp" \
  "2|--reset needs --token|--control ${CONTROL} --node forward-41 --reset" \
  "2|unknown argument|--control ${CONTROL} --node forward-41 --bogus" \
  "1|is not enrolled: --token is required|--control=${CONTROL} --node=forward-41"; do
  IFS='|' read -r want message args <<<"${case}"
  # shellcheck disable=SC2086 # the case's arguments are words.
  run_installer ${args}
  expect_status "${want}" "${args}"
  expect_out "${message}"
done
FAKE_ARCH=riscv64 run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 1 "riscv64"
expect_out "unsupported architecture riscv64"
rm -rf "${ROOT_DIR}/run/systemd"
run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 1 "no systemd"
expect_out "systemd is required"
mkdir -p "${ROOT_DIR}/sbin" && : >"${ROOT_DIR}/sbin/openrc-run"
run_installer --control "${CONTROL}" --node forward-41 --token "${TOKEN_A}"
expect_status 1 "OpenRC"
expect_out "OpenRC is not supported"
ok "arguments and platforms"

printf 'agent installer: %d checks passed\n' "${PASS}"
