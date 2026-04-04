# 订阅系统架构与使用指南

## 目录

- [1. 系统架构](#1-系统架构)
- [2. 订阅分组与权限](#2-订阅分组与权限)
- [3. 订阅模板](#3-订阅模板)
- [4. API 接口](#4-api-接口)
- [5. 支持的输出格式](#5-支持的输出格式)
- [6. 使用示例](#6-使用示例)
- [7. 安全注意事项](#7-安全注意事项)

---

## 1. 系统架构

### 1.1 核心概念

订阅系统采用 **分组-模板** 架构，支持灵活的权限控制和配置下发：

```
┌─────────────────────────────────────────────────────────────────────┐
│                          V2Board AnixOps                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────┐         ┌─────────────────────────────────┐    │
│  │ SubscriptionGroup│◀───────│ PlanSubscriptionGroup            │    │
│  │   订阅分组        │        │ (套餐自动获得分组权限)            │    │
│  │  - default       │         └─────────────────────────────────┘    │
│  │  - vip           │                                                │
│  │  - premium       │         ┌─────────────────────────────────┐    │
│  └────────┬────────┘◀────────│ UserSubscriptionGroup            │    │
│           │                   │ (手动分配用户分组权限)            │    │
│           ▼                   └─────────────────────────────────┘    │
│  ┌─────────────────┐                                                 │
│  │SubscriptionTemplate│  JSON 模板配置                               │
│  │   订阅模板        │  - 协议类型                                    │
│  │  - VLESS Reality │  - 服务器配置                                  │
│  │  - VMess WS      │  - TLS/Reality 配置                            │
│  │  - Trojan        │  - 传输层配置                                  │
│  │  - Hysteria2     │  - 变量占位符 ({{.UUID}})                      │
│  └────────┬────────┘                                                 │
│           │                                                          │
│           ▼                                                          │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                   订阅服务 (SubscriptionService)              │    │
│  │  1. 验证用户 Token                                            │    │
│  │  2. 获取用户可用分组 (套餐 + 手动分配)                         │    │
│  │  3. 渲染模板 (注入用户 UUID)                                   │    │
│  │  4. 格式化输出 (V2Ray/Clash/Surge...)                         │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                              │                                       │
└──────────────────────────────┼───────────────────────────────────────┘
                               ▼
                      GET /s/{user_token}
                               │
                               ▼
                   ┌───────────────────────┐
                   │    客户端订阅响应      │
                   │  - V2Ray Base64       │
                   │  - Clash YAML         │
                   │  - Base64 JSON (分组) │
                   └───────────────────────┘
```

### 1.2 数据流程

```
用户客户端                    面板                           数据库
    │                           │                               │
    │  GET /s/{token}           │                               │
    │  User-Agent: Clash/1.0    │                               │
    │ ─────────────────────────▶│                               │
    │                           │  根据 token 查询用户           │
    │                           │ ─────────────────────────────▶│
    │                           │◀─────────────────────────────│
    │                           │                               │
    │                           │  获取用户订阅分组              │
    │                           │  (套餐关联 + 直接分配)         │
    │                           │ ─────────────────────────────▶│
    │                           │◀─────────────────────────────│
    │                           │                               │
    │                           │  获取分组下的模板              │
    │                           │ ─────────────────────────────▶│
    │                           │◀─────────────────────────────│
    │                           │                               │
    │                           │  渲染模板 + 格式转换           │
    │  Clash YAML 响应          │  (检测 UA -> Clash 格式)       │
    │◀─────────────────────────│                               │
```

---

## 2. 订阅分组与权限

### 2.1 分组模型

```go
// SubscriptionGroup 订阅分组
type SubscriptionGroup struct {
    ID          uint      // 分组 ID
    Name        string    // 分组名称 (如: default, vip, premium)
    Description *string   // 分组描述
    Priority    int       // 优先级 (数值越大越靠前)
    Enable      int       // 是否启用
}
```

### 2.2 权限分配方式

#### 方式 1: 套餐关联 (推荐)

购买套餐自动获得对应分组权限：

```
套餐 A (基础版) ──关联──▶ 分组: default
套餐 B (高级版) ──关联──▶ 分组: default, vip
套餐 C (尊享版) ──关联──▶ 分组: default, vip, premium
```

#### 方式 2: 手动分配

管理员可为特定用户手动分配分组权限：

```http
POST /api/v2/admin/subscription/users/123/groups
{
    "group_id": 2,
    "expire_at": 1735689600  // 可选：权限过期时间
}
```

### 2.3 权限合并逻辑

用户最终可见的订阅 = 套餐关联分组 + 手动分配分组

```
用户 A:
  - 购买套餐: 基础版 → 获得 [default]
  - 手动分配: [vip] (到期时间: 2025-12-31)
  - 最终可见: [default, vip]
```

---

## 3. 订阅模板

### 3.1 模板模型

```go
// SubscriptionTemplate 订阅模板
type SubscriptionTemplate struct {
    ID               uint    // 模板 ID
    GroupID          uint    // 所属分组
    Name             string  // 节点名称 (显示给用户)
    Type             string  // 协议类型: vmess/vless/trojan/ss/hy2/tuic
    Enable           int     // 是否启用
    Sort             int     // 排序
    
    // 服务器配置
    Server           string  // 服务器地址
    Port             int     // 端口
    ServerName       *string // SNI/伪装域名
    
    // TLS 配置
    TLS              int     // 0=无, 1=TLS, 2=Reality
    TLSFingerprint   *string // TLS 指纹
    ALPN             *string // ALPN
    
    // Reality 配置
    RealityPublicKey *string
    RealityShortID   *string
    
    // 传输层配置
    Transport         string  // tcp/ws/grpc/h2/quic
    TransportSettings *string // JSON
    
    // 协议配置
    ProtocolSettings  *string // JSON
    
    // 高级: 自定义 JSON 模板
    TemplateJSON      string  // 支持变量占位符
}
```

### 3.2 变量占位符

模板名称和 TemplateJSON 支持以下变量：

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `{{.UUID}}` | 用户 UUID | `a1b2c3d4-...` |
| `{{.UserID}}` | 用户 ID | `123` |
| `{{.Email}}` | 用户邮箱 | `user@example.com` |
| `{{.ExpiredAt}}` | 过期时间戳 | `1735689600` |
| `{{.SpeedLimit}}` | 速度限制 (bytes/s) | `10485760` |
| `{{.DeviceLimit}}` | 设备限制 | `3` |
| `{{.TransferEnable}}` | 总流量 (bytes) | `107374182400` |
| `{{.UsedTraffic}}` | 已用流量 (bytes) | `5368709120` |
| `{{.Custom.xxx}}` | 自定义变量 | - |

### 3.3 模板示例

#### VLESS Reality 模板

```json
{
    "name": "🇺🇸 美国节点 - Reality",
    "type": "vless",
    "server": "us.example.com",
    "port": 443,
    "server_name": "www.microsoft.com",
    "tls": 2,
    "tls_fingerprint": "chrome",
    "transport": "tcp",
    "protocol_settings": "{\"flow\":\"xtls-rprx-vision\"}",
    "reality_public_key": "your-public-key",
    "reality_short_id": "abcd1234"
}
```

#### VMess WebSocket 模板

```json
{
    "name": "🇯🇵 日本节点 - WS",
    "type": "vmess",
    "server": "jp.example.com",
    "port": 443,
    "server_name": "jp.example.com",
    "tls": 1,
    "transport": "ws",
    "transport_settings": "{\"path\":\"/ws\",\"host\":\"jp.example.com\"}"
}
```

#### 自定义 JSON 模板 (高级)

使用 `template_json` 完全自定义输出：

```json
{
    "template_json": "{\"name\":\"🔥 VIP 专属 - {{.Email}}\",\"extra\":{\"uuid\":\"{{.UUID}}\",\"max_speed\":{{.SpeedLimit}}}}"
}
```

---

## 4. API 接口

### 4.1 用户订阅接口

#### 获取订阅

```http
GET /s/{user_token}
```

**URL 参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `type` | string | 输出格式: v2ray/clash/surge/shadowrocket/quantumultx/json/base64json |
| `groups` | string | 指定分组 ID (逗号分隔) |
| `include` | string | 包含节点名关键词 (正则) |
| `exclude` | string | 排除节点名关键词 (正则) |

**响应头:**

```
Content-Type: text/plain; charset=utf-8
Subscription-Userinfo: upload=0; download=5368709120; total=107374182400; expire=1735689600
Profile-Update-Interval: 24
Profile-Title: V2Board Subscription
```

**示例:**

```bash
# 自动检测格式 (根据 User-Agent)
curl https://panel.example.com/s/abc123

# 强制 Clash 格式
curl https://panel.example.com/s/abc123?type=clash

# Base64 JSON 格式 (分组呈现)
curl https://panel.example.com/s/abc123?type=base64json

# 只获取 VIP 分组
curl https://panel.example.com/s/abc123?groups=2

# 排除含 "测试" 的节点
curl https://panel.example.com/s/abc123?exclude=测试
```

### 4.2 管理员接口

#### 订阅分组管理

```http
# 获取所有分组
GET /api/v2/admin/subscription/groups

# 创建分组
POST /api/v2/admin/subscription/groups
{
    "name": "vip",
    "description": "VIP 专属节点",
    "priority": 10,
    "enable": 1
}

# 更新分组
PUT /api/v2/admin/subscription/groups/:id

# 删除分组
DELETE /api/v2/admin/subscription/groups/:id
```

#### 订阅模板管理

```http
# 获取分组下的模板
GET /api/v2/admin/subscription/groups/:id/templates

# 创建模板
POST /api/v2/admin/subscription/groups/:id/templates
{
    "name": "🇺🇸 美国节点",
    "type": "vless",
    "server": "us.example.com",
    "port": 443,
    "tls": 2,
    "transport": "tcp",
    "protocol_settings": "{\"flow\":\"xtls-rprx-vision\"}",
    "reality_public_key": "xxx",
    "enable": 1
}

# 更新模板
PUT /api/v2/admin/subscription/templates/:id

# 删除模板
DELETE /api/v2/admin/subscription/templates/:id
```

#### 权限分配

```http
# 为用户分配分组
POST /api/v2/admin/subscription/users/:user_id/groups
{
    "group_id": 2,
    "expire_at": 1735689600
}

# 移除用户分组
DELETE /api/v2/admin/subscription/users/:user_id/groups/:group_id

# 查看用户分组
GET /api/v2/admin/subscription/users/:user_id/groups

# 为套餐分配分组
POST /api/v2/admin/subscription/plans/:plan_id/groups
{
    "group_id": 2
}

# 移除套餐分组
DELETE /api/v2/admin/subscription/plans/:plan_id/groups/:group_id

# 查看套餐分组
GET /api/v2/admin/subscription/plans/:plan_id/groups
```

#### 工具接口

```http
# 获取支持的订阅格式
GET /api/v2/admin/subscription/formats

# 获取支持的协议类型
GET /api/v2/admin/subscription/protocols

# 预览用户订阅
POST /api/v2/admin/subscription/preview
{
    "user_id": 123,
    "format": "clash",
    "groups": [1, 2]
}
```

---

## 5. 支持的输出格式

| 格式 | 参数值 | 说明 | 适用客户端 |
|------|--------|------|------------|
| V2Ray Base64 | `v2ray` | Base64 编码的链接列表 | V2RayN, V2RayNG, V2Box, NekoRay |
| Clash YAML | `clash` | Clash 配置文件 | Clash, ClashX, Stash, Mihomo |
| Surge | `surge` | Surge 配置文件 | Surge iOS/macOS |
| Shadowrocket | `shadowrocket` | V2Ray 兼容格式 | Shadowrocket |
| Quantumult X | `quantumultx` | QX 专用格式 | Quantumult X |
| JSON | `json` | 原始 JSON 格式 | 自定义客户端 |
| Base64 JSON | `base64json` | Base64 编码的分组 JSON | 自定义 TLS 隧道 |

### Base64 JSON 格式详解 (自定义格式)

这是为自定义 TLS 隧道设计的特殊格式，按分组呈现配置：

```json
{
    "version": 1,
    "groups": {
        "default": [
            {
                "id": "node-1",
                "name": "🇺🇸 美国节点",
                "type": "vless",
                "server": "us.example.com",
                "port": 443,
                "uuid": "user-uuid-here",
                ...
            }
        ],
        "vip": [
            {
                "id": "node-2",
                "name": "🇯🇵 VIP 日本节点",
                ...
            }
        ]
    },
    "user": {
        "uuid": "user-uuid",
        "expired_at": 1735689600,
        "speed_limit": 10485760,
        "device_limit": 3,
        "transfer_enable": 107374182400,
        "used_traffic": 5368709120
    }
}
```

上述 JSON 会被 Base64 编码后返回。

---

## 6. 使用示例

### 6.1 基础配置流程

1. **创建订阅分组**

```bash
curl -X POST https://panel.example.com/api/v2/admin/subscription/groups \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"default","description":"默认分组","priority":0,"enable":1}'
```

2. **创建订阅模板**

```bash
curl -X POST https://panel.example.com/api/v2/admin/subscription/groups/1/templates \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "🇺🇸 美国 VLESS Reality",
    "type": "vless",
    "server": "us.example.com",
    "port": 443,
    "server_name": "www.microsoft.com",
    "tls": 2,
    "tls_fingerprint": "chrome",
    "transport": "tcp",
    "protocol_settings": "{\"flow\":\"xtls-rprx-vision\"}",
    "reality_public_key": "your-public-key",
    "reality_short_id": "abcd1234",
    "enable": 1
  }'
```

3. **关联套餐与分组**

```bash
curl -X POST https://panel.example.com/api/v2/admin/subscription/plans/1/groups \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"group_id": 1}'
```

4. **用户获取订阅**

```bash
# 用户 Token 从个人中心获取
curl https://panel.example.com/s/user-token-here
```

### 6.2 VIP 分组配置

```bash
# 1. 创建 VIP 分组
curl -X POST .../subscription/groups \
  -d '{"name":"vip","description":"VIP 专属高速节点","priority":10,"enable":1}'

# 2. 添加 VIP 节点模板
curl -X POST .../subscription/groups/2/templates \
  -d '{"name":"🚀 VIP 香港 Hysteria2","type":"hysteria2",...}'

# 3. 关联到高级套餐
curl -X POST .../subscription/plans/2/groups -d '{"group_id": 2}'

# 4. 或手动分配给特定用户
curl -X POST .../subscription/users/123/groups -d '{"group_id": 2}'
```

### 6.3 客户端订阅配置

**Clash / Stash:**
```
订阅地址: https://panel.example.com/s/your-token?type=clash
更新间隔: 24 小时
```

**V2RayN / V2RayNG:**
```
订阅地址: https://panel.example.com/s/your-token
(自动检测为 V2Ray Base64 格式)
```

**自定义 TLS 隧道客户端:**
```
订阅地址: https://panel.example.com/s/your-token?type=base64json
解码后获取分组配置 JSON
```

---

## 7. 安全注意事项

1. **订阅链接保密**: 用户 Token 应作为私密信息，不应公开分享
2. **Token 重置**: 用户可在个人中心重置 Token 以使旧链接失效
3. **HTTPS**: 订阅接口应始终使用 HTTPS
4. **权限隔离**: 不同分组的节点配置相互隔离，用户只能看到有权限的节点

---

## 8. 相关代码文件

| 文件 | 说明 |
|------|------|
| `internal/model/subscription.go` | 订阅系统数据模型 |
| `internal/parser/parser.go` | 解析器/格式化器接口定义 |
| `internal/parser/base64.go` | Base64 V2Ray 链接解析器 |
| `internal/parser/clash.go` | Clash YAML 解析器 |
| `internal/parser/formatter.go` | 多格式输出格式化器 |
| `internal/service/subscription_service.go` | 订阅服务核心逻辑 |
| `internal/handler/subscribe.go` | HTTP 处理器 |
| `internal/service/init_subscription.go` | 默认数据初始化 |
