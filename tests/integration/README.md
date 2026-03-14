# Integration Test Framework

完整的自动化测试框架，支持完全本地化的端到端代理测试。

## 目录结构

```
tests/integration/
├── config/              # 配置生成模块
│   ├── types.go         # 类型定义
│   ├── generator.go     # 生成器接口
│   ├── xray.go          # Xray JSON 配置
│   ├── mihomo.go        # Mihomo YAML 配置
│   └── generator_test.go
├── clients/             # 客户端管理
│   ├── client.go        # 客户端接口
│   ├── xray_mihomo.go   # Xray/Mihomo 实现
│   ├── manager.go       # 客户端管理器
│   └── client_test.go
├── runner/              # 测试执行器
│   ├── runner.go        # 测试运行器
│   └── runner_test.go
├── binary/              # 二进制自动下载
│   ├── manager.go       # GitHub Releases 下载
│   └── manager_test.go
├── mock/                # Mock 面板服务器
│   ├── server.go        # 模拟面板 API
│   └── server_test.go
├── echo/                # 内置 Echo 服务器
│   ├── server.go        # HTTP/TCP Echo
│   └── server_test.go
├── local/               # 本地虚拟环境
│   ├── environment.go   # 本地测试环境
│   └── environment_test.go
├── e2e/                 # 端到端完整测试 ✨
│   └── e2e_test.go      # 全流程测试
├── integration_test.go
└── README.md
```

## 端到端全流程测试

### 架构图

```
┌──────────────────────────────────────────────────────────────────────┐
│                     完整 E2E 测试流程                                 │
│                                                                      │
│  ┌────────────┐    ┌──────────────┐    ┌──────────────┐            │
│  │ 测试程序   │───→│ 客户端(Xray) │───→│ 服务端(Xray) │            │
│  │            │    │ SOCKS5 代理  │    │ 协议服务器   │            │
│  │ 发送请求   │    │ :随机端口    │    │ :随机端口    │            │
│  └────────────┘    └──────────────┘    └──────┬───────┘            │
│                                               ↓                      │
│                                        ┌──────────────┐              │
│                                        │ Echo 服务器  │              │
│                                        │ HTTP 回显    │              │
│                                        │ :随机端口    │              │
│                                        └──────────────┘              │
│                                                                      │
│  ✅ 完全本地化 - 不需要互联网                                        │
│  ✅ 自动端口分配 - 避免冲突                                          │
│  ✅ 自动进程管理 - 启动/停止/清理                                    │
│  ✅ 多协议支持 - Shadowsocks/VMess/VLESS/Trojan                     │
│  ✅ 并发测试 - 验证多请求场景                                        │
│  ✅ 大数据测试 - 验证数据传输                                        │
└──────────────────────────────────────────────────────────────────────┘
```

### 测试内容

| 测试名称 | 说明 |
|---------|------|
| `TestE2EShadowsocksFull` | Shadowsocks 完整流程 |
| `TestE2EVMessFull` | VMess 完整流程 |
| `TestE2EVLESSFull` | VLESS 完整流程 |
| `TestE2ETrojanFull` | Trojan 完整流程 |
| `TestE2EAllProtocols` | 所有协议批量测试 |
| `TestE2EConcurrency` | 并发请求测试 |
| `TestE2ELargeData` | 大数据传输测试 |

### 使用方式

```go
func TestMyE2E(t *testing.T) {
    // 创建测试套件
    suite := e2e.NewE2ETestSuite(t)
    if err := suite.Setup(); err != nil {
        t.Skipf("Setup failed: %v", err)
    }
    defer suite.Teardown()

    ctx := context.Background()

    // 启动 Echo 服务器
    echoPort, _ := suite.StartEchoServer(ctx)

    // 获取代理端口
    proxyPort, _ := suite.GetFreePort()

    // 运行完整测试
    result, err := suite.RunFullTest(ctx, proxyPort, echoPort, "shadowsocks")
    if err != nil {
        t.Fatal(err)
    }

    // 检查结果
    if !result.Success {
        t.Errorf("Test failed: %s", result.Error)
    }
    t.Logf("Latency: %v", result.Latency)
}
```

### 测试报告

