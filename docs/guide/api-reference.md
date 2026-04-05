# API 参考

## 概述

所有 API 使用 `/api/v2` 前缀。响应格式为 JSON。

---

## 认证方式

### 用户认证

使用 JWT Token，通过 `Authorization` 头传递：

```
Authorization: Bearer <token>
```

### 节点认证

UniProxy 鉴权使用 `node_id` 查询参数 + `X-API-Key` 请求头。

```
# URL 参数方式
GET /api/v2/server/UniProxy/config?node_id=1

# Header 方式
X-API-Key: <api_key>
```

---

## 公开接口

### 登录

```http
POST /api/v2/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123"
}
```

**响应**:
```json
{
  "message": "登录成功",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": 1,
      "email": "user@example.com",
      "is_admin": false
    }
  }
}
```

### 注册

```http
POST /api/v2/register
Content-Type: application/json

{
  "email": "newuser@example.com",
  "password": "password123"
}
```

---

## 用户接口

### 获取用户信息

```http
GET /api/v2/user/info
Authorization: Bearer <token>
```

**响应**:
```json
{
  "data": {
    "id": 1,
    "email": "user@example.com",
    "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "balance": 1000,
    "transfer_enable": 107374182400,
    "u": 1073741824,
    "d": 5368709120,
    "expired_at": 1735689600,
    "plan_id": 1
  }
}
```

### 获取订阅

```http
GET /api/v2/user/subscription
Authorization: Bearer <token>
```

---

## 管理员接口

### 仪表盘统计

```http
GET /api/v2/admin/dashboard
Authorization: Bearer <admin_token>
```

**响应**:
```json
{
  "data": {
    "total_users": 100,
    "active_users": 80,
    "expired_users": 15,
    "banned_users": 5,
    "today_new_users": 3,
    "total_orders": 50,
    "pending_orders": 2,
    "paid_orders": 48,
    "total_revenue": 50000,
    "month_revenue": 10000,
    "today_revenue": 500,
    "total_nodes": 10,
    "online_nodes": 8,
    "total_traffic": 1099511627776
  }
}
```

### 用户管理

```http
# 获取用户列表
GET /api/v2/admin/users?page=1&page_size=20&email=&status=

# 创建用户
POST /api/v2/admin/users
{
  "email": "newuser@example.com",
  "password": "password123",
  "plan_id": 1,
  "expired_at": "2025-12-31T23:59:59Z"
}

# 更新用户
PUT /api/v2/admin/users/:id
{
  "banned": 0,
  "balance": 1000
}

# 删除用户
DELETE /api/v2/admin/users/:id
```

### 订单管理

```http
# 获取订单列表
GET /api/v2/admin/orders?page=1&page_size=20&trade_no=&email=

# 订单统计
GET /api/v2/admin/orders/stats
```

### 节点管理

```http
# 获取节点列表
GET /api/v2/admin/nodes?page=1&size=20

# 节点统计
GET /api/v2/admin/nodes/stats

# 获取节点协议
GET /api/v2/admin/nodes/:id/protocols

# 添加协议
POST /api/v2/admin/nodes/:id/protocols
{
  "name": "VLESS-Reality",
  "type": "vless",
  "port": 443,
  "tls": 2,
  "transport": "tcp",
  "settings": "{\"flow\":\"xtls-rprx-vision\"}"
}

# 更新协议
PUT /api/v2/admin/nodes/:id/protocols/:protocol_id

# 删除协议
DELETE /api/v2/admin/nodes/:id/protocols/:protocol_id
```

### 授权密钥管理

```http
# 获取密钥列表
GET /api/v2/admin/auth-keys

# 生成密钥
POST /api/v2/admin/auth-keys
{
  "name": "Tokyo Node",
  "expires_at": "2025-12-31T23:59:59Z"
}

# 删除密钥
DELETE /api/v2/admin/auth-keys/:id
```

---

## 节点接口

### 节点注册

```http
POST /api/v2/node/register
Content-Type: application/json

{
  "auth_key": "abc123def456...",
  "name": "Tokyo-Node-01",
  "host": "node1.example.com",
  "port": 443,
  "server_version": "1.0.0",
  "server_os": "Linux 5.15"
}
```

**响应**:
```json
{
  "message": "注册成功",
  "data": {
    "node_id": 1,
    "api_key": "a1b2c3d4e5f6...",
    "secret": "x9y8z7w6v5u4...",
    "message": "节点注册成功"
  }
}
```

### 节点心跳

```http
POST /api/v2/node/heartbeat
X-API-Key: <api_key>
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

---

## UniProxy 接口

### 获取节点配置

```http
GET /api/v2/server/UniProxy/config?node_id=1
X-API-Key: <api_key>
```

**响应**:
```json
{
  "node_type": "vless",
  "server_port": 443,
  "host": "node1.example.com",
  "server_name": "node1.example.com",
  "send_through": "0.0.0.0",
  "network": "ws",
  "network_settings": {
    "path": "/ws"
  },
  "tls": 1,
  "tls_settings": {
    "server_name": "node1.example.com"
  },
  "flow": "xtls-rprx-vision",
  "routes": [],
  "base_config": {
    "push_interval": 60,
    "pull_interval": 60
  }
}
```

### 获取用户列表

```http
GET /api/v2/server/UniProxy/user?node_id=1
X-API-Key: <api_key>
```

**响应**:
```json
{
  "users": [
    {
      "id": 1,
      "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "speed_limit": 0,
      "device_limit": 3
    }
  ]
}
```

### 获取在线列表

```http
GET /api/v2/server/UniProxy/alivelist?node_id=1
X-API-Key: <api_key>
```

**响应**:
```json
{
  "alive": {
    "1": 2,
    "5": 1
  }
}
```

### 上报流量

```http
POST /api/v2/server/UniProxy/push?node_id=1
X-API-Key: <api_key>
Content-Type: application/json

