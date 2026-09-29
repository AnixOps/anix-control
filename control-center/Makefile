.PHONY: all build run test clean deps lint docker-build docker-run install help

# Variables
BINARY=anixops
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO?=go
LDFLAGS=-s -w -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(DATE)'

# Platforms
PLATFORMS=linux-amd64 linux-arm64 windows-amd64 windows-arm64 darwin-amd64 darwin-arm64

all: build

## ============================================================================
## Development
## ============================================================================

build:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY)$(BINARY_EXT) cmd/anixops/main.go

build-tui:
	$(GO) build -ldflags="$(LDFLAGS)" -o anixops-tui$(BINARY_EXT) cmd/anixops-tui/main.go

build-all: build build-tui

run:
	$(GO) run cmd/anixops/main.go

run-tui:
	$(GO) run cmd/anixops-tui/main.go

## ============================================================================
## Testing
## ============================================================================

test:
	$(GO) test -v ./...

test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-coverage-summary:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | grep total

test-critical:
	@echo "Running critical function tests..."
	$(GO) test -v -run "TestRegister|TestStartPlugin|TestStopPlugin" ./internal/core/plugin/...
	$(GO) test -v -run "TestValidateToken|TestCheckPassword|TestHasPermission" ./internal/security/auth/...
	$(GO) test -v -run "TestDefaultConfig|TestLoadFromString" ./internal/core/config/...

test-race:
	$(GO) test -race ./...

## ============================================================================
## Test Reports
## ============================================================================

test-report:
	@echo "Generating comprehensive test report..."
	@chmod +x scripts/test-report/run-all-tests.sh 2>/dev/null || true
	@bash scripts/test-report/run-all-tests.sh
	@echo "Report generated at reports/test-reports/latest/summary.html"

test-report-quick:
	@echo "Generating quick test report (skip Vue)..."
	@chmod +x scripts/test-report/run-all-tests.sh 2>/dev/null || true
	@bash scripts/test-report/run-all-tests.sh --skip-vue
	@echo "Report generated at reports/test-reports/latest/summary.html"

test-report-clean:
	rm -rf reports/test-reports/latest/*
	rm -rf reports/test-reports/historical/*

test-report-open: test-report
	@open reports/test-reports/latest/summary.html 2>/dev/null || \
	 xdg-open reports/test-reports/latest/summary.html 2>/dev/null || \
	 start reports/test-reports/latest/summary.html 2>/dev/null || \
	 echo "Open reports/test-reports/latest/summary.html manually"

## ============================================================================
## Quality
## ============================================================================

lint:
	golangci-lint run --timeout=5m

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

check: fmt vet lint test

## ============================================================================
## Build All Platforms
## ============================================================================

build-release: $(PLATFORMS)

$(PLATFORMS):
	@mkdir -p dist/$@
	@echo "Building for $@..."
	@$(eval GOOS=$(word 1,$(subst -, ,$@)))
	@$(eval GOARCH=$(word 2,$(subst -, ,$@)))
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags="$(LDFLAGS)" -o dist/$@/anixops$(if $(filter windows,$(GOOS)),.exe,) cmd/anixops/main.go
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -ldflags="$(LDFLAGS)" -o dist/$@/anixops-tui$(if $(filter windows,$(GOOS)),.exe,) cmd/anixops-tui/main.go

package-release: build-release
	@echo "Creating release archives..."
	@cd dist/linux-amd64 && tar -czvf ../anixops-$(VERSION)-linux-amd64.tar.gz *
	@cd dist/linux-arm64 && tar -czvf ../anixops-$(VERSION)-linux-arm64.tar.gz *
	@cd dist/windows-amd64 && zip -r ../anixops-$(VERSION)-windows-amd64.zip *
	@cd dist/windows-arm64 && zip -r ../anixops-$(VERSION)-windows-arm64.zip *
	@cd dist/darwin-amd64 && tar -czvf ../anixops-$(VERSION)-darwin-amd64.tar.gz *
	@cd dist/darwin-arm64 && tar -czvf ../anixops-$(VERSION)-darwin-arm64.tar.gz *
	@cd dist && sha256sum *.tar.gz *.zip > checksums.sha256

## ============================================================================
## Web GUI
## ============================================================================

web-deps:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

## ============================================================================
## Docker
## ============================================================================

docker-build:
	docker build -f deployments/docker/Dockerfile -t anixops:$(VERSION) .

docker-run:
	docker run -p 8080:8080 -p 50052:50052 anixops:$(VERSION)

docker-compose-up:
	docker-compose -f deployments/docker/docker-compose.yaml up -d

docker-compose-down:
	docker-compose -f deployments/docker/docker-compose.yaml down

## ============================================================================
## Installation
## ============================================================================

install: build
	cp $(BINARY)$(BINARY_EXT) /usr/local/bin/
	@echo "Installed $(BINARY) to /usr/local/bin/"

uninstall:
	rm -f /usr/local/bin/$(BINARY)$(BINARY_EXT)
	@echo "Uninstalled $(BINARY)"

## ============================================================================
## Release
## ============================================================================

release: clean test-coverage test-critical package-release
	@echo "Release $(VERSION) ready in dist/"

release-check:
	@echo "Checking release prerequisites..."
	@test -f .gitignore || (echo "Missing .gitignore" && exit 1)
	@test -f README.md || (echo "Missing README.md" && exit 1)
	@test -f LICENSE || (echo "Missing LICENSE" && exit 1)
	@echo "All checks passed!"

## ============================================================================
## Maintenance
## ============================================================================

clean:
	rm -f $(BINARY)$(BINARY_EXT) anixops-tui$(BINARY_EXT)
	rm -rf dist/
	rm -f coverage.out coverage.html

deps:
	$(GO) mod download
	$(GO) mod tidy

tidy:
	$(GO) mod tidy

## ============================================================================
## Version
## ============================================================================

version:
	@echo "AnixOps Control Center"
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
	@echo "  Date:    $(DATE)"

## ============================================================================
## Help
## ============================================================================

help:
	@echo "AnixOps Control Center - Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Development:"
	@echo "  build          Build CLI binary"
	@echo "  build-tui      Build TUI binary"
	@echo "  build-all      Build both CLI and TUI"
	@echo "  run            Run CLI"
	@echo "  run-tui        Run TUI"
	@echo ""
	@echo "Testing:"
	@echo "  test               Run tests"
	@echo "  test-coverage      Run tests with coverage report"
	@echo "  test-critical      Run critical function tests"
	@echo "  test-race          Run tests with race detection"
	@echo "  test-report        Generate comprehensive test report (all projects)"
	@echo "  test-report-quick  Generate quick test report (skip Vue)"
	@echo "  test-report-clean  Clean test report artifacts"
	@echo "  test-report-open   Generate and open test report"
	@echo ""
	@echo "Quality:"
	@echo "  lint           Run linter"
	@echo "  fmt            Format code"
	@echo "  vet            Run go vet"
	@echo "  check          Run all quality checks"
	@echo ""
	@echo "Build Platforms:"
	@echo "  build-release      Build for all platforms"
	@echo "  package-release    Create release archives"
	@echo "  release            Full release (test + build + package)"
	@echo ""
	@echo "Web GUI:"
	@echo "  web-deps       Install web dependencies"
	@echo "  web-dev        Start web dev server"
	@echo "  web-build      Build web for production"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-run     Run Docker container"
	@echo ""
	@echo "Other:"
	@echo "  clean          Clean build artifacts"
	@echo "  deps           Download dependencies"
	@echo "  version        Show version info"
	@echo "  help           Show this help"