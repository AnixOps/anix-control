.PHONY: build run clean test test-unit test-e2e test-coverage test-coverage-html test-integration test-frontend
.PHONY: pre-deploy deploy docker-build docker-run lint vet fmt bench grpc-gen swagger

# 鐗堟湰淇℃伅
VERSION := 2.0.0
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(GIT_COMMIT)

# Go 娴嬭瘯鍙傛暟
GO_TEST_FLAGS := -v -race -timeout 5m
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html

# 瑕嗙洊鐜囬槇鍊?
MIN_COVERAGE := 80.0

# ==========================================
# 缂栬瘧鐩稿叧
# ==========================================

# 缂栬瘧 (鍖呭惈鍓嶇)
build: build-web build-server

# 缂栬瘧鍓嶇
build-web:
	@echo "Building frontend..."
	cd web && npm install && npm run build

# 缂栬瘧鍚庣
build-server:
	@echo "Building server..."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o v2board ./cmd/server

# 缂栬瘧 Linux 鐗堟湰
build-linux:
	@echo "Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o v2board-linux ./cmd/server

# 杩愯寮€鍙戞湇鍔″櫒
run:
	go run ./cmd/server -config config/config.yaml

# 娓呯悊
clean:
	rm -f v2board v2board-linux $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -rf coverage/
	go clean -testcache

# ==========================================
# 娴嬭瘯鐩稿叧
# ==========================================

# 杩愯鎵€鏈夋祴璇?
test: test-unit

# 杩愯鍗曞厓娴嬭瘯
test-unit:
	@echo "Running unit tests..."
	go test $(GO_TEST_FLAGS) ./internal/...

# 杩愯 gRPC 娴嬭瘯
test-grpc:
	@echo "Running gRPC tests..."
	go test $(GO_TEST_FLAGS) ./internal/grpc/...

# 杩愯闆嗘垚娴嬭瘯
test-integration:
	@echo "Running integration tests..."
	go test $(GO_TEST_FLAGS) -tags=integration ./internal/tests/integration/...

# 杩愯 e2e 娴嬭瘯
test-e2e:
	@echo "Running e2e tests..."
	go test $(GO_TEST_FLAGS) ./internal/tests/e2e/...

# 杩愯瑕嗙洊鐜囨祴璇?
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -func=$(COVERAGE_FILE) | tail -1

# 妫€鏌ヨ鐩栫巼闃堝€?
test-coverage-check: test-coverage
	@echo "Checking coverage threshold ($(MIN_COVERAGE)%)..."
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < $(MIN_COVERAGE)" | bc -l) -eq 1 ]; then \
		echo "Error: Coverage $$COVERAGE% is below minimum $(MIN_COVERAGE)%"; \
		exit 1; \
	fi; \
	echo "鉁?Coverage $$COVERAGE% meets minimum $(MIN_COVERAGE)%"

# 鐢熸垚 HTML 瑕嗙洊鐜囨姤鍛?
test-coverage-html: test-coverage
	@echo "Generating HTML coverage report..."
	mkdir -p coverage
	go tool cover -html=$(COVERAGE_FILE) -o coverage/$(COVERAGE_HTML)
	@echo "Coverage report generated: coverage/$(COVERAGE_HTML)"

# 鍓嶇娴嬭瘯
test-frontend:
	@echo "Running frontend tests..."
	cd web && npm run test

# 杩愯鎵€鏈夋祴璇曞苟妫€鏌ヨ鐩栫巼
test-all: test-coverage-check
	@echo "鉁?All tests passed with sufficient coverage!"

# ==========================================
# 浠ｇ爜璐ㄩ噺
# ==========================================

# 鏍煎紡鍖栦唬鐮?
fmt:
	go fmt ./...

# 浠ｇ爜妫€鏌?
lint:
	golangci-lint run

# 妫€鏌ヤ唬鐮?
vet:
	@echo "Running go vet..."
	go vet ./...

# 鍩哄噯娴嬭瘯
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./internal/...

# ==========================================
# 閮ㄧ讲鐩稿叧
# ==========================================

# 閮ㄧ讲鍓嶆祴璇?
pre-deploy:
	@echo "Running pre-deployment tests..."
	@./config/scripts/pre-deploy.sh

