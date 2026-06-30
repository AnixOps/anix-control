#!/usr/bin/env bash
# V2Board AnixOps 面板可视化管理脚本
# 用法: ./scripts/manage.sh
#
# 支持的功能: 启动、停止、重启、状态查看、日志查看、构建、健康检查等
set -uo pipefail

# ============================ 配置 ============================
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
V2BOARD_DIR="${V2BOARD_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

# 从配置文件读取端口 (fallback 到默认值)
PANEL_PORT="${PANEL_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
PANEL_URL="http://127.0.0.1:${PANEL_PORT}"

BINARY_NAME="v2board"
if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]] || [[ "$OSTYPE" == "win32" ]]; then
    BINARY_NAME="v2board.exe"
fi

PID_FILE="${V2BOARD_DIR}/.v2board.pid"
LOG_FILE="${V2BOARD_DIR}/logs/panel.log"
BUILD_DIR="${V2BOARD_DIR}/build"
BINARY_PATH="${BUILD_DIR}/${BINARY_NAME}"
CONFIG_PATH="${V2BOARD_DIR}/config/config.yaml"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Windows Git Bash 可能缺少 Go 路径，自动补全
if [[ ("$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32") ]] && ! command -v go > /dev/null 2>&1; then
    for gopath in \
        "/c/Program Files/Go/bin" \
        "/c/Program Files (x86)/Go/bin" \
        "$HOME/go/bin" \
        "/usr/local/go/bin"; do
        if [[ -x "${gopath}/go" ]] || [[ -x "${gopath}/go.exe" ]]; then
            export PATH="${gopath}:${PATH}"
            break
        fi
    done
fi

# ============================ 工具函数 ============================
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERR]${NC}   $*"; }

# 检查 whiptail/dialog 可用性
if command -v whiptail > /dev/null 2>&1; then
    DIALOG_CMD="whiptail"
    DIALOG_TITLE="--title"
elif command -v dialog > /dev/null 2>&1; then
    DIALOG_CMD="dialog"
    DIALOG_TITLE="--title"
else
    DIALOG_CMD="menu"
fi

show_dialog() {
    local title="$1"
    local text="$2"
    if [[ "$DIALOG_CMD" != "menu" ]]; then
        $DIALOG_CMD --msgbox "$text" 12 60 $DIALOG_TITLE "$title" 2>&1
    else
        echo "=== $title ==="
        echo "$text"
        echo ""
        read -p "按回车键继续..."
    fi
}

show_menu() {
    local title="$1"
    shift
    local items=("$@")
    if [[ "$DIALOG_CMD" == "whiptail" ]]; then
        local args=()
        for item in "${items[@]}"; do
            args+=("$item" "")
        done
        whiptail --menu "$title" 20 60 12 "${args[@]}" 3>&1 1>&2 2>&3
    elif [[ "$DIALOG_CMD" == "dialog" ]]; then
        local args=()
        for item in "${items[@]}"; do
            args+=("$item" "")
        done
        dialog --menu "$title" 20 60 12 "${args[@]}" 3>&1 1>&2 2>&3
    else
        echo "=== $title ==="
        for i in "${!items[@]}"; do
            echo "  $((i+1)). ${items[$i]}"
        done
        echo ""
        read -p "请选择 [1-${#items[@]}]: " choice
        echo "${items[$((choice-1))]}"
    fi
}

# ============================ 状态检测 ============================
get_pid() {
    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid=$(cat "$PID_FILE" 2>/dev/null)
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            echo "$pid"
            return 0
        fi
    fi
    # fallback: 通过进程名查找
    if command -v pgrep > /dev/null 2>&1; then
        local pid
        pid=$(pgrep -f "${BINARY_NAME}.*-config" 2>/dev/null | head -1)
        if [[ -n "$pid" ]]; then
            echo "$pid"
            return 0
        fi
    fi
    echo ""
    return 1
}

is_running() {
    local pid
    pid=$(get_pid) && [[ -n "$pid" ]]
}

health_check() {
    if curl -sf "${PANEL_URL}/health" > /dev/null 2>&1; then
        curl -sf "${PANEL_URL}/health" 2>/dev/null
        return 0
    fi
    return 1
}

