#!/usr/bin/env bash
# V2bX 本地开发环境健康检查
# 用法: ./scripts/check-health.sh
set -euo pipefail

V2BOARD_DIR="${V2BOARD_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

# Auto-detect sibling repos relative to v2board_AnixOps parent dir
V2BOARD_PARENT="$(cd "${V2BOARD_DIR}" && dirname "$(pwd)")"
V2BX_DIR="${V2BX_DIR:-${V2BOARD_PARENT}/V2bX_AnixOps}"
SPEEDTEST_DIR="${SPEEDTEST_DIR:-${V2BOARD_PARENT}/AnixOps-speedtest}"

# Export for Python heredocs
export V2BOARD_DIR
export PYTHONIOENCODING=utf-8

PANEL_PORT="${PANEL_PORT:-8080}"
PANEL_URL="http://127.0.0.1:${PANEL_PORT}"
V2BX_PORT="${V2BX_PORT:-9000}"

PASS=0
FAIL=0
WARN=0

pass() { PASS=$((PASS+1)); echo "  [PASS] $*"; }
fail() { FAIL=$((FAIL+1)); echo "  [FAIL] $*"; }
warn() { WARN=$((WARN+1)); echo "  [WARN] $*"; }

echo "========================================="
echo "  V2bX 本地环境健康检查"
echo "========================================="
echo ""

# 1. 面板健康检查
echo "面板 (v2board)"
if curl -sf "${PANEL_URL}/health" > /dev/null 2>&1; then
    RESP=$(curl -s "${PANEL_URL}/health")
    pass "面板运行 ${PANEL_URL} - ${RESP}"
else
    fail "面板未运行 ${PANEL_URL}"
fi
echo ""

# 2. V2bX 监听
echo "V2bX 节点"
if command -v netstat > /dev/null 2>&1; then
    if netstat -ano 2>/dev/null | grep -q ":${V2BX_PORT}.*LISTEN"; then
        pass "V2bX 监听 0.0.0.0:${V2BX_PORT}"
    else
        fail "V2bX 未监听 :${V2BX_PORT}"
    fi
fi

if [ -f "${V2BX_DIR}/logs/v2bx.log" ]; then
    if grep -q "Added.*new users" "${V2BX_DIR}/logs/v2bx.log" 2>/dev/null; then
        USERS=$(grep "Added.*new users" "${V2BX_DIR}/logs/v2bx.log" | tail -1)
        pass "用户加载: ${USERS}"
    elif grep -q "No users found" "${V2BX_DIR}/logs/v2bx.log" 2>/dev/null; then
        warn "V2bX 无用户 (数据链路未配置)"
    fi
else
    warn "无 V2bX 日志文件"
fi
echo ""

# 3. UniProxy API + 数据库状态
echo "数据库 / UniProxy API"
DB="${V2BOARD_DIR}/config/data/v2board.db"
if [ ! -f "$DB" ]; then
    fail "数据库文件不存在: $DB"
else
    export V2BOARD_DIR PANEL_URL PYTHONIOENCODING=utf-8
    python3 << 'PYEOF'
import sqlite3, os, urllib.request, json

v2board_dir = os.environ["V2BOARD_DIR"]
panel_url = os.environ.get("PANEL_URL", "http://127.0.0.1:8080")
db_path = os.path.join(v2board_dir, "config", "data", "v2board.db")

conn = sqlite3.connect(db_path)
cur = conn.cursor()

# Users
cur.execute('SELECT id, email, group_id, transfer_enable FROM v2_user WHERE email != ?', ('admin@example.invalid',))
users = cur.fetchall()
if users:
    for u in users:
        gb = u[3] / 1073741824 if u[3] > 0 else 0
        print(f"  [INFO] 用户 {u[1]} (id={u[0]}): group={u[2]}, quota={gb:.1f}GB")
else:
    print("  [WARN] 无测试用户")

# Nodes
cur.execute('SELECT id, name, host, port, group_id, api_key FROM v2_node ORDER BY id DESC')
nodes = cur.fetchall()
if nodes:
    node = nodes[0]
    node_id, name, host, port, group_id, api_key = node
    print(f"  [INFO] 节点 {name} (id={node_id}): {host}:{port}, group={group_id}")

    # Test UniProxy API
    if api_key:
        url = f"{panel_url}/api/v2/server/UniProxy/user?node_id={node_id}"
        req = urllib.request.Request(url, headers={"X-API-Key": api_key})
        resp = urllib.request.urlopen(req).read().decode()
        data = json.loads(resp)
        count = len(data.get("users", []))
        if count > 0:
            print(f"  [INFO] UniProxy API: {count} 个用户")
        else:
            print("  [WARN] UniProxy API: 无用户 (检查 group_id / transfer_enable)")
else:
    print("  [WARN] 无节点记录")

# Subscription groups
cur.execute('SELECT id, name FROM v2_subscription_group')
groups = cur.fetchall()
if groups:
    names = ", ".join(f"{g[0]}:{g[1]}" for g in groups)
    print(f"  [INFO] 订阅分组: {names}")

conn.close()
PYEOF
fi
echo ""

# 4. Speedtest
echo "Speedtest"
if [ -f "${SPEEDTEST_DIR}/test_local.yaml" ]; then
    pass "Speedtest 配置存在"
else
    warn "Speedtest 配置不存在 (运行 setup.sh 后自动生成)"
fi

if [ -f "${SPEEDTEST_DIR}/speedtest.exe" ]; then
    pass "Speedtest 二进制存在"
else
    fail "Speedtest 二进制缺失"
fi

if [ -f "${SPEEDTEST_DIR}/output/report.txt" ] && [ -s "${SPEEDTEST_DIR}/output/report.txt" ]; then
    while IFS= read -r line; do
        if echo "$line" | grep -q "vmess"; then
            pass "最近测速: ${line:0:100}"
            break
        fi
    done < <(tac "${SPEEDTEST_DIR}/output/report.txt")
fi
echo ""

# 5. 配置完整性
echo "配置完整性"
if [ -f "${V2BOARD_DIR}/config/config.yaml" ]; then
    BACKEND=$(python3 -c "
import os, yaml
d = yaml.safe_load(open(os.environ['V2BOARD_DIR'] + '/config/config.yaml'))
print(d['forward_runtime']['backend'])
")
    case "$BACKEND" in
        nftables_ansible) pass "forward_runtime: ${BACKEND}" ;;
        iptables_ansible) warn "forward_runtime: ${BACKEND} (建议切换到 nftables)" ;;
        *)                pass "forward_runtime: ${BACKEND}" ;;
    esac
fi

if [ -f "${V2BX_DIR}/config.local.json" ]; then
    pass "V2bX config.local.json 存在"
else
    fail "V2bX config.local.json 缺失"
fi

if [ -f "${V2BOARD_DIR}/config/deploy/ansible/inventory.ini" ]; then
    pass "Ansible inventory.ini 存在"
else
    warn "Ansible inventory.ini 缺失 (中继节点暂不需要)"
fi
echo ""

# Summary
echo "========================================="
echo "  总计: ${PASS} 通过, ${WARN} 警告, ${FAIL} 失败"
echo "========================================="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
