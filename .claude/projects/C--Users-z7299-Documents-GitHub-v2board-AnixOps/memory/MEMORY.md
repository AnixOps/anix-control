# V2Board AnixOps 开发记忆

## 项目概览
- **后端**: Go + Gin + GORM + SQLite/PostgreSQL
- **前端**: Vue 3 + Vite + Pinia + Vue Router
- **节点端**: V2bX_AnixOps (Go + Xray/Sing-box/Hysteria2)

## 架构升级 (2026-03-12)

### gRPC 双边通信 (替代心跳)
- 协议定义: `api/grpc/v2board.proto`
- 服务端实现: `internal/grpc/node_server.go`
- 支持双向流式通信，实现配置实时推送

### WebSocket 订阅模式
- 实现: `internal/websocket/subscription.go`
- 支持用户实时订阅节点变更
- 心跳保活机制

## 测试框架设置

### Go 后端测试
- 测试框架: `testify/suite` + `httptest`
- 测试工具目录: `tests/testutil/`
- E2E 测试目录: `tests/e2e/`
- 运行命令: `make test` 或 `go test -v ./...`

### 当前覆盖率

| 模块 | 覆盖率 |
|------|--------|
| internal/utils | 62.0% |
| internal/model | 11.5% |
| internal/service | 11.2% |
| internal/handler | 3.6% |

### 关键命令
```bash
make test              # 运行所有测试
make test-coverage     # 覆盖率报告
make test-coverage-html # HTML 报告
```

## 数据库模型关键字段
- User: `Email`, `Password`, `Token`, `UUID`, `TransferEnable`, `Banned`, `IsAdmin`
- Plan: `Name`, `TransferEnable`, `SpeedLimit`, `DeviceLimit`
- Node: `Name`, `Host`, `Port`, `Status`, `Rate`, `TrafficRate`
- NodeProtocol: `NodeID`, `Type`, `Port`, `TLS`, `Transport`

## 新增文件
- `api/grpc/v2board.proto` - gRPC 协议定义
- `internal/grpc/node_server.go` - gRPC 服务端
- `internal/websocket/subscription.go` - WebSocket 订阅
- `internal/service/service_test.go` - 服务层测试
- `internal/handler/handler_test.go` - Handler 测试
- `tests/e2e/*.go` - E2E 测试套件