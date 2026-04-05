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
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ansible \
        bash \
        ca-certificates \
        curl \
        jq \
        openssh-client \
        sshpass \
        tzdata && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --uid 1000 --shell /bin/bash v2board

WORKDIR /app

# Copy binary and config
COPY --from=builder /app/v2board .
COPY --from=builder /app/docs ./docs
COPY --from=builder /app/config/config.yaml.example ./config/config.yaml.example
COPY --from=builder /app/config/deploy ./config/deploy

# Copy frontend build (if exists)
COPY --from=builder /app/web/public ./web/public

# Create necessary directories
RUN mkdir -p /app/config/data /app/web/public /app/logs /home/v2board/.ssh && \
    chown -R v2board:v2board /app /home/v2board && \
    chmod 700 /home/v2board/.ssh

# Switch to non-root user
USER v2board

# Set timezone
ENV HOME=/home/v2board
ENV TZ=Asia/Shanghai
ENV GIN_MODE=release
ENV ANSIBLE_CONFIG=/app/config/deploy/ansible/ansible.cfg

# Expose ports (HTTP + frontend + gRPC)
EXPOSE 8080 3000 50051

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:8080/health >/dev/null || exit 1

# Default command
CMD ["./v2board", "-config", "config/config.yaml"]
