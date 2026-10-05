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

### 重置订阅链接

```http
POST /api/v2/user/subscription/reset
Authorization: Bearer <token>
Idempotency-Key: <optional, a retry with the same key applies once>

{"password": "当前密码"}
# 已开启两步验证时改为提交验证码或恢复码：
{"code": "123456", "method": "totp"}
{"code": "ABCD-1234", "method": "backup"}
```

The signed-in user issues themselves a new subscription token, exactly as
`POST /api/v2/admin/users/:id/reset-subscribe` does: the old `/s/<token>`
link stops working at once, the proxy UUID is kept (nodes and connected
clients are unaffected), and the subscriber request ledger records it as
`identity.user_reset_subscribe:<user>:<digest>`. Served by identity-platform
(group A) in every edition; audited as `user` / `reset_subscribe` without
the body.

- Re-authentication: the current password; with two-step verification on,
  a TOTP code or a recovery code instead (`method` `totp` or `backup`;
  empty tries TOTP, then a recovery code, which is consumed).
- Limit: three attempts that check a credential per user and hour,
  successful or not; then `Retry-After`.

**响应**（v2 panel envelope）:
```json
{"code": 0, "msg": "操作成功", "data": {"token": "<new subscription token>"}}
```

| `msg` (code ≠ 0) | Meaning |
|---|---|
| `参数错误` | body is not JSON |
| `password required` / `invalid password` | no second factor: password missing or wrong |
| `mfa code required` / `invalid mfa code` | second factor on: code missing or wrong |
| `too many subscription reset attempts, please try again later` | limit reached (`Retry-After` header) |
| `用户不存在` | the account no longer exists |
| `重置订阅失败: …` | Control could not reset the token |

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
GET /api/v2/admin/users?sort=traffic&order=desc

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

# 重置流量 / 重置订阅链接（新 token，UUID 不变；用户也可自助重置，见「重置订阅链接」）
POST /api/v2/admin/users/:id/reset-traffic
POST /api/v2/admin/users/:id/reset-subscribe

# 用户最近在线时间（v4，仅管理员）
GET /api/v4/admin/users/activity?ids=3,5,8

# 批量封禁 / 解封 / 重置流量（v4，仅管理员；见下文 Bulk actions）
POST /api/v4/admin/users/bulk
{"action": "ban", "ids": [3, 5, 8]}

