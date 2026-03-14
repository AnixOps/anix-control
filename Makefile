.PHONY: build run clean test test-unit test-e2e test-coverage test-coverage-html test-integration test-frontend
.PHONY: pre-deploy deploy docker-build docker-run lint vet fmt bench grpc-gen swagger

# 版本信息
VERSION := 2.0.0
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(GIT_COMMIT)

# Go 测试参数
GO_TEST_FLAGS := -v -race -timeout 5m
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html

# 覆盖率阈值
MIN_COVERAGE := 80.0

# ==========================================
# 编译相关
# ==========================================

# 编译 (包含前端)
build: build-web build-server

# 编译前端
build-web:
	@echo "Building frontend..."
	cd web && npm install && npm run build
	cp -r web/dist/* public/

# 编译后端
build-server:
	@echo "Building server..."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o v2board ./cmd/server

# 编译 Linux 版本
build-linux:
	@echo "Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o v2board-linux ./cmd/server

# 运行开发服务器
run:
	go run ./cmd/server -config config/config.yaml

# 清理
clean:
	rm -f v2board v2board-linux $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -rf coverage/
	go clean -testcache

# ==========================================
# 测试相关
# ==========================================

# 运行所有测试
test: test-unit

# 运行单元测试
test-unit:
	@echo "Running unit tests..."
	go test $(GO_TEST_FLAGS) ./internal/...

# 运行 gRPC 测试
test-grpc:
	@echo "Running gRPC tests..."
	go test $(GO_TEST_FLAGS) ./internal/grpc/...

# 运行集成测试
test-integration:
	@echo "Running integration tests..."
	go test $(GO_TEST_FLAGS) -tags=integration ./tests/integration/...

# 运行 e2e 测试
test-e2e:
	@echo "Running e2e tests..."
	go test $(GO_TEST_FLAGS) ./tests/e2e/...

# 运行覆盖率测试
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -func=$(COVERAGE_FILE) | tail -1

# 检查覆盖率阈值
test-coverage-check: test-coverage
	@echo "Checking coverage threshold ($(MIN_COVERAGE)%)..."
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < $(MIN_COVERAGE)" | bc -l) -eq 1 ]; then \
		echo "Error: Coverage $$COVERAGE% is below minimum $(MIN_COVERAGE)%"; \
		exit 1; \
	fi; \
	echo "✓ Coverage $$COVERAGE% meets minimum $(MIN_COVERAGE)%"

# 生成 HTML 覆盖率报告
test-coverage-html: test-coverage
	@echo "Generating HTML coverage report..."
	mkdir -p coverage
	go tool cover -html=$(COVERAGE_FILE) -o coverage/$(COVERAGE_HTML)
	@echo "Coverage report generated: coverage/$(COVERAGE_HTML)"

# 前端测试
test-frontend:
	@echo "Running frontend tests..."
	cd web && npm run test

# 运行所有测试并检查覆盖率
test-all: test-coverage-check
	@echo "✓ All tests passed with sufficient coverage!"

# ==========================================
# 代码质量
# ==========================================

# 格式化代码
fmt:
	go fmt ./...

# 代码检查
lint:
	golangci-lint run

# 检查代码
vet:
	@echo "Running go vet..."
	go vet ./...

# 基准测试
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./internal/...

# ==========================================
# 部署相关
# ==========================================

# 部署前测试
pre-deploy:
	@echo "Running pre-deployment tests..."
	@./scripts/pre-deploy.sh

# 部署
deploy:
	@echo "Deploying..."
	@./scripts/deploy.sh

# ==========================================
# Docker 相关
# ==========================================

# 构建 Docker 镜像
docker-build:
	@echo "Building Docker image..."
	docker build -t v2board:latest .

# 运行 Docker 容器
docker-run:
	@echo "Running Docker container..."
	docker run -d --name v2board \
		-p 8080:8080 \
		-p 50051:50051 \
		-v $(PWD)/data:/app/data \
		v2board:latest

# 停止 Docker 容器
docker-stop:
	docker stop v2board || true
	docker rm v2board || true

# ==========================================
# gRPC 相关
# ==========================================

# 生成 gRPC 代码
grpc-gen:
	@echo "Generating gRPC code..."
	protoc --go_out=. --go-grpc_out=. api/grpc/v2board.proto

# 生成 Swagger 文档
swagger:
	@echo "Generating Swagger docs..."
	$(eval SWAG := $(shell go env GOPATH)/bin/swag)
	@command -v $(SWAG) >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	$(SWAG) init -g cmd/server/main.go -o docs --parseInternal

# 查看 Swagger 文档
swagger-serve:
	@echo "Open http://localhost:8080/swagger/index.html after starting the server"

# ==========================================
# 依赖管理
# ==========================================

# 安装依赖
deps:
	go mod tidy
	go mod download

# 安装测试依赖
install-deps:
	@echo "Installing test dependencies..."
	go install github.com/stretchr/testify@latest
	cd web && npm install

# ==========================================
# 帮助
# ==========================================

help:
	@echo "V2Board Makefile 帮助"
	@echo ""
	@echo "编译命令:"
	@echo "  make build          - 编译前后端"
	@echo "  make build-server   - 仅编译后端"
	@echo "  make build-web      - 仅编译前端"
	@echo "  make build-linux    - 编译 Linux 版本"
	@echo "  make run            - 运行开发服务器"
	@echo ""
	@echo "测试命令:"
	@echo "  make test           - 运行单元测试"
	@echo "  make test-grpc      - 运行 gRPC 测试"
	@echo "  make test-coverage  - 运行覆盖率测试"
	@echo "  make test-coverage-check - 检查覆盖率阈值"
	@echo "  make test-all       - 运行所有测试并检查覆盖率"
	@echo ""
	@echo "部署命令:"
	@echo "  make pre-deploy     - 部署前测试"
	@echo "  make deploy         - 部署应用"
	@echo ""
	@echo "Docker 命令:"
	@echo "  make docker-build   - 构建 Docker 镜像"
	@echo "  make docker-run     - 运行 Docker 容器"
	@echo "  make docker-stop    - 停止 Docker 容器"
	@echo ""
	@echo "其他命令:"
	@echo "  make fmt            - 格式化代码"
	@echo "  make lint           - 代码检查"
	@echo "  make vet            - go vet"
	@echo "  make swagger        - 生成 Swagger 文档"
	@echo "  make grpc-gen       - 生成 gRPC 代码"
	@echo "  make clean          - 清理编译产物"