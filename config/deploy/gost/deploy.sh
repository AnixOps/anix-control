#!/bin/bash
set -euo pipefail

NODE_TYPE=${1:-relay}
API_TOKEN=${2:-}
GOST_VERSION="3.2.6"
GOST_DIR="/opt/gost"
CONFIG_FILE="$GOST_DIR/gost.yml"
SERVICE_FILE="/etc/systemd/system/gost.service"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

if [ "$EUID" -ne 0 ]; then
    log_error "Please run this script as root."
    exit 1
fi

if [ -z "$API_TOKEN" ]; then
    log_error "API token is required."
    echo "Usage: $0 <node_type> <api_token>"
    exit 1
fi

install_gost() {
    log_info "Installing gost $GOST_VERSION ..."

    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l) ARCH="armv7" ;;
        *)
            log_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    DOWNLOAD_URL="https://github.com/go-gost/gost/releases/download/v${GOST_VERSION}/gost_${GOST_VERSION}_${OS}_${ARCH}.tar.gz"

    log_info "Downloading $DOWNLOAD_URL"
    curl -fsSL -o /tmp/gost.tar.gz "$DOWNLOAD_URL"

    mkdir -p "$GOST_DIR"
    tar -xzf /tmp/gost.tar.gz -C "$GOST_DIR"
    chmod +x "$GOST_DIR/gost"
    ln -sf "$GOST_DIR/gost" /usr/local/bin/gost
}

create_config() {
    log_info "Writing gost config to $CONFIG_FILE"

    cat > "$CONFIG_FILE" << EOF
# Relay role: $NODE_TYPE
# Generated at: $(date -Is)

chains: []
hops: []

authers:
  - name: auther-0
    auths:
      - username: admin
        password: $API_TOKEN

api:
  addr: ":18080"
  pathPrefix: /api
  accesslog: true
  auther: auther-0
  auth:
    username: admin
    password: $API_TOKEN

log:
  output: stderr
  level: info
  format: text

metrics:
  addr: :9000
  path: /metrics
EOF
}

create_service() {
    log_info "Writing systemd unit to $SERVICE_FILE"

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=gost relay service
After=network.target

[Service]
Type=simple
ExecStart=$GOST_DIR/gost -C $CONFIG_FILE
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable gost
}

start_service() {
    log_info "Starting gost ..."
    systemctl restart gost
    sleep 2

    if systemctl is-active --quiet gost; then
        log_info "gost is running"
        log_info "Relay API: http://0.0.0.0:18080/api/config/services"
        log_info "Metrics:   http://0.0.0.0:9000/metrics"
    else
        log_error "gost failed to start"
        journalctl -u gost --no-pager -n 40
        exit 1
    fi
}

main() {
    log_info "=========================================="
    log_info "gost relay deployment"
    log_info "node type: $NODE_TYPE"
    log_info "=========================================="

    if command -v gost >/dev/null 2>&1; then
        log_warn "gost already installed, refreshing binary and config"
    fi

    install_gost
    create_config
    create_service
    start_service

    log_info "Verification:"
    log_info "curl -u admin:$API_TOKEN http://127.0.0.1:18080/api/config/services"
}

main "$@"