# 批量撤销邀请码
POST /api/v4/admin/invite-codes/bulk
{"action": "revoke", "ids": [11, 12]}
```

The list (`data.list`, with `data.total`) filters by `email` (substring),
`plan_id` and `status` (`active`, `expired`, `banned`), newest first
(`created_at DESC, id DESC`). Each user carries the account, the
subscription summary and its plan as `{id, name}` (left out when the user
has no plan or the plan no longer exists), never the subscription token or
proxy UUID; read those, the remark and the plan row from
`GET /api/v2/admin/users/:id`, one user at a time (`docs/UPGRADE.md`).

**Sorting (`sort`, `order`).** Without them the list keeps the order above.
`sort` names the column: `id`, `email`, `created_at`, `expired_at`,
`traffic` (used traffic, `u + d`) or `transfer_enable`; `order` is `asc` (the
default) or `desc`, in any case, and is read only with `sort`. Users that
share a value come by id, newest first, so pages neither repeat nor skip a
user; a user without an expiry sorts as the largest value (last ascending,
first descending, on SQLite and PostgreSQL alike). An empty `sort` is no
sort. Any other `sort` or `order` is refused as this list refuses every bad
input, HTTP 200 with the panel error envelope (`code` -1 and a `msg` naming
the accepted values). The same two parameters sort the order and node
lists below, each with its own columns. The identity package's native list
answers the same order (`internal/tests/identitycompat`). The user's
status is derived (banned, expired, exhausted) and the plan's name comes
from another table, so neither is a sort column.

```json
{
  "id": 3, "email": "user@example.com", "balance": 0, "commission_balance": 0,
  "device_limit": 2, "speed_limit": null, "flowResetTime": 0,
  "transfer_enable": 107374182400, "u": 1024, "d": 4096,
  "plan_id": 2, "group_id": 1, "expired_at": 1798761600,
  "banned": 0, "is_admin": 0, "is_staff": 0, "created_at": "2026-09-01T08:00:00Z",
  "plan": {"id": 2, "name": "Pro"}
}
```

**Last online (`GET /api/v4/admin/users/activity`).** `ids` is a comma
separated list of user ids (at most 200, repeats dropped, HTTP 400
`{"error": {"code": "invalid_request", "message": ...}}` for an empty list, a
value that is not a positive id or more than 200). It answers `{"data":
{"users": [...]}}` with an entry per id, in the order asked:

```json
{"data": {"users": [
  {"user_id": 3, "last_online_at": 1760000000},
  {"user_id": 5, "last_online_at": null}
]}}
```

`last_online_at` is a Unix time in seconds: the last time a node reported
the user's traffic or listed one of their connections (UniProxy, gRPC and
Agent Control alike), accurate to a minute, or `null` for a user never seen
since Control began recording it (`docs/reference/traffic-stats-operations.md`,
"User Last Online"). The user list and detail above keep their v2 shape, which
the identity package's native handlers must answer byte for byte; ask this
route for the ids of the page shown.

**Bulk actions (`POST /api/v4/admin/users/bulk`, `/api/v4/admin/invite-codes/bulk`).**
One request does an action for up to 200 ids instead of the console calling
the single-item route once per row. `action` is `ban`, `unban` or
`reset_traffic` for users and `revoke` for invite codes; `ids` are positive
integers (repeats are dropped, the order kept). Deleting users and resetting
subscription links are deliberately not bulk actions.

- **Each item is the single-item request.** The kernel runs
  `POST /api/v2/admin/users/:id/ban`, `.../unban`, `.../reset-traffic` and
  `DELETE /api/v2/admin/invite/codes/:id` for each id in turn, through the same
  package gateway the route is served by, so the item runs wherever that
  route's mode puts it (the identity package natively, or the kernel's
  handler) and does exactly what the single request does. There is no second
  implementation.
- **Per item, not all or nothing.** Items are independent: a failure stops
  nothing and nothing is rolled back. A valid request is always HTTP 200 with
  every outcome, in the order of the request; only an invalid request (not
  JSON, an unknown field or action, no ids, an id that is not a positive
  integer, more than 200 ids) is HTTP 400
  `{"error": {"code": "invalid_request", "message": ...}}`, and then nothing
  runs. A request has 25 seconds, under the server's write timeout; the items it
  has no time left for are `not_attempted`.

```json
{"data": {"action": "ban", "requested": 3, "succeeded": 2, "failed": 1, "results": [
  {"id": 3, "ok": true},
  {"id": 5, "ok": true},
  {"id": 8, "ok": false, "error": {"code": "not_found", "message": "用户不存在"}}
]}}
```

  Error codes: `not_found` (no such user or invite code), `conflict` (an
  invite code that was used is kept), `forbidden_self` (an administrator
  cannot ban their own account in a bulk request, so a page selected whole is
  safe), `not_attempted`, a code of the package gateway
  (`package_route_frozen`, `plugin_host_unavailable`, ...) and `failed` for
  any other refusal of the route, with the route's own message.
- **Idempotent where it can be.** `ban` and `unban` are: a user already in
  the target state is `ok`. `reset_traffic` zeroes once per `Idempotency-Key`
  header and user (as the single route does): a client that retries a bulk
  request sends the same key and no counter is reset twice; without the
  header every request is a new reset. A retried `revoke` reports the codes
  that were revoked the first time as `not_found`.
- **Audited.** The middleware records the request itself (module `users` or
  `invite-codes`, action `bulk_ban`, `bulk_unban`, `bulk_reset_traffic` or
  `bulk_revoke`, with the action and ids as the request body), and each item
  also leaves the row its single-item route leaves (`POST
  /api/v2/admin/users/12/ban`, ...), with the administrator, address and
  request id of the bulk request and, for a refused item, the HTTP status
  (404, 409, 403, 422...) and the code and message, so a user's history is
  found by path whichever way it was changed.

### 订单管理

```http
# 获取订单列表
GET /api/v2/admin/orders?page=1&page_size=20&trade_no=&email=
GET /api/v2/admin/orders?sort=total_amount&order=desc

# 订单详情
GET /api/v2/admin/orders/:id

# 订单统计
GET /api/v2/admin/orders/stats
```

Each order in the list (`data.list`) and the detail (`data`) carries the
order's own fields, its plan as `{id, name}` and its buyer as `{id, email}`;
a plan or buyer that no longer exists is left out. The user's routes
(`GET /api/v2/user/order`, `GET /api/v2/user/order/:id`) answer the caller's
own orders the same way, without `user`. No answer carries the buyer's
subscription token or UUID (`docs/UPGRADE.md`).

The admin list sorts with `sort` and `order` as the user list does (see
"Sorting" there): `sort` is `id`, `trade_no`, `status`, `type`,
`total_amount`, `created_at` or `paid_at` (an unpaid order, `NULL`, sorts as
the largest value), orders that share a value come by id, newest first, and
without `sort` the list stays newest first. The buyer's e-mail and the
plan's name are other tables' and are not sort columns. The order package's
native list answers the same order.

```json
{
  "id": 12, "user_id": 3, "plan_id": 2, "trade_no": "20261001120000ABCD1234",
  "period": "month", "total_amount": 3000, "status": 0, "...": "the other order fields",
  "user": {"id": 3, "email": "buyer@example.com"},
  "plan": {"id": 2, "name": "Pro"}
}
```

### 节点管理

```http
# 获取节点列表
GET /api/v2/admin/nodes?page=1&size=20
GET /api/v2/admin/nodes?sort=last_check_at&order=desc

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

