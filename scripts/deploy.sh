#!/bin/bash

# V2Board 部署脚本

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# 配置
APP_NAME="v2board"
BINARY_NAME="v2board"
DEPLOY_DIR="/opt/v2board"
DATA_DIR="/opt/v2board/data"
BACKUP_DIR="/opt/v2board/backups"

# 解析参数
ENVIRONMENT="production"
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")

while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--environment)
            ENVIRONMENT="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -h|--help)
            echo "用法: $0 [-e|--environment env] [-v|--version ver]"
            echo "  -e, --environment  环境名称 (development/staging/production)"
            echo "  -v, --version      版本号"
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            exit 1
            ;;
    esac
done

echo "=========================================="
echo "V2Board 部署脚本"
echo "=========================================="
echo "环境: ${ENVIRONMENT}"
echo "版本: ${VERSION}"
echo "=========================================="

# 1. 运行部署前测试
echo -e "${BLUE}[1/6] 运行部署前测试...${NC}"
if [ -f "./scripts/pre-deploy.sh" ]; then
    if ./scripts/pre-deploy.sh; then
        echo -e "${GREEN}✓ 部署前测试通过${NC}"
    else
        echo -e "${RED}✗ 部署前测试失败，终止部署${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠ 未找到部署前测试脚本，跳过${NC}"
fi

# 2. 构建应用
echo -e "${BLUE}[2/6] 构建应用...${NC}"
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o ${BINARY_NAME} ./cmd/server
echo -e "${GREEN}✓ 构建完成${NC}"

# 3. 创建必要目录
echo -e "${BLUE}[3/6] 创建部署目录...${NC}"
sudo mkdir -p ${DEPLOY_DIR}
sudo mkdir -p ${DATA_DIR}
sudo mkdir -p ${BACKUP_DIR}
echo -e "${GREEN}✓ 目录创建完成${NC}"

# 4. 备份旧版本
echo -e "${BLUE}[4/6] 备份旧版本...${NC}"
if [ -f "${DEPLOY_DIR}/${BINARY_NAME}" ]; then
    BACKUP_NAME="${BINARY_NAME}.backup.$(date +%Y%m%d_%H%M%S)"
    sudo cp "${DEPLOY_DIR}/${BINARY_NAME}" "${BACKUP_DIR}/${BACKUP_NAME}"
    echo -e "${GREEN}✓ 已备份到 ${BACKUP_DIR}/${BACKUP_NAME}${NC}"
else
    echo -e "${YELLOW}⚠ 未找到旧版本，跳过备份${NC}"
fi

# 5. 部署新版本
echo -e "${BLUE}[5/6] 部署新版本...${NC}"
sudo cp ${BINARY_NAME} ${DEPLOY_DIR}/
sudo chmod +x ${DEPLOY_DIR}/${BINARY_NAME}

# 复制配置文件（如果不存在）
if [ ! -f "${DEPLOY_DIR}/config/config.yaml" ]; then
    sudo mkdir -p ${DEPLOY_DIR}/config
    sudo cp config/config.yaml.example ${DEPLOY_DIR}/config/config.yaml
    echo -e "${GREEN}✓ 已创建默认配置文件${NC}"
fi

echo -e "${GREEN}✓ 部署完成${NC}"

# 6. 重启服务
echo -e "${BLUE}[6/6] 重启服务...${NC}"
if command -v systemctl &> /dev/null; then
    if systemctl is-active --quiet v2board; then
        sudo systemctl restart v2board
        echo -e "${GREEN}✓ 服务已重启${NC}"
    else
        echo -e "${YELLOW}⚠ v2board 服务未安装，请手动配置 systemd${NC}"
    fi
else
    echo -e "${YELLOW}⚠ systemctl 不可用，请手动重启服务${NC}"
fi

# 健康检查
echo ""
echo -e "${BLUE}执行健康检查...${NC}"
sleep 2
if curl -sf http://localhost:8080/health > /dev/null; then
    echo -e "${GREEN}✓ 应用运行正常${NC}"
else
    echo -e "${YELLOW}⚠ 健康检查失败，请检查应用状态${NC}"
fi

echo ""
echo "=========================================="
echo -e "${GREEN}✓ 部署成功!${NC}"
echo "二进制文件: ${DEPLOY_DIR}/${BINARY_NAME}"
echo "数据目录: ${DATA_DIR}"
echo "配置文件: ${DEPLOY_DIR}/config/config.yaml"
echo "=========================================="