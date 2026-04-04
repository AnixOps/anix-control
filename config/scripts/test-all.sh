#!/bin/bash

# 杩愯鎵€鏈夋祴璇?
# 鍖呮嫭鍗曞厓娴嬭瘯銆乪2e 娴嬭瘯鍜岄泦鎴愭祴璇?

set -e

echo "=========================================="
echo "V2Board 娴嬭瘯濂椾欢"
echo "=========================================="

# 棰滆壊瀹氫箟
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# 杩借釜澶辫触鐨勬祴璇?
FAILED=()

# 杩愯 Go 鍗曞厓娴嬭瘯
echo -e "${BLUE}杩愯 Go 鍗曞厓娴嬭瘯...${NC}"
if go test -v -race -timeout 5m ./internal/...; then
    echo -e "${GREEN}鉁?鍗曞厓娴嬭瘯閫氳繃${NC}"
else
    echo -e "${RED}鉁?鍗曞厓娴嬭瘯澶辫触${NC}"
    FAILED+=("unit-tests")
fi

# 杩愯 e2e 娴嬭瘯
echo -e "${BLUE}杩愯 E2E 娴嬭瘯...${NC}"
if go test -v -race -timeout 5m ./internal/tests/e2e/...; then
    echo -e "${GREEN}鉁?E2E 娴嬭瘯閫氳繃${NC}"
else
    echo -e "${RED}鉁?E2E 娴嬭瘯澶辫触${NC}"
    FAILED+=("e2e-tests")
fi

# 杩愯鍓嶇娴嬭瘯
if [ -d "web/node_modules" ]; then
    echo -e "${BLUE}杩愯鍓嶇娴嬭瘯...${NC}"
    cd web
    if npm run test; then
        echo -e "${GREEN}鉁?鍓嶇娴嬭瘯閫氳繃${NC}"
    else
        echo -e "${RED}鉁?鍓嶇娴嬭瘯澶辫触${NC}"
        FAILED+=("frontend-tests")
    fi
    cd ..
else
    echo -e "${YELLOW}鈿?璺宠繃鍓嶇娴嬭瘯 (璇峰厛杩愯 cd web && npm install)${NC}"
fi

# 鎶ュ憡缁撴灉
echo ""
echo "=========================================="
if [ ${#FAILED[@]} -eq 0 ]; then
    echo -e "${GREEN}鎵€鏈夋祴璇曢€氳繃!${NC}"
    exit 0
else
    echo -e "${RED}浠ヤ笅娴嬭瘯澶辫触:${NC}"
    for test in "${FAILED[@]}"; do
        echo -e "${RED}  - $test${NC}"
    done
    exit 1
fi