# 閮ㄧ讲
deploy:
	@echo "Deploying..."
	@./config/scripts/deploy.sh

# ==========================================
# Docker 鐩稿叧
# ==========================================

# 鏋勫缓 Docker 闀滃儚
docker-build:
	@echo "Building Docker image..."
	docker build -t v2board:latest .

# 杩愯 Docker 瀹瑰櫒
docker-run:
	@echo "Running Docker container..."
	docker run -d --name v2board \
		-p 8080:8080 \
		-p 50051:50051 \
		-v $(PWD)/config/data:/app/config/data \
		-v $(PWD)/web/public:/app/web/public \
		v2board:latest

# 鍋滄 Docker 瀹瑰櫒
docker-stop:
	docker stop v2board || true
	docker rm v2board || true

# ==========================================
# gRPC 鐩稿叧
# ==========================================

# 鐢熸垚 gRPC 浠ｇ爜
grpc-gen:
	@echo "Generating gRPC code..."
	protoc --go_out=. --go-grpc_out=. api/grpc/v2board.proto

# 鐢熸垚 Swagger 鏂囨。
swagger:
	@echo "Generating Swagger docs..."
	$(eval SWAG := $(shell go env GOPATH)/bin/swag)
	@command -v $(SWAG) >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	$(SWAG) init -g cmd/server/main.go -o docs --parseInternal

# 鏌ョ湅 Swagger 鏂囨。
swagger-serve:
	@echo "Open http://localhost:8080/swagger/index.html after starting the server"

# ==========================================
# 渚濊禆绠＄悊
# ==========================================

# 瀹夎渚濊禆
deps:
	go mod tidy
	go mod download

# 瀹夎娴嬭瘯渚濊禆
install-deps:
	@echo "Installing test dependencies..."
	go install github.com/stretchr/testify@latest
	cd web && npm install

# ==========================================
# 甯姪
# ==========================================

help:
	@echo "V2Board Makefile 甯姪"
	@echo ""
	@echo "缂栬瘧鍛戒护:"
	@echo "  make build          - 缂栬瘧鍓嶅悗绔?
	@echo "  make build-server   - 浠呯紪璇戝悗绔?
	@echo "  make build-web      - 浠呯紪璇戝墠绔?
	@echo "  make build-linux    - 缂栬瘧 Linux 鐗堟湰"
	@echo "  make run            - 杩愯寮€鍙戞湇鍔″櫒"
	@echo ""
	@echo "娴嬭瘯鍛戒护:"
	@echo "  make test           - 杩愯鍗曞厓娴嬭瘯"
	@echo "  make test-grpc      - 杩愯 gRPC 娴嬭瘯"
	@echo "  make test-coverage  - 杩愯瑕嗙洊鐜囨祴璇?
	@echo "  make test-coverage-check - 妫€鏌ヨ鐩栫巼闃堝€?
	@echo "  make test-all       - 杩愯鎵€鏈夋祴璇曞苟妫€鏌ヨ鐩栫巼"
	@echo ""
	@echo "閮ㄧ讲鍛戒护:"
	@echo "  make pre-deploy     - 閮ㄧ讲鍓嶆祴璇?
	@echo "  make deploy         - 閮ㄧ讲搴旂敤"
	@echo ""
	@echo "Docker 鍛戒护:"
	@echo "  make docker-build   - 鏋勫缓 Docker 闀滃儚"
	@echo "  make docker-run     - 杩愯 Docker 瀹瑰櫒"
	@echo "  make docker-stop    - 鍋滄 Docker 瀹瑰櫒"
	@echo ""
	@echo "鍏朵粬鍛戒护:"
	@echo "  make fmt            - 鏍煎紡鍖栦唬鐮?
	@echo "  make lint           - 浠ｇ爜妫€鏌?
	@echo "  make vet            - go vet"
	@echo "  make swagger        - 鐢熸垚 Swagger 鏂囨。"
	@echo "  make grpc-gen       - 鐢熸垚 gRPC 浠ｇ爜"
	@echo "  make clean          - 娓呯悊缂栬瘧浜х墿"