```
========== E2E Test Report ==========
Time: 2026-03-13T12:00:00Z
Total: 4, Passed: 3, Failed: 1
Pass Rate: 75.0%

Details:
  ✅ PASS [shadowsocks] Server:54321 → Proxy:54322 → Echo:54320 (latency: 10ms)
  ✅ PASS [vmess] Server:54323 → Proxy:54324 → Echo:54320 (latency: 15ms)
  ✅ PASS [vless] Server:54325 → Proxy:54326 → Echo:54320 (latency: 12ms)
  ❌ FAIL [trojan] Server:54327 → Proxy:54328 → Echo:54320 (latency: 0s)
       Error: connection timeout
======================================
```

## 运行测试

```bash
# 运行所有单元测试（跳过需要二进制的测试）
go test -v -short ./tests/integration/...

# 运行完整 E2E 测试（需要安装 xray）
go test -v ./tests/integration/e2e/...

# 运行特定协议测试
go test -v -run TestE2EShadowsocksFull ./tests/integration/e2e/...

# 运行并发测试
go test -v -run TestE2EConcurrency ./tests/integration/e2e/...
```

## 前置要求

### 安装 Xray

```bash
# Windows (使用 Scoop)
scoop install xray

# macOS
brew install xray

# Linux
curl -L https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip -o xray.zip
unzip xray.zip
sudo mv xray /usr/local/bin/
```

### 安装 Mihomo（可选）

```bash
# Windows
scoop install mihomo

# macOS
brew install mihomo
```

## 测试覆盖

| 模块 | 测试数 | 覆盖内容 |
|------|--------|---------|
| config | 6 | 配置生成、协议支持 |
| clients | 8 | 客户端创建、管理 |
| runner | 7 | 测试执行、报告 |
| binary | 5 | 二进制下载、缓存 |
| mock | 6 | 面板 API 模拟 |
| echo | 7 | Echo 服务器 |
| local | 8 | 本地环境 |
| e2e | 8 | 端到端测试 |
| **总计** | **55+** | - |

## CI/CD 集成

```yaml
# .github/workflows/integration-test.yml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Install Xray
        run: |
          curl -L https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip -o xray.zip
          unzip xray.zip
          sudo mv xray /usr/local/bin/

      - name: Run unit tests
        run: go test -v -short ./tests/integration/...

      - name: Run E2E tests
        run: go test -v ./tests/integration/e2e/...
```

## 支持的协议

| 协议 | TLS | Reality | WebSocket | gRPC | QUIC |
|------|-----|---------|-----------|------|------|
| VMess | ✅ | ✅ | ✅ | ✅ | ❌ |
| VLESS | ✅ | ✅ | ✅ | ✅ | ❌ |
| Trojan | ✅ | ❌ | ✅ | ✅ | ❌ |
| Shadowsocks | ❌ | ❌ | ❌ | ❌ | ❌ |
| Hysteria2 | ✅ | ❌ | ❌ | ❌ | ✅ |
| TUIC | ✅ | ❌ | ❌ | ❌ | ✅ |

## 使用方法

### CLI 命令

```bash
# 构建
go build -o integration-test ./cmd/integration-test/

# 基本用法（自动下载二进制）
./integration-test -host example.com -uuid your-uuid -download

# 完整参数
./integration-test \
  -host example.com \
  -port 443 \
  -protocol vless \
  -transport tcp \
  -tls reality \
  -uuid your-uuid \
  -email test@example.com \
  -sni www.google.com \
  -public-key your-public-key \
  -short-id your-short-id \
  -download \
  -timeout 30s \
  -output report.json

# 并行测试
./integration-test -host example.com -uuid your-uuid -parallel 4 -download

# 只测试 Xray
./integration-test -host example.com -uuid your-uuid -client xray -download
```

### 自动下载二进制

添加 `-download` 标志后，框架会自动：

1. 检查系统 PATH 是否有 xray/mihomo
2. 检查缓存目录 `~/.cache/v2board-test/`
3. 从 GitHub Releases 自动下载最新版本
   - Xray: `XTLS/Xray-core`
   - Mihomo: `MetaCubeX/mihomo`

### 作为库使用

