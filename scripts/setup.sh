#!/usr/bin/env bash
# V2bX 本机节点完整工作流脚本
# 用法: ./scripts/setup.sh [options]
#
# 步骤: 启动面板 → 生成AuthKey → 构建V2bX → 启动V2bX → 配置数据链路 → 验证
set -euo pipefail

# ============================ 配置 ============================
V2BOARD_DIR="${V2BOARD_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
V2BX_DIR="${V2BX_DIR:-${V2BOARD_DIR%v2board_AnixOps}V2bX_AnixOps}"
SPEEDTEST_DIR="${SPEEDTEST_DIR:-${V2BOARD_DIR%v2board_AnixOps}AnixOps-speedtest}"

PANEL_HOST="${PANEL_HOST:-127.0.0.1}"
PANEL_PORT="${PANEL_PORT:-8080}"
PANEL_URL="http://${PANEL_HOST}:${PANEL_PORT}"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@anixops.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-00..Zdw999}"
API_TOKEN="${API_TOKEN:-internal-api-token-for-v2bx-2026}"

TEST_USER_EMAIL="${TEST_USER_EMAIL:-testuser@local.dev}"
TEST_USER_PASSWORD="${TEST_USER_PASSWORD:-TestPass123!}"

V2BX_PORT="${V2BX_PORT:-9000}"
V2BX_NODE_NAME="${V2BX_NODE_NAME:-local-v2bx-node}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()     { echo -e "${RED}[ERR]${NC}   $*"; }

# ============================ 工具函数 ============================
api() {
    curl -s -X "$1" "${PANEL_URL}$2" \
        ${3:+"-H" "Content-Type: application/json"} \
        ${4:+"-H" "Authorization: Bearer $4"} \
        ${5:+"-d" "$5"}
}

json_val() { python3 -c "import sys,json;d=json.load(sys.stdin);print(d${1})"; }

# ============================ Step 1: 启动面板 ============================
step_panel() {
    info "Step 1: 启动面板"
    cd "$V2BOARD_DIR"

    if curl -sf "${PANEL_URL}/health" > /dev/null 2>&1; then
        ok "面板已运行 $(curl -sf "${PANEL_URL}/health")"
        return 0
    fi

    info "构建面板..."
    go build -o build/v2board.exe -trimpath ./cmd/server
    rm -f config/data/v2board.db
    info "启动面板 (端口 ${PANEL_PORT})..."
    nohup ./build/v2board.exe -config config/config.yaml > logs/panel.log 2>&1 &
    sleep 3

    for i in $(seq 1 10); do
        if curl -sf "${PANEL_URL}/health" > /dev/null 2>&1; then
            ok "面板已就绪"
            return 0
        fi
        sleep 1
    done
    err "面板启动超时，查看日志: tail -f logs/panel.log"
    exit 1
}

# ============================ Step 2: 生成 AuthKey ============================
step_authkey() {
    info "Step 2: 生成 AuthKey"
    cd "$V2BOARD_DIR"

    RESP=$(curl -s -X POST "${PANEL_URL}/api/v2/internal/auth-keys" \
        -H "Content-Type: application/json" \
        -H "X-API-Key: ${API_TOKEN}" \
        -d "{\"name\":\"${V2BX_NODE_NAME}\"}")

    AUTH_KEY=$(echo "$RESP" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['key'])")
    ok "AuthKey: ${AUTH_KEY}"
    echo "$AUTH_KEY" > .authkey
}

# ============================ Step 3: 构建 V2bX ============================
step_v2bx_build() {
    info "Step 3: 构建 V2bX"
    cd "$V2BX_DIR"

    if [ "${1:-}" = "rebuild" ] || [ ! -f build/V2bX.exe ]; then
        go get github.com/sagernet/quic-go@latest
        GOEXPERIMENT=jsonv2 go build -tags xray -o build/V2bX.exe -trimpath .
        ok "V2bX 构建完成"
    else
        ok "V2bX 已存在，跳过构建 (加 rebuild 参数强制重新构建)"
    fi
}

