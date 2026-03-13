# V2Board AnixOps 开发进度报告

## 完成的任务

### 1. gRPC 双边通信架构 ✅

**协议定义文件**: `api/grpc/v2board.proto`

包含以下服务:
- **NodeService**: 节点注册、配置获取、状态上报、双向流通信
- **UserService**: 用户列表获取、用户变更实时推送
- **TrafficService**: 流量上报、在线状态上报、双向流实时通信
- **ConfigSyncService**: 配置同步、变更推送
- **HealthService**: 健康检查

**服务端实现**: `internal/grpc/node_server.go`

核心特性:
- 替代原有心跳机制，使用 gRPC 双向流 (Bidirectional Streaming)
- 节点配置实时推送
- 用户列表实时同步
- 流量实时上报
- 支持双向通信，面板可主动推送配置变更

**面板端 gRPC 代码**: `api/grpc/v2boardpb/`
- `v2board.pb.go` - Protobuf 消息定义
- `v2board_grpc.pb.go` - gRPC 服务存根

### 2. 节点端 gRPC 客户端 ✅

**实现文件**: `V2bX_AnixOps/api/grpc/client.go`

核心功能:
- gRPC 连接管理 (keepalive, 重连)
- 节点注册 (自动发现)
- 节点配置获取
- 用户列表获取
- 流量上报 (批量 & 流式)
- 在线状态上报 (批量 & 流式)
- 节点状态上报 (CPU/内存/磁盘/在线用户)
- 双向流通信:
  - StatusStream: 状态上报 & 配置更新接收
  - TrafficStream: 流量实时上报
  - OnlineStream: 在线状态实时上报
  - UserChanges: 用户变更通知

**Protobuf 文件**: `V2bX_AnixOps/api/grpc/v2boardpb/`
- 复用面板端 protobuf 定义

### 3. WebSocket 订阅模式 ✅

**实现文件**: `internal/websocket/subscription.go`

核心功能:
- 用户实时订阅节点变更
- 订阅内容实时更新
- 用户状态实时通知
- 支持广播和定向推送
- 心跳保活机制

消息类型:
- `subscribe` / `unsubscribe`: 订阅管理
- `node_update`: 节点更新通知
- `user_update`: 用户更新通知
- `config_update`: 配置更新通知
- `heartbeat`: 心跳

### 4. 测试覆盖率 🔄

**当前覆盖率**:

| 模块 | 覆盖率 |
|------|--------|
| internal/utils | 62.0% |
| internal/model | 11.5% |
| internal/service | 11.2% |
| internal/handler | 3.6% |

**新增测试文件**:
- `internal/service/service_test.go` - 服务层全面测试
- `internal/handler/handler_test.go` - Handler 测试
- `tests/e2e/auth_test.go` - 认证 API E2E 测试
- `tests/e2e/admin_test.go` - 管理员 API E2E 测试
- `tests/e2e/uniproxy_test.go` - UniProxy API 测试
- `tests/e2e/subscribe_test.go` - 订阅 API 测试

## 架构变更

### 通信架构升级

```
原架构:
┌─────────┐    HTTP心跳     ┌─────────┐
│  节点端  │ ──────────────→ │  面板端  │
└─────────┘                 └─────────┘

新架构:
┌─────────┐    gRPC 双向流    ┌─────────┐
│  节点端  │ ←──────────────→ │  面板端  │
└─────────┘                   └─────────┘
     ↓                              ↓
     │     WebSocket 订阅推送       │
     └──────────────────────────────┘
```

### 用户订阅模式

```
┌─────────┐    WebSocket     ┌─────────┐
│  用户端  │ ←──────────────→ │  面板端  │
└─────────┘                   └─────────┘
     ↑                              │
     │      实时推送订阅更新         │
     └──────────────────────────────┘
```

## 使用说明

### 面板端 gRPC 服务启动

```go
import (
    "google.golang.org/grpc"
    pb "github.com/anixops/v2board/api/grpc/v2boardpb"
    grpcServer "github.com/anixops/v2board/internal/grpc"
)

func main() {
    lis, _ := net.Listen("tcp", ":50051")
    s := grpc.NewServer()

    pb.RegisterNodeServiceServer(s, grpcServer.NewNodeGRPCServer())
    pb.RegisterUserServiceServer(s, grpcServer.NewUserGRPCServer())
    pb.RegisterTrafficServiceServer(s, grpcServer.NewTrafficGRPCServer())
    pb.RegisterHealthServiceServer(s, grpcServer.NewHealthGRPCServer())

    s.Serve(lis)
}
```

### 节点端 gRPC 客户端

```go
import (
    "github.com/InazumaV/V2bX/api/grpc"
)

func main() {
    client, err := grpc.NewGRPCClient(&grpc.GRPCClientConfig{
        Host:   "panel.example.com:50051",
        NodeID: 1,
        APIKey: "your-api-key",
    })
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // 获取节点配置
    nodeInfo, err := client.GetNodeConfig()

    // 获取用户列表
    users, err := client.GetUsers()

    // 启动双向流
    client.StartStreams()

    // 上报流量
    client.ReportTraffic(traffics)
}
```

### WebSocket 订阅

```javascript
// 用户端连接
const ws = new WebSocket('ws://localhost:8080/ws?token=YOUR_JWT_TOKEN');

// 订阅节点更新
ws.send(JSON.stringify({
    type: 'subscribe',
    data: { channels: ['node_updates', 'config_updates'] }
}));

// 接收消息
ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    console.log('Received:', msg.type, msg.data);
};
```

## 测试命令

```bash
# 运行所有测试
make test

# 运行服务层测试
go test -v ./internal/service/...

# 运行 E2E 测试
go test -v ./tests/e2e/...

# 生成覆盖率报告
make test-coverage-html
```

## 编译命令

```bash
# 面板端
cd v2board_AnixOps
go build -o v2board cmd/server/main.go

# 节点端 (需要 GOEXPERIMENT=jsonv2)
cd V2bX_AnixOps
GOEXPERIMENT=jsonv2 go build -o V2bX main.go
```