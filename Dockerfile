# Build stage
FROM golang:1.24-alpine AS builder

# Install build tools
RUN apk add --no-cache git make nodejs npm

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    $(go env GOPATH)/bin/swag init -g cmd/server/main.go -o docs --parseInternal

# Build frontend (if package.json exists)
RUN if [ -f "web/package.json" ]; then \
    cd web && npm ci && npm run build && cd ..; \
    fi

# Build binary with version info
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o v2board ./cmd/server

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN adduser -D -u 1000 v2board

WORKDIR /app

# Copy binary and config
COPY --from=builder /app/v2board .
COPY --from=builder /app/docs ./docs
COPY --from=builder /app/config/config.yaml.example ./config/config.yaml.example

# Copy frontend build (if exists)
COPY --from=builder /app/web/public ./web/public

# Create necessary directories
RUN mkdir -p /app/config/data /app/web/public /app/logs && chown -R v2board:v2board /app

# Switch to non-root user
USER v2board

# Set timezone
ENV TZ=Asia/Shanghai
ENV GIN_MODE=release

# Expose ports (HTTP + gRPC)
EXPOSE 8080 50051

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command
CMD ["./v2board", "-config", "config/config.yaml"]