# ============================ Step 4: 创建 V2bX 配置 ============================
step_v2bx_config() {
    info "Step 4: 创建 V2bX 配置"
    cd "$V2BX_DIR"

    AUTH_KEY="${1:-$(cat .authkey 2>/dev/null || echo '')}"
    if [ -z "$AUTH_KEY" ]; then
        err "未找到 AuthKey，请先运行 Step 2"
        exit 1
    fi

    cat > config.local.json <<EOCONFIG
{
  "Log": {
    "Level": "debug",
    "Output": ""
  },
  "Cores": [
    {
      "Type": "xray",
      "Log": {
        "Level": "debug",
        "Timestamp": true
      },
      "NTP": {
        "Enable": true,
        "Server": "time.apple.com",
        "ServerPort": 0
      }
    }
  ],
  "Nodes": [
    {
      "Core": "xray",
      "ApiHost": "http://${PANEL_HOST}:${PANEL_PORT}",
      "AutoRegister": true,
      "AuthKey": "${AUTH_KEY}",
      "NodeName": "${V2BX_NODE_NAME}",
      "NodeHost": "${PANEL_HOST}",
      "NodePort": ${V2BX_PORT},
      "CredentialFile": "data/credential.json",
      "HeartbeatInterval": 30,
      "EnableSign": true,
      "EncryptCredential": true,
      "Timeout": 30,
      "ListenIP": "0.0.0.0",
      "SendIP": "0.0.0.0",
      "DeviceOnlineMinTraffic": 200,
      "MinReportTraffic": 0,
      "TCPFastOpen": false,
      "SniffEnabled": true,
      "CertConfig": {
        "CertMode": "none",
        "RejectUnknownSni": false,
        "CertDomain": "localhost",
        "CertFile": "",
        "KeyFile": "",
        "Provider": "",
        "DNSEnv": {}
      }
    }
  ]
}
EOCONFIG
    ok "V2bX 配置已写入 config.local.json"
}

# ============================ Step 5: 启动 V2bX ============================
step_v2bx_start() {
    info "Step 5: 启动 V2bX"
    cd "$V2BX_DIR"

    # 检查 V2bX 是否已在运行
    if command -v pgrep > /dev/null 2>&1; then
        if pgrep -f "V2bX.exe" > /dev/null 2>&1; then
            warn "V2bX 已在运行，先停止旧进程"
            pkill -f "V2bX.exe" || true
            sleep 1
        fi
    fi

    rm -f data/credential.json.enc
    nohup GOEXPERIMENT=jsonv2 ./build/V2bX.exe -c config.local.json > logs/v2bx.log 2>&1 &
    sleep 3

    for i in $(seq 1 10); do
        if grep -q "Added.*new users" logs/v2bx.log 2>/dev/null || \
           grep -q "No users found" logs/v2bx.log 2>/dev/null; then
            ok "V2bX 已启动"
            return 0
        fi
        sleep 1
    done
    warn "V2bX 已启动 (等待用户数据链路配置)"
}

