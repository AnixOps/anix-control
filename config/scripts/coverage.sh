#!/bin/bash

# 测试覆盖率报告生成脚�?
# 生成 Go 后端和前端测试覆盖率报告

set -e

echo "=========================================="
echo "V2Board 测试覆盖率报告生�?
echo "=========================================="

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 创建输出目录
mkdir -p coverage

# Go 后端测试
echo -e "${BLUE}运行 Go 后端测试...${NC}"
go test -v -race -coverprofile=coverage/go-coverage.out -covermode=atomic ./internal/... ./internal/tests/...

# 生成 HTML 报告
echo -e "${BLUE}生成 Go 覆盖�?HTML 报告...${NC}"
go tool cover -html=coverage/go-coverage.out -o coverage/go-coverage.html

# 显示覆盖率摘�?
echo -e "${GREEN}Go 测试覆盖率摘�?${NC}"
go tool cover -func=coverage/go-coverage.out | tail -1

# 前端测试 (如果已安装依�?
if [ -d "web/node_modules" ]; then
    echo -e "${BLUE}运行前端测试...${NC}"
    cd web
    npm run test:coverage
    cp -r coverage ../coverage/frontend-coverage
    cd ..
else
    echo -e "${BLUE}跳过前端测试 (请先运行 cd web && npm install)${NC}"
fi

echo -e "${GREEN}=========================================="
echo "覆盖率报告生成完�?"
echo "Go HTML 报告: coverage/go-coverage.html"
echo "前端覆盖率报�? coverage/frontend-coverage/"
echo "==========================================${NC}"