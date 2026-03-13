# Build stage
FROM golang:1.24-alpine AS builder

# 安装必要的构建工具
RUN apk add --no-cache git make

WORKDIR /app

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o v2board ./cmd/server

# 运行镜像
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 创建非 root 用户
RUN adduser -D -u 1000 v2board

# 复制编译产物
COPY --from=builder /app/v2board .
COPY --from=builder /app/config/config.yaml.example ./config/config.yaml.example

# 创建必要目录
RUN mkdir -p /app/data /app/public && chown -R v2board:v2board /app

# 切换到非 root 用户
USER v2board

# 设置时区
ENV TZ=Asia/Shanghai

# 暴露端口 (HTTP + gRPC)
EXPOSE 8080 50051

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./v2board", "-config", "config/config.yaml.example"]