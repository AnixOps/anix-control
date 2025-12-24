# V2 API 迁移指南

## 概述

本项目已从 `/api/v1` 完全迁移到 `/api/v2`。所有客户端（包括 V2bX、XrayR 等节点程序）需要相应更新配置。

## API 变更清单

| 旧路径 (v1) | 新路径 (v2) | 说明 |
|-------------|-------------|------|
| `/api/v1/login` | `/api/v2/login` | 用户登录 |
| `/api/v1/register` | `/api/v2/register` | 用户注册 |
| `/api/v1/user/*` | `/api/v2/user/*` | 用户接口 |
| `/api/v1/admin/*` | `/api/v2/admin/*` | 管理员接口 |
| `/api/v1/node/register` | `/api/v2/node/register` | 节点注册 |
| `/api/v1/node/heartbeat` | `/api/v2/node/heartbeat` | 节点心跳 |
| `/api/v1/server/UniProxy/*` | `/api/v2/server/UniProxy/*` | 节点通信 |
| `/api/v1/payment/*` | `/api/v2/payment/*` | 支付接口 |

---

## V2bX 客户端配置修改

### 1. 配置文件修改 (`config.yml`)

```yaml
# 修改前
Nodes:
  - ApiHost: "https://your-panel.com"
    ApiPath: "/api/v1"  # ← 旧版

# 修改后
Nodes:
  - ApiHost: "https://your-panel.com"
    ApiPath: "/api/v2"  # ← 新版
```

### 2. 完整配置示例

```yaml
Log:
  Level: warning
  Output: console

Nodes:
  - ApiHost: "https://your-v2board.com"
    ApiPath: "/api/v2"
    NodeID: 1
    NodeType: V2ray  # 可选: V2ray, Shadowsocks, Trojan, Hysteria, Hysteria2, TUIC, AnyTLS
    Token: "your-api-token"
    RuleListPath: ""
```

### 3. V2bX 源码修改 (如需自编译)

如果你需要自己编译 V2bX，需要修改以下文件：

**文件: `api/panel/panel.go`**

```go
// 修改默认 API 路径
const defaultAPIPath = "/api/v2"  // 原来是 "/api/v1"
```

---

## XrayR 客户端配置修改

### 配置文件修改 (`config.yml`)

```yaml
# 修改前
Nodes:
  - ApiConfig:
      ApiHost: "https://your-panel.com"
      NodeID: 1
      NodeType: V2ray

# 修改后
Nodes:
  - ApiConfig:
      ApiHost: "https://your-panel.com/api/v2"  # 添加 /api/v2 后缀
      NodeID: 1
      NodeType: V2ray
```

---

## 自定义节点客户端修改

### API 端点更新

所有 API 请求需要将 `/api/v1` 替换为 `/api/v2`：

```go
// 修改前
const (
    RegisterURL  = "/api/v1/node/register"
    HeartbeatURL = "/api/v1/node/heartbeat"
    ConfigURL    = "/api/v1/server/UniProxy/config"
    UsersURL     = "/api/v1/server/UniProxy/user"
)

// 修改后
const (
    RegisterURL  = "/api/v2/node/register"
    HeartbeatURL = "/api/v2/node/heartbeat"
    ConfigURL    = "/api/v2/server/UniProxy/config"
    UsersURL     = "/api/v2/server/UniProxy/user"
)
```

---

## 节点自动注册流程

### 1. 注册请求

```bash
POST https://your-panel.com/api/v2/node/register
Content-Type: application/json

{
    "auth_key": "your-authorization-key",
    "name": "Node-01",
    "host": "node1.example.com",
    "port": 443,
    "server_version": "1.0.0",
    "server_os": "linux amd64"
}
```

### 2. 获取节点配置

```bash
GET https://your-panel.com/api/v2/server/UniProxy/config?node_id=1&token=your-api-key
```

### 3. 获取用户列表

```bash
GET https://your-panel.com/api/v2/server/UniProxy/user?node_id=1&token=your-api-key
```

### 4. 上报心跳

```bash
POST https://your-panel.com/api/v2/node/heartbeat
X-API-Key: your-api-key
Content-Type: application/json

{
    "cpu_usage": 45.5,
    "memory_usage": 60.2,
    "disk_usage": 30.0,
    "uptime": 86400,
    "online_users": 150,
    "upload": 1073741824,
    "download": 5368709120
}
```

### 5. 上报流量

```bash
POST https://your-panel.com/api/v2/server/UniProxy/push
Content-Type: application/json

[
    {"uid": 1, "u": 1024000, "d": 2048000},
    {"uid": 2, "u": 512000, "d": 1024000}
]
```

---

## 响应格式

### 节点配置响应 (V2)

```json
{
    "node_type": "vmess",
    "server_port": 443,
    "host": "node1.example.com",
    "server_name": "node1.example.com",
    "send_through": "0.0.0.0",
    "network": "tcp",
    "tls": 0,
    "routes": [],
    "base_config": {
        "push_interval": 60,
        "pull_interval": 60
    }
}
```

> **注意**: V2 API 使用 `node_type` 字段（而非 `type`）来标识节点类型。

---

## 常见问题

### Q: 为什么要迁移到 v2？

A: v2 API 提供了更清晰的语义和更好的扩展性。主要改进包括：
- 使用 `node_type` 代替 `type` 更加明确
- 支持新版节点管理系统（多协议、RawConfig）
- 统一的错误响应格式

### Q: 旧版 v1 API 还能用吗？

A: 不能。系统已完全移除 v1 API 支持。

### Q: V2bX 报错 "node type not found"？

A: 请参考 [v2bx-client-fix.md](v2bx-client-fix.md) 修改 V2bX 源码以支持 `node_type` 字段。

### Q: 如何验证 API 是否正常？

A: 可以使用以下命令测试：

```bash
# 测试健康检查
curl https://your-panel.com/health

# 测试登录 (应返回 token)
curl -X POST https://your-panel.com/api/v2/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"your-password"}'
```

---

## 版本兼容性

| 后端版本 | API 版本 | 状态 |
|---------|----------|------|
| ≥ 2.0.0 | v2 only | ✅ 当前 |
| < 2.0.0 | v1 only | ❌ 已弃用 |