```go
package main

import (
    "context"
    "time"

    "github.com/anixops/v2board/tests/integration/binary"
    "github.com/anixops/v2board/tests/integration/clients"
    "github.com/anixops/v2board/tests/integration/config"
    "github.com/anixops/v2board/tests/integration/runner"
)

func main() {
    // 初始化二进制管理器
    binMgr := binary.NewManager("")
    clients.SetBinaryManager(&binaryAdapter{mgr: binMgr})

    // 创建服务端配置
    server := config.ServerConfig{
        Host:      "example.com",
        Port:      443,
        Protocol:  config.ProtocolVLESS,
        Transport: config.TransportTCP,
        TLSType:   config.TLSReality,
        SNI:       "www.google.com",
        PublicKey: "your-public-key",
        ShortID:   "your-short-id",
    }

    // 创建用户配置
    user := config.UserConfig{
        UUID:  "your-uuid",
        Email: "test@example.com",
    }

    // 创建运行器
    r := runner.NewRunner(
        runner.WithTimeout(30*time.Second),
        runner.WithParallel(4),
    )

    // 运行测试
    report := r.Run(context.Background(), server, user)

    // 打印报告
    r.PrintReport()
}
```

### Mock 服务器

用于本地测试，模拟面板 API：

```go
package main

import (
    "context"
    "github.com/anixops/v2board/tests/integration/mock"
)

func main() {
    srv := mock.NewServer(8080)

    // 添加测试用户
    srv.AddUser(&mock.MockUser{
        ID:    1,
        UUID:  "test-uuid",
        Email: "test@example.com",
    })

    // 启动
    srv.Start(context.Background())
    defer srv.Stop(context.Background())

    // API 可用:
    // GET  /health
    // GET  /api/v2/server/UniProxy/config
    // GET  /api/v2/server/UniProxy/user
    // POST /api/v2/server/UniProxy/push
    // POST /api/v2/server/UniProxy/alive
    // POST /api/v2/node/register
    // POST /api/v2/node/heartbeat
}
```

## 测试场景

默认测试场景：

| 名称 | 协议 | 传输 | TLS |
|------|------|------|-----|
| vmess-tcp | VMess | TCP | 无 |
| vmess-ws-tls | VMess | WebSocket | TLS |
| vless-reality | VLESS | TCP | Reality |
| trojan-tls | Trojan | TCP | TLS |
| ss-tcp | Shadowsocks | TCP | 无 |
| hysteria2 | Hysteria2 | QUIC | TLS |

## 环境变量

运行完整集成测试需要设置：

```bash
export TEST_SERVER_HOST="your-server.com"
export TEST_SERVER_PORT="443"
export TEST_USER_UUID="your-uuid"
export TEST_PROTOCOL="vless"
export TEST_REALITY_PUBLIC_KEY="your-public-key"
export TEST_REALITY_SHORT_ID="your-short-id"
```

## 运行测试

```bash
# 运行所有单元测试（跳过需要网络的测试）
go test -v -short ./tests/integration/...

# 运行所有测试（包括完整集成测试）
go test -v ./tests/integration/...

# 运行特定模块测试
go test -v ./tests/integration/config/...
go test -v ./tests/integration/clients/...
go test -v ./tests/integration/runner/...
go test -v ./tests/integration/binary/...
go test -v ./tests/integration/mock/...

# 测试覆盖率
go test -cover ./tests/integration/...
```

## CI/CD 集成

项目已配置 GitHub Actions 自动运行测试。工作流文件位于 `.github/workflows/integration-test.yml`。

## 输出示例

```
========== Integration Test ==========
Server: example.com:443
Protocol: vless
Transport: tcp
TLS: reality
UUID: your-uuid
Scenarios: 6
Timeout: 30s
=======================================

========== Test Report ==========
Time: 2026-03-13T12:00:00Z
Total: 12, Passed: 10, Failed: 2
Pass Rate: 83.3%

Details:
  ✅ PASS [xray/vless] vless-reality (5.2s) - 120ms
  ✅ PASS [mihomo/vless] vless-reality (4.8s) - 98ms
  ✅ PASS [xray/vmess] vmess-tcp (3.1s) - 45ms
  ❌ FAIL [mihomo/vmess] vmess-ws-tls (2.5s) - 0s
       Error: connectivity test failed: connection timeout
=================================
```