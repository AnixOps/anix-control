#!/bin/bash

# 生产环境部署前测试脚本
# 必须通过所有核心测试且核心包覆盖率 >= 80% 才能部署

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

MIN_COVERAGE=80.0
FAILED=0

# 核心测试包
CORE_PACKAGES="./internal/grpc/..."

# 检测操作系统
if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "win32" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
    IS_WINDOWS=1
else
    IS_WINDOWS=0
fi

echo "=========================================="
echo "V2Board 生产环境部署前测试"
echo "=========================================="
echo ""

# 1. 运行核心包测试
echo -e "${BLUE}[1/5] 运行核心包测试...${NC}"
if [ $IS_WINDOWS -eq 1 ]; then
    # Windows 不支持 -race
    if go test -v -timeout 10m ${CORE_PACKAGES}; then
        echo -e "${GREEN}✓ 核心测试通过${NC}"
    else
        echo -e "${RED}✗ 核心测试失败${NC}"
        FAILED=1
    fi
else
    if go test -v -race -timeout 10m ${CORE_PACKAGES}; then
        echo -e "${GREEN}✓ 核心测试通过${NC}"
    else
        echo -e "${RED}✗ 核心测试失败${NC}"
        FAILED=1
    fi
fi

# 2. 运行内部包测试（排除 e2e）
echo ""
echo -e "${BLUE}[2/5] 运行内部包测试...${NC}"
if [ $IS_WINDOWS -eq 1 ]; then
    if go test -v -timeout 10m ./internal/...; then
        echo -e "${GREEN}✓ 内部包测试通过${NC}"
    else
        echo -e "${YELLOW}⚠ 部分内部包测试失败，请检查${NC}"
    fi
else
    if go test -v -race -timeout 10m ./internal/...; then
        echo -e "${GREEN}✓ 内部包测试通过${NC}"
    else
        echo -e "${YELLOW}⚠ 部分内部包测试失败，请检查${NC}"
    fi
fi

# 3. 检查 gRPC 覆盖率
echo ""
echo -e "${BLUE}[3/5] 检查 gRPC 覆盖率 (${MIN_COVERAGE}%)...${NC}"
go test -coverprofile=grpc_coverage.out -covermode=atomic ./internal/grpc/...
GRPC_COVERAGE=$(go tool cover -func=grpc_coverage.out | grep total | awk '{print $3}' | sed 's/%//')
echo -e "gRPC 覆盖率: ${GRPC_COVERAGE}%"

# 比较覆盖率 (兼容 Windows/Linux)
check_coverage() {
    local coverage=$1
    local min=$2
    # 使用 awk 进行浮点数比较
    awk -v cov="$coverage" -v min="$min" 'BEGIN { exit (cov >= min) ? 0 : 1 }'
}

if check_coverage "$GRPC_COVERAGE" "$MIN_COVERAGE"; then
    echo -e "${GREEN}✓ gRPC 覆盖率达标: ${GRPC_COVERAGE}% >= ${MIN_COVERAGE}%${NC}"
else
    echo -e "${RED}✗ gRPC 覆盖率不足: ${GRPC_COVERAGE}% < ${MIN_COVERAGE}%${NC}"
    FAILED=1
fi

# 4. 验证核心功能
echo ""
echo -e "${BLUE}[4/5] 验证核心功能...${NC}"

# gRPC 核心测试
echo "  - 节点注册功能..."
if go test -v -run "TestNodeRegistrationFlow" ./internal/grpc/... > /dev/null 2>&1; then
    echo -e "    ${GREEN}✓ 节点注册功能正常${NC}"
else
    echo -e "    ${RED}✗ 节点注册功能异常${NC}"
    FAILED=1
fi

echo "  - 配置获取功能..."
if go test -v -run "TestGetConfig" ./internal/grpc/... > /dev/null 2>&1; then
    echo -e "    ${GREEN}✓ 配置获取功能正常${NC}"
else
    echo -e "    ${RED}✗ 配置获取功能异常${NC}"
    FAILED=1
fi

echo "  - 流量上报功能..."
if go test -v -run "TestReportTraffic" ./internal/grpc/... > /dev/null 2>&1; then
    echo -e "    ${GREEN}✓ 流量上报功能正常${NC}"
else
    echo -e "    ${RED}✗ 流量上报功能异常${NC}"
    FAILED=1
fi

echo "  - 用户管理功能..."
if go test -v -run "TestGetUsers" ./internal/grpc/... > /dev/null 2>&1; then
    echo -e "    ${GREEN}✓ 用户管理功能正常${NC}"
else
    echo -e "    ${RED}✗ 用户管理功能异常${NC}"
    FAILED=1
fi

echo "  - 健康检查功能..."
if go test -v -run "TestHealthCheck" ./internal/grpc/... > /dev/null 2>&1; then
    echo -e "    ${GREEN}✓ 健康检查功能正常${NC}"
else
    echo -e "    ${RED}✗ 健康检查功能异常${NC}"
    FAILED=1
fi

# 5. 构建检查
echo ""
echo -e "${BLUE}[5/5] 构建检查...${NC}"
if go build -o /dev/null ./cmd/server 2>/dev/null || go build -o NUL ./cmd/server 2>/dev/null; then
    echo -e "${GREEN}✓ 构建成功${NC}"
else
    echo -e "${RED}✗ 构建失败${NC}"
    FAILED=1
fi

# 生成 HTML 覆盖率报告
echo ""
mkdir -p coverage
go tool cover -html=grpc_coverage.out -o coverage/grpc-coverage.html
echo -e "${BLUE}gRPC 覆盖率报告: coverage/grpc-coverage.html${NC}"

# 结果汇总
echo ""
echo "=========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ 所有检查通过，可以部署到生产环境${NC}"
    echo ""
    echo "覆盖率摘要:"
    echo "  - gRPC 核心覆盖率: ${GRPC_COVERAGE}%"
    echo ""
    echo "下一步操作:"
    echo "  1. git add ."
    echo "  2. git commit -m 'feat: gRPC implementation with 80%+ coverage'"
    echo "  3. git push origin production"
    exit 0
else
    echo -e "${RED}✗ 部分检查未通过，请修复后再部署${NC}"
    echo ""
    echo "请运行以下命令查看详细错误:"
    echo "  go test -v ./internal/grpc/..."
    exit 1
fi