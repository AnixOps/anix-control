#!/bin/bash

# 运行所有测试
# 包括单元测试、e2e 测试和集成测试

set -e

echo "=========================================="
echo "V2Board 测试套件"
echo "=========================================="

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# 追踪失败的测试
FAILED=()

# 运行 Go 单元测试
echo -e "${BLUE}运行 Go 单元测试...${NC}"
if go test -v -race -timeout 5m ./internal/...; then
    echo -e "${GREEN}✓ 单元测试通过${NC}"
else
    echo -e "${RED}✗ 单元测试失败${NC}"
    FAILED+=("unit-tests")
fi

# 运行 e2e 测试
echo -e "${BLUE}运行 E2E 测试...${NC}"
if go test -v -race -timeout 5m ./tests/e2e/...; then
    echo -e "${GREEN}✓ E2E 测试通过${NC}"
else
    echo -e "${RED}✗ E2E 测试失败${NC}"
    FAILED+=("e2e-tests")
fi

# 运行前端测试
if [ -d "web/node_modules" ]; then
    echo -e "${BLUE}运行前端测试...${NC}"
    cd web
    if npm run test; then
        echo -e "${GREEN}✓ 前端测试通过${NC}"
    else
        echo -e "${RED}✗ 前端测试失败${NC}"
        FAILED+=("frontend-tests")
    fi
    cd ..
else
    echo -e "${YELLOW}⚠ 跳过前端测试 (请先运行 cd web && npm install)${NC}"
fi

# 报告结果
echo ""
echo "=========================================="
if [ ${#FAILED[@]} -eq 0 ]; then
    echo -e "${GREEN}所有测试通过!${NC}"
    exit 0
else
    echo -e "${RED}以下测试失败:${NC}"
    for test in "${FAILED[@]}"; do
        echo -e "${RED}  - $test${NC}"
    done
    exit 1
fi