# ============================ Step 6: 配置数据链路 ============================
step_datachain() {
    info "Step 6: 配置数据链路"
    cd "$V2BOARD_DIR"

    # 登录管理员
    ADMIN_TOKEN=$(curl -s -X POST "${PANEL_URL}/api/v2/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
        | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
    ok "管理员登录成功"

    # 注册测试用户
    RESP=$(curl -s -X POST "${PANEL_URL}/api/v2/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${TEST_USER_EMAIL}\",\"password\":\"${TEST_USER_PASSWORD}\"}")
    USER_ID=$(echo "$RESP" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['user_id'])")
    ok "测试用户注册成功 (ID=${USER_ID})"

    # 创建套餐 (10GB)
    PLAN_ID=$(curl -s -X POST "${PANEL_URL}/api/v2/admin/plans" \
        -H "Content-Type: application/json" -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -d '{"name":"Local Test Plan","content":"Monthly 10GB","renew_price":29.9,"transfer_enable":10737418240,"device_limit":3,"speed_limit":0,"show":1,"sell_duration":30,"renew_method":"on","with_package":0,"sort":1}' \
        | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['id'])")
    ok "套餐创建成功 (ID=${PLAN_ID})"

    # 创建订阅分组
    GROUP_ID=$(curl -s -X POST "${PANEL_URL}/api/v2/admin/subscription/groups" \
        -H "Content-Type: application/json" -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -d '{"name":"Local Test Group","show":1}' \
        | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['id'])")
    ok "订阅分组创建成功 (ID=${GROUP_ID})"

    # 用户关联分组
    curl -sf -X POST "${PANEL_URL}/api/v2/admin/subscription/users/${USER_ID}/groups" \
        -H "Content-Type: application/json" -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -d "{\"group_id\":${GROUP_ID}}" > /dev/null
    ok "用户关联分组"

    # 套餐关联分组
    curl -sf -X POST "${PANEL_URL}/api/v2/admin/subscription/plans/${PLAN_ID}/groups" \
        -H "Content-Type: application/json" -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -d "{\"group_id\":${GROUP_ID}}" > /dev/null
    ok "套餐关联分组"

    # 节点协议关联分组
    NODE_ID=$(sqlite3 config/data/v2board.db "SELECT id FROM v2_node WHERE name='${V2BX_NODE_NAME}' ORDER BY id DESC LIMIT 1;")
    if [ -n "$NODE_ID" ]; then
        PROTOCOL_ID=$(curl -s "${PANEL_URL}/api/v2/admin/nodes/${NODE_ID}/protocols" \
            -H "Authorization: Bearer ${ADMIN_TOKEN}" \
            | python3 -c "import sys,json;print(json.load(sys.stdin)['data'][0]['id'])")

        curl -sf -X POST "${PANEL_URL}/api/v2/admin/subscription/groups/${GROUP_ID}/protocols" \
            -H "Content-Type: application/json" -H "Authorization: Bearer ${ADMIN_TOKEN}" \
            -d "{\"protocol_ids\":[${PROTOCOL_ID}]}" > /dev/null
        ok "节点协议关联分组 (NodeID=${NODE_ID}, ProtocolID=${PROTOCOL_ID})"
    fi

    # 修复数据库 (transfer_enable + group_id)
    python3 -c "
import sqlite3
conn = sqlite3.connect('config/data/v2board.db')
cur = conn.cursor()
cur.execute('UPDATE v2_user SET transfer_enable = 10737418240, group_id = ? WHERE id = ?', (${GROUP_ID}, ${USER_ID}))
cur.execute('UPDATE v2_node SET group_id = ? WHERE id = ?', (${GROUP_ID}, ${NODE_ID}))
conn.commit()
conn.close()
"
    ok "数据库修复: transfer_enable=10GB, group_id=${GROUP_ID}"
}

# ============================ Step 7: 重启 V2bX 并验证 ============================
step_v2bx_verify() {
    info "Step 7: 重启 V2bX 并验证"
    cd "$V2BX_DIR"

    # 停止旧进程
    if command -v pkill > /dev/null 2>&1; then
        pkill -f "V2bX.exe" || true
        sleep 2
    fi

    # 重启
    GOEXPERIMENT=jsonv2 ./build/V2bX.exe -c config.local.json > logs/v2bx.log 2>&1 &
    sleep 3

    for i in $(seq 1 15); do
        if grep -q "Added.*new users" logs/v2bx.log 2>/dev/null; then
            ok "V2bX 加载用户成功"
            grep "Added.*new users" logs/v2bx.log | tail -1
            return 0
        fi
        sleep 1
    done
    warn "V2bX 日志:"
    tail -5 logs/v2bx.log 2>/dev/null || true
}

# ============================ Step 8: 获取订阅 ============================
step_subscribe() {
    info "Step 8: 获取订阅"
    cd "$V2BOARD_DIR"

    USER_TOKEN=$(curl -s -X POST "${PANEL_URL}/api/v2/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${TEST_USER_EMAIL}\",\"password\":\"${TEST_USER_PASSWORD}\"}" \
        | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")

    SUB_TOKEN=$(curl -s "${PANEL_URL}/api/v2/user/profile" \
        -H "Authorization: Bearer ${USER_TOKEN}" \
        | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")

    VMESS_RAW=$(curl -s "${PANEL_URL}/s/${SUB_TOKEN}")
    UUID=$(echo "$VMESS_RAW" | python3 -c "
import sys, base64, json
raw = sys.stdin.read().strip()
decoded = base64.b64decode(raw).decode()
vmess = decoded.split('vmess://')[1]
obj = json.loads(base64.b64decode(vmess))
print(obj['id'])
")

    ok "UUID: ${UUID}"
    echo "$UUID" > .uuid
    echo "$SUB_TOKEN" > .sub_token
}

# ============================ Step 9: 生成 Speedtest 配置 ============================
step_speedtest() {
    info "Step 9: 生成 Speedtest 配置"

    UUID="${1:-$(cat "$V2BOARD_DIR/.uuid" 2>/dev/null || echo '')}"
    if [ -z "$UUID" ]; then
        warn "未找到 UUID，跳过 speedtest 配置生成"
        return 0
    fi

    cat > "${SPEEDTEST_DIR}/test_local.yaml" <<EOCFG
proxies:
  - alterId: 0
    cipher: auto
    name: ${V2BX_NODE_NAME}
    port: ${V2BX_PORT}
    server: ${PANEL_HOST}
    type: vmess
    uuid: ${UUID}
    network: tcp
    skip-cert-verify: true
EOCFG
    ok "Speedtest 配置已写入 ${SPEEDTEST_DIR}/test_local.yaml"
}

# ============================ Step 10: 运行 Speedtest ============================
step_speedtest_run() {
    info "Step 10: 运行 Speedtest"
    cd "$SPEEDTEST_DIR"

    if [ ! -f test_local.yaml ]; then
        warn "test_local.yaml 不存在，跳过"
        return 0
    fi

    if [ ! -f speedtest.exe ]; then
        err "speedtest.exe 不存在"
        return 0
    fi

    ./speedtest.exe -f test_local.yaml -no-ats -no-sign -format table 2>&1
    if [ -f output/report.txt ]; then
        echo ""
        ok "报告已保存到 output/report.txt"
        cat output/report.txt
    fi
}

# ============================ 主流程 ============================
main() {
    echo -e "${CYAN}=======================================${NC}"
    echo -e "${CYAN}  V2bX 本机节点完整工作流${NC}"
    echo -e "${CYAN}=======================================${NC}"
    echo ""

    case "${1:-all}" in
        panel)      step_panel ;;
        authkey)    step_authkey ;;
        build)      step_v2bx_build "${2:-}" ;;
        config)     step_v2bx_config "${2:-}" ;;
        start)      step_v2bx_start ;;
        datachain)  step_datachain ;;
        verify)     step_v2bx_verify ;;
        subscribe)  step_subscribe ;;
        speedtest)  step_speedtest "${2:-}" ;;
        speedtest-run) step_speedtest_run ;;
        all)
            step_panel
            step_authkey
            step_v2bx_build
            step_v2bx_config
            step_v2bx_start
            sleep 2
            step_datachain
            step_v2bx_verify
            step_subscribe
            step_speedtest
            step_speedtest_run
            echo ""
            echo -e "${GREEN}=======================================${NC}"
            echo -e "${GREEN}  V2bX 工作流完成${NC}"
            echo -e "${GREEN}=======================================${NC}"
            ;;
        *)
            echo "用法: $0 {all|panel|authkey|build|config|start|datachain|verify|subscribe|speedtest|speedtest-run}"
            echo ""
            echo "  all            执行完整工作流 (默认)"
            echo "  panel          仅启动面板"
            echo "  authkey        仅生成 AuthKey"
            echo "  build [rebuild] 仅构建 V2bX (加 rebuild 强制重新构建)"
            echo "  config         仅创建 V2bX 配置"
            echo "  start          仅启动 V2bX"
            echo "  datachain      仅配置数据链路"
            echo "  verify         仅重启 V2bX 并验证用户加载"
            echo "  subscribe      仅获取订阅"
            echo "  speedtest      仅生成 speedtest 配置"
            echo "  speedtest-run  仅运行 speedtest"
            exit 1
            ;;
    esac
}

main "$@"
