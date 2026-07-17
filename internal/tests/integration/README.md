# Integration Test Framework

`internal/tests/integration` 提供一套可复用的集成测试框架，用于验证节点协议、配置生成、客户端连通性与端到端链路。

## 目录结构

```text
internal/tests/integration/
├── config/               # 配置结构与配置生成器（Xray/Mihomo）
├── clients/              # 测试客户端抽象与实现
├── runner/               # 测试执行器与报告汇总
├── binary/               # 测试依赖二进制管理（下载/探测）
├── mock/                 # 面板 API Mock Server
├── echo/                 # Echo 服务（HTTP/TCP）
├── local/                # 本地运行环境封装
├── e2e/                  # 端到端协议测试
├── reports/              # 测试报告输出目录（如启用）
├── integration_test.go
└── README.md
```

## 测试目标

- 验证配置生成逻辑（Xray JSON / Mihomo YAML）
- 验证客户端启动流程与端口可用性
- 验证代理链路可达性（经代理访问 Echo 服务）
- 验证多协议场景回归稳定性

## 前置依赖

### 必需

- Go 1.24+
- `xray` 可执行文件（E2E 测试必需）

### 可选

- `mihomo` 可执行文件（部分场景）

如果本机未安装相关二进制，可在 CLI 工具中开启 `-download` 自动拉取。

## 快速开始

### 1) 运行基础集成测试（推荐先跑）

```bash
go test -v -short ./internal/tests/integration/...
```

### 2) 运行端到端测试

```bash
go test -v ./internal/tests/integration/e2e/...
```

### 3) 运行单个场景

```bash
go test -v -run TestE2EShadowsocksFull ./internal/tests/integration/e2e/...
```

## CLI 工具

仓库提供 `cmd/integration-test`，用于快速执行指定协议/传输组合测试。

### 构建

```bash
go build -o integration-test ./cmd/integration-test
```

### 常见用法

```bash
# 最小必需参数
./integration-test -host example.com -uuid your-uuid

# 指定协议与 TLS 类型
./integration-test \
  -host example.com \
  -port 443 \
  -protocol vless \
  -transport tcp \
  -tls reality \
  -uuid your-uuid

# 自动下载二进制（xray/mihomo）
./integration-test -host example.com -uuid your-uuid -download

# 并发场景执行
./integration-test -host example.com -uuid your-uuid -parallel 4

# 输出 JSON 报告
./integration-test -host example.com -uuid your-uuid -output report.json
```

### 主要参数

- `-host`：服务端地址（必填）
- `-port`：服务端端口（默认 `443`）
- `-protocol`：`vmess | vless | trojan | shadowsocks | hysteria2 | tuic`
- `-transport`：`tcp | ws | grpc | h2 | quic`
- `-tls`：`none | tls | reality`
- `-uuid`：用户 UUID（必填）
- `-timeout`：单次测试超时（默认 `30s`）
- `-parallel`：并发执行数（`0` 表示串行）
- `-download`：自动下载依赖二进制
- `-output`：报告输出路径（JSON）

## 框架模块说明

### `config`

- 协议、传输、TLS 枚举与配置结构定义
- Xray/Mihomo 配置文件生成

### `clients`

- 客户端能力抽象
- 统一调用与生命周期管理

### `runner`

- 串行/并行执行测试场景
- 聚合结果并输出统计

### `binary`

- 检测本地二进制
- 可按需从上游下载测试依赖

### `e2e`

- 启动服务端与客户端
- 启动 Echo 服务作为目标
- 通过代理进行连通性验证并统计时延

## CI 建议

```bash
# 快速回归（PR 必跑）
go test -v -short ./internal/tests/integration/...

# 全量 E2E（可在 nightly 或特定分支跑）
go test -v ./internal/tests/integration/e2e/...
```

建议在 CI 中缓存 Go 模块与二进制依赖，减少构建时间。

## 故障排查

### `xray binary not found in PATH`

- 安装 `xray`，或在 CLI 使用 `-download`
- 确认 `xray` 可在当前 shell 直接执行

### 端口占用

- 检查本机端口冲突
- 重试或清理历史残留进程

### 连通性失败

- 检查服务器参数、协议、TLS 组合是否匹配
- 检查防火墙和出口策略
- 查看 E2E 日志中的具体错误信息

## 输出示例

```text
========== Test Report ==========
Time: 2026-03-13T12:00:00Z
Total: 4, Passed: 3, Failed: 1
Pass Rate: 75.0%
=================================
```
