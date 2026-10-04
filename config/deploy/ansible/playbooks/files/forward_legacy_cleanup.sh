#!/usr/bin/env bash
# Remove the legacy forward runtime of this host and verify it is gone
# (v4.2 upgrade, F5c). Arguments: MASQUERADE rules to remove, each as
# "proto,host,port" (the targets of the flux forwards this host ran).
#
# It touches only the objects it names; it exits 0 when none is left and 3
# when one still is (printing which).
set -euo pipefail

NFT=${NFT:-nft}
IPTABLES=${IPTABLES:-iptables}
SYSTEMCTL=${SYSTEMCTL:-systemctl}
ROOT=${LEGACY_ROOT:-}

readonly LEGACY_NFT_TABLES=("inet v2b_forward" "ip v2b_forward" "ip anixops_forward")
readonly LEGACY_UNIT="v2forward-agent.service"
readonly LEGACY_PATHS=("/etc/v2board-forward-agent" "/usr/local/bin/v2forward-agent")
readonly CHAIN_RE='^V2B_FWD_[0-9]+_(TCP|UDP)$'

remaining=()
removed=()

have() { command -v "$1" >/dev/null 2>&1; }

# nftables tables.
if have "${NFT}"; then
  for table in "${LEGACY_NFT_TABLES[@]}"; do
    # shellcheck disable=SC2086
    if "${NFT}" list table ${table} >/dev/null 2>&1; then
      # shellcheck disable=SC2086
      "${NFT}" delete table ${table} && removed+=("nft table ${table}") || true
    fi
    # shellcheck disable=SC2086
    if "${NFT}" list table ${table} >/dev/null 2>&1; then
      remaining+=("nft table ${table}")
    fi
  done
fi

# iptables chains of the iptables runtime and the rules using them.
legacy_chains() {
  "${IPTABLES}" -t nat -S 2>/dev/null | awk -v re="${CHAIN_RE}" '$1 == "-N" && $2 ~ re { print $2 }'
}
legacy_jumps() {
  "${IPTABLES}" -t nat -S PREROUTING 2>/dev/null | awk -v re="${CHAIN_RE}" '$1 == "-A" && $NF ~ re && $(NF-1) == "-j"'
}
if have "${IPTABLES}"; then
  while read -r rule; do
    [[ -n "${rule}" ]] || continue
    read -r -a parts <<<"${rule}"
    parts[0]="-D"
    "${IPTABLES}" -t nat "${parts[@]}" && removed+=("iptables nat ${rule#-A }") || true
  done < <(legacy_jumps)
  for spec in "$@"; do
    IFS=, read -r proto host port <<<"${spec}"
    [[ "${proto}" =~ ^(tcp|udp)$ && -n "${host}" && "${port}" =~ ^[0-9]+$ ]] || continue
    while "${IPTABLES}" -t nat -C POSTROUTING -p "${proto}" -d "${host}" --dport "${port}" -j MASQUERADE 2>/dev/null; do
      "${IPTABLES}" -t nat -D POSTROUTING -p "${proto}" -d "${host}" --dport "${port}" -j MASQUERADE || break
      removed+=("iptables nat MASQUERADE ${proto} ${host}:${port}")
    done
  done
  while read -r chain; do
    [[ -n "${chain}" ]] || continue
    "${IPTABLES}" -t nat -F "${chain}" || true
    "${IPTABLES}" -t nat -X "${chain}" && removed+=("iptables nat chain ${chain}") || true
  done < <(legacy_chains)
  while read -r chain; do
    [[ -n "${chain}" ]] && remaining+=("iptables nat chain ${chain}")
  done < <(legacy_chains)
  while read -r rule; do
    [[ -n "${rule}" ]] && remaining+=("iptables nat ${rule}")
  done < <(legacy_jumps)
fi

# The clean agent, as the Agent's installer removes it.
unit_files=("/etc/systemd/system/${LEGACY_UNIT}" "/lib/systemd/system/${LEGACY_UNIT}" "/usr/lib/systemd/system/${LEGACY_UNIT}")
for unit_file in "${unit_files[@]}"; do
  if [[ -f "${ROOT}${unit_file}" ]]; then
    if have "${SYSTEMCTL}"; then
      "${SYSTEMCTL}" disable --now "${LEGACY_UNIT}" >/dev/null 2>&1 || true
    fi
    rm -f -- "${ROOT}${unit_file}"
    removed+=("systemd unit ${unit_file}")
  fi
  if [[ -f "${ROOT}${unit_file}" ]]; then
    remaining+=("systemd unit ${unit_file}")
  fi
done
for legacy_path in "${LEGACY_PATHS[@]}"; do
  if [[ -e "${ROOT}${legacy_path}" || -L "${ROOT}${legacy_path}" ]]; then
    rm -rf -- "${ROOT}${legacy_path}"
    removed+=("${legacy_path}")
  fi
  if [[ -e "${ROOT}${legacy_path}" || -L "${ROOT}${legacy_path}" ]]; then
    remaining+=("${legacy_path}")
  fi
done
if have "${SYSTEMCTL}"; then
  "${SYSTEMCTL}" daemon-reload >/dev/null 2>&1 || true
fi

for item in "${removed[@]+"${removed[@]}"}"; do
  echo "removed: ${item}"
done
if ((${#remaining[@]} > 0)); then
  for item in "${remaining[@]}"; do
    echo "LEGACY_FORWARD_DIRTY: ${item}" >&2
  done
  exit 3
fi
echo "LEGACY_FORWARD_CLEAN"