get_status() {
    local pid
    local running=false
    local healthy=false
    local health_resp=""

    pid=$(get_pid 2>/dev/null) || true
    if [[ -n "$pid" ]]; then
        running=true
    fi

    if $running; then
        health_resp=$(health_check) && healthy=true
    fi

    echo "PID: ${pid:-N/A}"
    echo "状态: $($running && echo '运行中' || echo '已停止')"
    echo "健康检查: $($healthy && echo "通过 (${health_resp})" || echo '未响应')"
    echo "端口: ${PANEL_PORT}"
    echo "日志: ${LOG_FILE}"

    if $running && [[ -f "$LOG_FILE" ]]; then
        echo "日志大小: $(wc -c < "$LOG_FILE" 2>/dev/null || echo 'N/A') bytes"
        echo "最后更新: $(stat -c %y "$LOG_FILE" 2>/dev/null || stat -f %Sm "$LOG_FILE" 2>/dev/null || echo 'N/A')"
    fi
}

# ============================ 操作函数 ============================

# 构建面板
action_build() {
    cd "$V2BOARD_DIR" || return 1

    mkdir -p "$BUILD_DIR" "$(dirname "$LOG_FILE")"

    info "构建 V2Board 面板..."
    local output
    if output=$(go build -o "$BINARY_PATH" -trimpath ./cmd/server 2>&1); then
        ok "构建成功: ${BINARY_PATH}"
        return 0
    else
        err "构建失败"
        echo "$output"
        return 1
    fi
}

# 启动面板
action_start() {
    cd "$V2BOARD_DIR" || return 1

    if is_running; then
        local pid
        pid=$(get_pid)
        warn "面板已在运行 (PID: ${pid})"
        return 0
    fi

    # 检查二进制是否存在
    if [[ ! -f "$BINARY_PATH" ]]; then
        info "二进制文件不存在，先构建..."
        action_build || return 1
    fi

    # 检查配置文件
    if [[ ! -f "$CONFIG_PATH" ]]; then
        err "配置文件不存在: ${CONFIG_PATH}"
        return 1
    fi

    mkdir -p "$(dirname "$LOG_FILE")"

    info "启动面板 (端口 ${PANEL_PORT})..."
    nohup "$BINARY_PATH" -config "$CONFIG_PATH" >> "$LOG_FILE" 2>&1 &
    local pid=$!
    echo "$pid" > "$PID_FILE"

    # 等待启动
    for i in $(seq 1 15); do
        if health_check > /dev/null 2>&1; then
            ok "面板已就绪 (PID: ${pid})"
            return 0
        fi
        sleep 1
    done

    warn "面板已启动但健康检查未通过，查看日志: tail -f ${LOG_FILE}"
    return 0
}

# 停止面板
action_stop() {
    if ! is_running; then
        warn "面板未运行"
        rm -f "$PID_FILE"
        return 0
    fi

    local pid
    pid=$(get_pid)
    info "停止面板 (PID: ${pid})..."

    kill "$pid" 2>/dev/null || true

    # 等待进程退出
    for i in $(seq 1 10); do
        if ! kill -0 "$pid" 2>/dev/null; then
            ok "面板已停止"
            rm -f "$PID_FILE"
            return 0
        fi
        sleep 1
    done

    # 强制终止
    warn "面板未响应 SIGTERM，发送 SIGKILL..."
    kill -9 "$pid" 2>/dev/null || true
    sleep 1
    ok "面板已强制停止"
    rm -f "$PID_FILE"
}

# 重启面板
action_restart() {
    action_stop
    sleep 1
    action_start
}

# 查看状态
action_status() {
    local status_output
    status_output=$(get_status)
    show_dialog "面板状态" "$status_output"
}

# 查看日志
action_logs() {
    if [[ ! -f "$LOG_FILE" ]]; then
        warn "日志文件不存在: ${LOG_FILE}"
        return 1
    fi

    local lines="${1:-50}"
    if [[ "$DIALOG_CMD" != "menu" ]]; then
        tail -n "$lines" "$LOG_FILE" | $DIALOG_CMD --textbox /dev/stdin 20 80 $DIALOG_TITLE "面板日志 (最后 ${lines} 行)" 2>&1
    else
        echo "=== 面板日志 (最后 ${lines} 行) ==="
        tail -n "$lines" "$LOG_FILE"
        echo ""
        read -p "按回车键继续..."
    fi
}

# 健康检查
action_health() {
    local result
    if result=$(health_check 2>&1); then
        show_dialog "健康检查" "状态: 正常
响应: ${result}
URL: ${PANEL_URL}/health"
    else
        show_dialog "健康检查" "状态: 异常
面板未响应
URL: ${PANEL_URL}/health"
    fi
}

