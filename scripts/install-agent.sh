#!/usr/bin/env bash
# Migrate one drained/low-risk V2bX node to the AnixOps Agent runtime.
# This installer only changes the control process; the legacy REST/UniProxy
# data plane remains available through the preserved node configuration.
set -Eeuo pipefail

REPO_OWNER="${REPO_OWNER:-AnixOps}"; REPO_NAME="${REPO_NAME:-anix-agent}"
VERSION="${ANIX_AGENT_VERSION:-}"; COMMAND="install"; SKIP_START=0
OLD_DIR="${OLD_DIR:-/usr/local/V2bX}"; OLD_CONFIG_DIR="${OLD_CONFIG_DIR:-/etc/V2bX}"
OLD_SERVICE="${OLD_SERVICE:-V2bX.service}"; INSTALL_DIR="${INSTALL_DIR:-/usr/local/anixops-agent}"
CONFIG_DIR="${CONFIG_DIR:-/etc/anixops/agent}"; BACKUP_ROOT="${BACKUP_ROOT:-/var/backups/anixops-agent}"
SERVICE="anix-agent.service"

die(){ printf '[ERROR] %s\n' "$*" >&2; exit 1; }
info(){ printf '[INFO] %s\n' "$*"; }
usage(){ printf 'Usage: install-agent.sh [install|migrate|rollback] --version <tag> [--skip-start]\n'; }
parse(){
  case "${1:-}" in install|migrate|rollback) COMMAND="$1"; shift;; esac
  while (($#)); do case "$1" in --version) VERSION="${2:-}"; shift 2;; --skip-start) SKIP_START=1; shift;; -h|--help) usage; exit 0;; *) die "Unknown argument: $1";; esac; done
}
need_root(){ [[ $EUID -eq 0 ]] || die 'Run as root'; }
asset(){ case "$(uname -m)" in x86_64|amd64) echo anix-agent-linux-amd64.tar.gz;; aarch64|arm64) echo anix-agent-linux-arm64.tar.gz;; *) die 'Unsupported architecture';; esac; }
download(){ local n="$1"; curl -fL --retry 3 "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${n}" -o "${TMP}/${n}"; }
write_unit(){
  cat > "/etc/systemd/system/${SERVICE}" <<EOF
[Unit]
Description=AnixOps Agent
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/anix-agent --config ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ReadWritePaths=${INSTALL_DIR} ${CONFIG_DIR}
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload; systemctl enable "${SERVICE}" >/dev/null
}
backup(){
  SNAP="${BACKUP_ROOT}/$(date -u +%Y%m%dT%H%M%SZ)-migration"; install -d -m 0700 "${SNAP}"
  [[ -d "$OLD_DIR" ]] && cp -a "$OLD_DIR" "$SNAP/old-program"
  [[ -d "$OLD_CONFIG_DIR" ]] && cp -a "$OLD_CONFIG_DIR" "$SNAP/old-config"
  systemctl cat "$OLD_SERVICE" > "$SNAP/old.service" 2>/dev/null || true
  printf '%s\n' "$SNAP" >&2
  printf '%s\n' "$SNAP"
}
rollback(){
  local s="${1:-}"; [[ -n "$s" ]] || s="$(find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -type d -name '*-migration' 2>/dev/null | sort | tail -n1)"; [[ -d "$s" ]] || die 'No Agent migration snapshot found'
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  [[ -d "$s/old-program" ]] && rm -rf "$OLD_DIR" && cp -a "$s/old-program" "$OLD_DIR"
  [[ -d "$s/old-config" ]] && rm -rf "$OLD_CONFIG_DIR" && cp -a "$s/old-config" "$OLD_CONFIG_DIR"
  [[ -f "$s/old.service" ]] && cp -a "$s/old.service" "/etc/systemd/system/${OLD_SERVICE}"
  systemctl daemon-reload; systemctl enable "$OLD_SERVICE" >/dev/null; systemctl start "$OLD_SERVICE"; info "Rolled back Agent from $s"
}
migrate(){
  [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+)?)?$ ]] || die 'migrate requires --version <tag>'
  [[ -d "$OLD_DIR" || -d "$OLD_CONFIG_DIR" ]] || die 'Legacy V2bX layout not found'
  TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
  local a; a="$(asset)"; download "$a"; download SHA256SUMS.txt
  local expected; expected="$(awk -v n="$a" '$2==n{print $1; exit}' "$TMP/SHA256SUMS.txt")"; [[ "$expected" == "$(sha256sum "$TMP/$a"|awk '{print $1}')" ]] || die 'Agent checksum verification failed'
  local snap; snap="$(backup)"; install -d -m 0755 "$INSTALL_DIR" "$CONFIG_DIR"
  [[ -d "$OLD_CONFIG_DIR" ]] && cp -a "$OLD_CONFIG_DIR/." "$CONFIG_DIR/"
  tar -xzf "$TMP/$a" -C "$INSTALL_DIR"; [[ -x "$INSTALL_DIR/anix-agent" ]] || { [[ -f "$INSTALL_DIR/anix-agent-linux" ]] && mv "$INSTALL_DIR/anix-agent-linux" "$INSTALL_DIR/anix-agent"; }; chmod 0755 "$INSTALL_DIR/anix-agent"
  # TLS is the default. Existing node API credentials remain in the copied config.
  if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then printf 'control:\n  tls:\n    enabled: true\n    allow_insecure: false\n' > "$CONFIG_DIR/config.yaml"; fi
  # Never inherit an insecure control channel from the legacy configuration.
  sed -i -E 's/^([[:space:]]*allow_insecure:[[:space:]]*).*/\1false/' "$CONFIG_DIR/config.yaml" || true
  write_unit; [[ "$SKIP_START" -eq 1 ]] || { systemctl stop "$OLD_SERVICE" >/dev/null 2>&1 || true; systemctl start "$SERVICE"; }
  info "Agent canary installed; legacy data plane remains available"
}
parse "$@"; [[ "$COMMAND" == rollback ]] && { need_root; rollback; exit 0; }; need_root; migrate
