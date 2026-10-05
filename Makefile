.PHONY: build run dev-config clean test test-unit test-e2e test-coverage test-coverage-html test-frontend
.PHONY: pre-deploy deploy docker-build docker-run lint vet fmt bench grpc-gen swagger
.PHONY: test-quick test-clean test-summary test-grpc test-cmd test-all test-coverage-all

# 版本信息
VERSION := 4.2.0-rc.2
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(GIT_COMMIT)
RUN_CONFIG ?= config/config.dev.yaml

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

# 编译后端
build-server:
	@echo "Building server..."
	mkdir -p build
	GOWORK=off CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o build/anix-control ./cmd/server

# 编译 Linux 版本
build-linux:
	@echo "Building for Linux..."
	mkdir -p build
	GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/anix-control-linux ./cmd/server

# 运行开发服务器
run: dev-config
	GOWORK=off go run ./cmd/server -config $(RUN_CONFIG)

dev-config:
	@if [ "$(RUN_CONFIG)" = "config/config.dev.yaml" ] && [ ! -f "$(RUN_CONFIG)" ]; then \
		cp config/config.dev.yaml.example "$(RUN_CONFIG)"; \
		echo "Created $(RUN_CONFIG) from the isolated development template"; \
	fi
	@test -f "$(RUN_CONFIG)" || { echo "Run config not found: $(RUN_CONFIG)" >&2; exit 1; }

# 清理
clean:
	rm -f anix-control anix-control-linux build/anix-control build/anix-control-linux $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -rf coverage/
	go clean -testcache

# ==========================================
# 测试相关
# ==========================================

# 运行所有单元测试（默认）
test: test-unit

# 运行单元测试
test-unit:
	@echo "Running unit tests..."
	GOWORK=off go test $(GO_TEST_FLAGS) ./internal/...

# 运行 gRPC 测试
test-grpc:
	@echo "Running gRPC tests..."
	GOWORK=off go test $(GO_TEST_FLAGS) -coverprofile=grpc_coverage.out ./internal/grpc/...
	@echo "gRPC coverage:"; go tool cover -func=grpc_coverage.out | grep total

# 运行 E2E 测试
test-e2e:
	@echo "Running e2e tests..."
	GOWORK=off go test -v -count=1 ./internal/tests/e2e/...

# 运行 cmd 包测试
test-cmd:
	@echo "Running cmd tests..."
	GOWORK=off go test $(GO_TEST_FLAGS) ./cmd/...

# 快速测试：遇到失败立即停止
test-quick:
	@echo "Running tests (fail-fast)..."
	GOWORK=off go test -v -race -failfast -timeout 5m $(filter-out $@,$(MAKECMDGOALS))

# 运行全部测试（分阶段执行，避免数据库干扰）
test-all: test-unit test-grpc test-e2e
	@echo "All test suites passed!"

# 简洁模式：只显示结果不显示详细日志
test-summary:
	@echo "Running tests (summary only)..."
	GOWORK=off go test -count=1 ./internal/... 2>&1 | grep -E "^(ok|FAIL|---)"

# 运行覆盖率测试
test-coverage:
	@echo "Running tests with coverage..."
	GOWORK=off go test -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./internal/... ./cmd/...
	@go tool cover -func=$(COVERAGE_FILE) | tail -1

# 检查覆盖率阈值
test-coverage-check: test-coverage
	@echo "Checking coverage threshold ($(MIN_COVERAGE)%)..."
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < $(MIN_COVERAGE)" | bc -l) -eq 1 ]; then \
		echo "Error: Coverage $$COVERAGE% is below minimum $(MIN_COVERAGE)%"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets minimum $(MIN_COVERAGE)%"

# 生成 HTML 覆盖率报告
test-coverage-html: test-coverage
	@echo "Generating HTML coverage report..."
	mkdir -p coverage
	go tool cover -html=$(COVERAGE_FILE) -o coverage/$(COVERAGE_HTML)
	@echo "Coverage report generated: coverage/$(COVERAGE_HTML)"

# 清理测试缓存和临时文件
test-clean:
	@echo "Cleaning test cache..."
	go clean -testcache
	rm -f *_coverage.out $(COVERAGE_FILE) $(COVERAGE_HTML)
	find . -name "v2board_*_test.db" -delete 2>/dev/null || true
	find . -name "*.test" -delete 2>/dev/null || true
	@echo "Test cache cleaned."

# 前端测试
test-frontend:
	@echo "Running frontend tests..."
	cd web && npm run test

# 运行所有测试并检查覆盖率
test-coverage-all: test-coverage-check
	@echo "✅ All tests passed with sufficient coverage!"

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
	@./config/scripts/pre-deploy.sh

# 部署
deploy:
	@echo "Deploying..."
	@./config/scripts/deploy.sh

# ==========================================
# Docker 相关
# ==========================================

# 构建 Docker 镜像 (source target; release images are built only by CI)
docker-build:
	@echo "Building Docker image (source target)..."
	docker build --target source \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) \
		--build-arg COMMIT=$(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown) \
		-t anix-control:dev .

# 运行 Docker 容器 (needs a reachable PostgreSQL; override DOCKER_RUN_ENV)
DOCKER_RUN_ENV ?= -e ANIX_CONTROL_DATABASE_HOST=host.docker.internal \
	-e ANIX_CONTROL_DATABASE_PASSWORD=anix_control \
	-e ANIX_CONTROL_JWT_SECRET=dev-only-jwt-secret-change-me \
	-e ANIX_CONTROL_ENV=development
docker-run:
	@echo "Running Docker container..."
	docker run -d --name anix-control \
		--add-host host.docker.internal:host-gateway \
		--read-only --tmpfs /tmp:rw,exec,mode=1777 \
		-p 127.0.0.1:8080:8080 -p 127.0.0.1:3000:3000 \
		$(DOCKER_RUN_ENV) \
		anix-control:dev

# 停止 Docker 容器
docker-stop:
	docker stop anix-control || true
	docker rm anix-control || true

# ==========================================
# gRPC 相关
# ==========================================

# 生成 gRPC 代码
grpc-gen:
	@echo "Generating gRPC code..."
	bash api/grpc/gen.sh

# 生成 Swagger 文档
swagger:
	@echo "Generating Swagger docs..."
	$(eval SWAG := $(shell go env GOPATH)/bin/swag)
	@command -v $(SWAG) >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	$(SWAG) init -g cmd/server/main.go -o docs --parseInternal --exclude control-center,packages

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
	@echo "AnixOps Control Makefile 帮助"
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
