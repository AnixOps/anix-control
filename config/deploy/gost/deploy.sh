#!/bin/bash
# gost 转发节点一键部署脚本
# 使用方法: ./deploy.sh <node_type> <api_token>
# node_type: relay (中转) 或 exit (落地)
# api_token: 面板生成的 API Token

set -e

NODE_TYPE=${1:-relay}
API_TOKEN=${2:-}
GOST_VERSION="3.0.0-rc10"
GOST_DIR="/opt/gost"
CONFIG_FILE="$GOST_DIR/gost.yml"
SERVICE_FILE="/etc/systemd/system/gost.service"

# 颜色输出
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

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    log_error "请使用 root 权限运行此脚本"
    exit 1
fi

# 检查参数
if [ -z "$API_TOKEN" ]; then
    log_error "请提供 API Token"
    echo "使用方法: $0 <node_type> <api_token>"
    exit 1
fi

# 安装 gost
install_gost() {
    log_info "正在安装 gost $GOST_VERSION..."

    # 检测系统架构
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l) ARCH="armv7" ;;
        *)
            log_error "不支持的架构: $ARCH"
            exit 1
            ;;
    esac

    # 检测操作系统
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')

    # 下载 gost
    DOWNLOAD_URL="https://github.com/go-gost/gost/releases/download/v${GOST_VERSION}/gost_${GOST_VERSION}_${OS}_${ARCH}.tar.gz"

    log_info "下载地址: $DOWNLOAD_URL"
    curl -L -o /tmp/gost.tar.gz "$DOWNLOAD_URL"

    # 解压安装
    mkdir -p "$GOST_DIR"
    tar -xzf /tmp/gost.tar.gz -C "$GOST_DIR"
    chmod +x "$GOST_DIR/gost"

    # 创建软链接
    ln -sf "$GOST_DIR/gost" /usr/local/bin/gost

    log_info "gost 安装完成"
}

# 创建配置文件
create_config() {
    log_info "创建配置文件..."

    cat > "$CONFIG_FILE" << EOF
# gost 转发节点配置
# 节点类型: $NODE_TYPE
# 生成时间: $(date)

services:
  # API 管理服务
  - name: api
    addr: ":18080"
    handler:
      type: api
      auth:
        username: admin
        password: $API_TOKEN
    listener:
      type: tcp

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

    log_info "配置文件已创建: $CONFIG_FILE"
}

# 创建 systemd 服务
create_service() {
    log_info "创建 systemd 服务..."

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=gost tunnel service
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

    log_info "systemd 服务已创建"
}

# 启动服务
start_service() {
    log_info "启动 gost 服务..."
    systemctl start gost
    sleep 2

    if systemctl is-active --quiet gost; then
        log_info "gost 服务启动成功"
        log_info "API 地址: http://0.0.0.0:18080"
        log_info "监控地址: http://0.0.0.0:9000/metrics"
    else
        log_error "gost 服务启动失败"
        journalctl -u gost --no-pager -n 20
        exit 1
    fi
}

# 主流程
main() {
    log_info "=========================================="
    log_info "gost 转发节点部署脚本"
    log_info "节点类型: $NODE_TYPE"
    log_info "=========================================="

    # 检查是否已安装
    if command -v gost &> /dev/null; then
        log_warn "gost 已安装，跳过安装步骤"
    else
        install_gost
    fi

    create_config
    create_service
    start_service

    log_info "=========================================="
    log_info "部署完成!"
    log_info ""
    log_info "下一步:"
    log_info "1. 在面板添加此节点，填写 IP 和 API Port: 18080"
    log_info "2. API Token: $API_TOKEN"
    log_info "3. 创建转发规则"
    log_info "=========================================="
}

main "$@"