{
  "1": [1024000, 2048000],
  "2": [512000, 1024000]
}
```

**说明**: key 为用户 ID，value 为 [上传, 下载] 字节数

### 上报在线状态

```http
POST /api/v2/server/UniProxy/alive?node_id=1
X-API-Key: <api_key>
Content-Type: application/json

{
  "1": ["192.168.1.100", "192.168.1.101"],
  "2": ["10.0.0.50"]
}
```

**说明**: key 为用户 ID，value 为在线 IP 列表

---

## 支付接口

### 创建订单

```http
POST /api/v2/payment/order
Authorization: Bearer <token>
Content-Type: application/json

{
  "plan_id": 1,
  "period": "monthly",
  "payment_method": "stripe"
}
```

### 支付回调

```http
POST /api/v2/payment/notify/:method
```

---

## 错误响应

### 格式

```json
{
  "message": "错误信息",
  "code": 400
}
```

### 常见错误码

| 状态码 | 说明 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未授权 / Token 无效 |
| 403 | 禁止访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 版本兼容性

| 后端版本 | API 版本 | 状态 |
|---------|----------|------|
| ≥ 2.0.0 | v2 only | ✅ 当前 |
| < 2.0.0 | v1 only | ❌ 已弃用 |

**注意**: v1 API 已完全移除，所有客户端必须使用 v2 API。

## Flux-panel Compat Endpoints

### Response Envelope

Compat endpoints wrap replies in `{ "code": <int>, "msg": "<string>", "ts": <ms>, "data": {...} }`, mirroring the flux-panel reference. Timestamps stay in milliseconds so the cloned UI avoids extra conversions, and error branches keep the same envelope so the page always unwraps `data`.

### Forward & Tunnel Endpoint Mapping

| Flux Reference | Local Route | Auth Scope | Notes |
|------|------|------|------|
| `POST /api/v1/forward/create` | `POST /api/v2/forward/create` + `/api/v2/admin/forward/create` | JWT user | Identical payload and response; admin mirror exists for compatibility. |
| `POST /api/v1/forward/list` | `POST /api/v2/forward/list` + `/api/v2/admin/forward/list` | JWT user | Returns the `PanelForwardListItem` used by the cloned Forward view. |
| `POST /api/v1/forward/update` | `POST /api/v2/forward/update` + `/api/v2/admin/forward/update` | JWT user | Keeps the `ForwardUserTunnel` check for non-admins. |
| `POST /api/v1/forward/delete` | `POST /api/v2/forward/delete` + `/api/v2/admin/forward/delete` | JWT user | Matches reference semantics. |
| `POST /api/v1/forward/force-delete` | `POST /api/v2/forward/force-delete` + `/api/v2/admin/forward/force-delete` | JWT user | Same request/response bodies. |
| `POST /api/v1/forward/pause` | `POST /api/v2/forward/pause` + `/api/v2/admin/forward/pause` | JWT user | Local handler persists status while the reference also touches remote runtime. |
| `POST /api/v1/forward/resume` | `POST /api/v2/forward/resume` + `/api/v2/admin/forward/resume` | JWT user | Mirrors pause/resume contract. |
| `POST /api/v1/forward/diagnose` | `POST /api/v2/forward/diagnose` + `/api/v2/admin/forward/diagnose` | JWT user | Diagnoses still run from the panel side; flux-panel traces node chains. |
| `POST /api/v1/forward/update-order` | `POST /api/v2/forward/update-order` + `/api/v2/admin/forward/update-order` | JWT user | Uses the same `{ "forwards": [{ "id": id, "inx": idx }] }` payload. |
| `POST /api/v1/tunnel/user/tunnel` | `POST /api/v2/tunnel/user/tunnel` + `/api/v2/admin/tunnel/user/tunnel` | JWT user | Supplies the tunnel list consumed by the Forward UI. |

### DTO Expectations

Tunnel DTOs preserve the reference fields: `id`, `name`, `ip`, `inNodePortSta`, `inNodePortEnd`, `type`, `protocol`. The local service may also include `inIp` and `status`, but UI code must continue reading the flux field names only. Forward DTOs keep the original names (`tunnelId`, `strategy`, `inPort`, `remoteAddr`, `interfaceName`, `status`, `inFlow`, `outFlow`, `createdTime`, `updatedTime`, etc.) under the standard envelope.

### Current Known Gaps

- **Runtime side effects**: flux-panel triggers remote runtime updates on forward create/update/delete/pause/resume, while our handler currently updates database entries only; document this shortfall before claiming full parity.
- **Diagnose semantics**: the reference traces relay→exit node chains, whereas ours still uses panel-side dialing; log the divergence in clone docs.
- **UserTunnel lifecycle**: the list honors `ForwardUserTunnel`, but the remaining quota/expire/flow reset flows still need flux-level implementation.
- **DTO casing**: keep camelCase names intact and avoid removing fields that exist in the flux reference.