节点列表默认按管理员排序（`sort` 升序，`id` 倒序）。`sort` / `order` 与用户列表同规则
（见「用户管理」中的 Sorting）：`sort` 取 `id`、`name`、`host`、`sort`（管理员权重）、
`created_at`、`last_check_at`（从未上报心跳的节点视为最大值）、`cpu_usage`、`online_users`，
并列的节点按 `id` 倒序；非法的 `sort` / `order` 返回 HTTP 400 `{"message": ...}`
（本列表一贯的错误形状）。显示状态由最近一次心跳推算、协议数来自另一张表，二者不是排序列。
proxy-node 包的原生列表给出同样的顺序。

管理端应答不再明文返回节点密钥：协议 `settings`、`tls_settings`、
`transport_settings`、`reality_settings`、`custom_config` 中名称表示密钥的字段
（Reality/TLS `private_key`、WireGuard `server_private_key`、`server_key`、
`password`、`obfs-password`、`psk`、`token` 等）以及节点 `raw_config` 中的密钥都显示为
`********`；公钥和 Reality `short_id` 照常显示。更新时原样提交 `********` 即保留已存的值，
提交新值则替换。节点自身仍通过 UniProxy / gRPC 拿到真实配置。

`GET /api/v2/admin/nodes/:id/credentials` 返回单个节点的 `api_key` / `secret`
（部署助手和 Ansible 使用），每次读取都记入审计日志（action `reveal`）。

SMTP 密码同样不再明文返回：`GET /api/v2/admin/notification/email/config` 的
`password`，以及系统配置列表、单项读取和更新应答中 `notification.email.config`
值里的 `password` 字段，已设置时显示为 `********`（未设置为空）。更新时提交
`********`（或在邮件配置接口中留空）即保留已存的密码，提交新值则替换。测试邮件仍使用已存的密码。

### 授权密钥管理

```http
# 获取密钥列表（key 显示为 ********）
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

授权密钥只在 `POST /api/v2/admin/auth-keys` 的应答中返回一次，请当场复制；
列表中的 `key` 显示为 `********`，节点注册按密钥哈希校验，不受影响。

### 转发（v4.2）

转发接口是 forward 软件包的 `/api/v4/forward/*`，见
[`../forwarding/v4-api.md`](../forwarding/v4-api.md)。

v4.2（F5d）删除了 flux 兼容的 v2 转发接口，这些路径现在返回 404：

- 用户与管理员的转发增删改、强制删除、暂停、恢复、诊断
  （`/api/v2/forward/*`、`/api/v2/admin/forward/{create,update,delete,force-delete,pause,resume,diagnose}`）；
- 旧版规则（`/api/v2/admin/forward/rules*`、`POST /api/v2/user/forward/rules`）、
  `POST /api/v2/admin/forward/sync-backend`、`GET /api/v2/admin/forward/runtime/jobs`；
- 隧道诊断与更新、隧道授权的删除与更新
  （`/api/v2/admin/tunnel/{diagnose,update}`、`/api/v2/{admin/,}tunnel/user/{remove,update}`）、
  `POST /api/v2/speed-limit/update`；
- 转发节点与 Ansible 机器（`/api/v2/admin/forward/nodes*`、`/api/v2/admin/forward/ansible-machines*`，
  由 `/api/v4/forward/nodes`、`/ansible-machines` 取代）、可观测性
  `targets`/`trend`/`topology`（由 `/api/v4/forward/observability/*` 取代）；
- 干净代理的令牌接口与安装脚本（`/api/v2/admin/forward/agents*`、
  `GET /api/v2/forward-agent/install.sh`）。

仍保留、但已没有页面的 v2 接口（只读或旧运行时，随旧版清理 F5c 一起移除）：转发与隧道列表、
排序、隧道创建与删除、隧道授权的分配与列表、限速规则的增删查、`POST /api/v2/user/reset`、
多入口对比与统计、运行时状态与诊断（`/admin/forward/{runtime,local,nodex}/*`）、
干净代理的注册、心跳与上报，以及内部流量上报。旧版接口的契约存档在
[`../archive/forwarding-v2-api.md`](../archive/forwarding-v2-api.md)。

### Scheduled reset semantics

- `flowResetTime = 0` 表示不参与自动月重置。
- `flowResetTime = 1..31` 表示每月对应日期重置；当月没有该日时，按月末补执行。
- 当前本地实现会在应用启动时执行一次补扫，然后每天本地时间 `00:00:05` 扫描。
- 扫描会重置用户流量；对已过期用户会暂停其旧版转发。

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