# 数据库备份
action_backup() {
    cd "$V2BOARD_DIR" || return 1

    local db_dir="${V2BOARD_DIR}/config/data"
    local db_file="${db_dir}/v2board.db"

    if [[ ! -f "$db_file" ]]; then
        warn "数据库文件不存在: ${db_file}"
        return 1
    fi

    mkdir -p "$db_dir"
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${db_dir}/v2board.db.bak_${timestamp}"

    info "备份数据库..."
    cp "$db_file" "$backup_file"
    ok "备份完成: ${backup_file}"

    # 保留最近 10 个备份
    local backup_count
    backup_count=$(ls -1 "${db_dir}"/v2board.db.bak_* 2>/dev/null | wc -l)
    if [[ "$backup_count" -gt 10 ]]; then
        ls -1t "${db_dir}"/v2board.db.bak_* | tail -n +11 | xargs rm -f
        info "清理旧备份，保留最近 10 个"
    fi
}

# 清理日志
action_clean_logs() {
    if [[ -f "$LOG_FILE" ]]; then
        local size
        size=$(wc -c < "$LOG_FILE")
        info "清空日志文件 (${size} bytes)..."
        > "$LOG_FILE"
        ok "日志已清空"
    else
        warn "日志文件不存在"
    fi
}

# 查看配置
action_config() {
    if [[ ! -f "$CONFIG_PATH" ]]; then
        warn "配置文件不存在: ${CONFIG_PATH}"
        return 1
    fi

    if [[ "$DIALOG_CMD" != "menu" ]]; then
        cat "$CONFIG_PATH" | $DIALOG_CMD --textbox /dev/stdin 30 80 $DIALOG_TITLE "配置文件: ${CONFIG_PATH}" 2>&1
    else
        echo "=== 配置文件: ${CONFIG_PATH} ==="
        cat "$CONFIG_PATH"
        echo ""
        read -p "按回车键继续..."
    fi
}

# 版本信息
action_version() {
    if [[ -f "$BINARY_PATH" ]]; then
        local version_output
        version_output=$("$BINARY_PATH" -h 2>&1 | head -5 || echo "无法获取版本信息")
        show_dialog "版本信息" "二进制: ${BINARY_PATH}
${version_output}"
    else
        show_dialog "版本信息" "二进制文件不存在，请先构建"
    fi
}

# 主菜单
main_menu() {
    while true; do
        local status_line=""
        if is_running; then
            local pid
            pid=$(get_pid 2>/dev/null) || true
            status_line="${GREEN}运行中${NC} (PID: ${pid:-?})"
        else
            status_line="${RED}已停止${NC}"
        fi

        local title="V2Board AnixOps 面板管理
状态: ${status_line}  端口: ${PANEL_PORT}"

        local choice
        choice=$(show_menu "$title" \
            "1 启动面板" \
            "2 停止面板" \
            "3 重启面板" \
            "4 查看状态" \
            "5 查看日志" \
            "6 构建二进制" \
            "7 健康检查" \
            "8 数据库备份" \
            "9 查看配置" \
            "10 清理日志" \
            "11 版本信息" \
            "0 退出")

        case "$choice" in
            *启动*) action_start ;;
            *停止*) action_stop ;;
            *重启*) action_restart ;;
            *状态*) action_status ;;
            *日志*) action_logs ;;
            *构建*) action_build ;;
            *健康*) action_health ;;
            *备份*) action_backup ;;
            *配置*) action_config ;;
            *清理*) action_clean_logs ;;
            *版本*) action_version ;;
            *退出*|0)
                info "退出管理脚本"
                exit 0
                ;;
            "")
                # 用户取消
                continue
                ;;
            *)
                warn "无效选择: ${choice}"
                ;;
        esac

        echo ""
        read -p "按回车键返回主菜单..."
    done
}

# ============================ 快捷命令 ============================
case "${1:-menu}" in
    start)      action_start ;;
    stop)       action_stop ;;
    restart)    action_restart ;;
    status)     get_status ;;
    logs)       action_logs "${2:-50}" ;;
    build)      action_build ;;
    health)     action_health ;;
    backup)     action_backup ;;
    config)     action_config ;;
    clean)      action_clean_logs ;;
    version)    action_version ;;
    menu|"")    main_menu ;;
    *)
        echo "用法: $0 {menu|start|stop|restart|status|logs|build|health|backup|config|clean|version}"
        echo ""
        echo "  menu     交互式菜单 (默认)"
        echo "  start    启动面板"
        echo "  stop     停止面板"
        echo "  restart  重启面板"
        echo "  status   查看状态"
        echo "  logs [N] 查看日志 (默认 50 行)"
        echo "  build    构建二进制"
        echo "  health   健康检查"
        echo "  backup   数据库备份"
        echo "  config   查看配置"
        echo "  clean    清理日志"
        echo "  version  版本信息"
        exit 1
        ;;
esac
