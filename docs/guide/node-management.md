# 节点管理

## 概述

V2Board AnixOps 采用新一代节点管理架构，支持：
- 节点自动注册与发现
- 多协议配置（单节点多协议）
- 实时心跳监控
- 流量统计

## 架构设计

### 密钥层次结构

```
┌─────────────────────────────────────────────────────────────────┐
│  授权密钥 (AuthorizedKey)                                        │
│  - 一次性使用，用于节点首次注册                                    │
│  - 管理员在后台生成，部署时配置到节点                               │
│  - 注册成功后自动失效                                             │
└─────────────────────────────────────────────────────────────────┘
                              ↓ 注册成功后获得
┌─────────────────────────────────────────────────────────────────┐
│  节点凭证 (api_key + secret)                                     │
│  - 每个节点独立一组                                               │
│  - 用于后续所有 API 通信 (心跳、上报等)                            │
│  - 本地持久化存储                                                 │
│  - 可在管理后台撤销/重置                                          │
└─────────────────────────────────────────────────────────────────┘
```

### 注册流程

```
┌──────────────┐                         ┌──────────────┐
│   节点客户端   │                         │   V2Board    │
└──────┬───────┘                         └──────┬───────┘
       │                                        │
       │  1. 检查本地是否有凭证                   │
       │  ┌───────────────────┐                 │
       │  │ 有凭证 → 跳到步骤4   │                 │
       │  │ 无凭证 → 继续步骤2   │                 │
       │  └───────────────────┘                 │
       │                                        │
       │  2. POST /api/v2/node/register        │
       │  {auth_key, name, host, port, ...}    │
       │ ─────────────────────────────────────►│
       │                                        │
       │  3. 返回节点凭证                        │
       │  {node_id, api_key, secret}           │
       │◄───────────────────────────────────── │
       │                                        │
       │  4. 保存凭证到本地文件                   │
       │                                        │
       │  5. GET /api/v2/server/UniProxy/config │
       │ ─────────────────────────────────────►│
       │                                        │
       │  6. 定时心跳 (每60秒)                   │
       │ ─────────────────────────────────────►│
       │                                        │
```

---

## 节点状态

节点状态通过 `last_check_at` 字段判断：

| 状态 | 条件 | 说明 |
|------|------|------|
| 在线 | `last_check_at > now - 300s` | 5分钟内有心跳 |
| 离线 | `last_check_at <= now - 300s` | 超过5分钟无心跳 |
| 待激活 | `status = 0` | 新注册但未配置协议 |

心跳更新触发点：
- UniProxy `/push` 接口（流量上报）
- UniProxy `/alive` 接口（在线状态上报）
- `/api/v2/node/heartbeat` 接口

---

## 协议配置

### 支持的协议类型

| 协议 | 标识 | 说明 |
|------|------|------|
| VMess | `vmess` | V2Ray VMess |
| VLESS | `vless` | V2Ray VLESS |
| Trojan | `trojan` | Trojan |
| Shadowsocks | `shadowsocks` | Shadowsocks |
| Hysteria2 | `hysteria2` | Hysteria2 |
| TUIC | `tuic` | TUIC |
| AnyTLS | `anytls` | AnyTLS |

### 配置优先级

节点配置按以下优先级解析：

1. **RawConfig** - 原始 JSON 配置（管理员高级模式）
2. **协议配置** - 从 `v2_node_protocol` 表构建
3. **默认配置** - 最小化启动配置

### 协议配置示例

```json
{
  "node_type": "vless",
  "server_port": 443,
  "host": "node1.example.com",
  "server_name": "node1.example.com",
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

---

## 数据库表结构

### v2_node (节点表)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| name | string | 节点名称 |
| host | string | 节点地址 |
| port | int | API 端口 |
| group_id | uint | 节点分组 |
| status | int | 状态: 0=待激活, 1=在线 |
| last_check_at | int64 | 最后心跳时间戳 |
| total_upload | int64 | 总上传流量 |
| total_download | int64 | 总下载流量 |
| api_key_hash | string | API Key 哈希 |
| secret_hash | string | Secret 哈希 |

### v2_node_protocol (协议表)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| node_id | uint | 关联节点 |
| name | string | 协议名称 |
| type | string | 协议类型 |
| port | int | 监听端口 |
| enable | int | 是否启用 |
| tls | int | TLS 模式 |
| transport | string | 传输层协议 |
| settings | json | 协议配置 |
| tls_settings | json | TLS 配置 |
| transport_settings | json | 传输层配置 |

### v2_authorized_key (授权密钥表)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| key_hash | string | 密钥哈希 |
| name | string | 备注名称 |
| used | int | 是否已使用 |
| used_by_node_id | uint | 使用该密钥的节点 |
| expires_at | time | 过期时间 |

---

## 管理操作

### 生成授权密钥

```bash
POST /api/v2/admin/auth-keys
{
  "name": "Tokyo Node Key",
  "expires_at": "2025-12-31T23:59:59Z"
}
```

### 查看节点列表

```bash
GET /api/v2/admin/nodes?page=1&size=20
```

### 添加协议配置

```bash
POST /api/v2/admin/nodes/1/protocols
{
  "name": "VLESS-Reality",
  "type": "vless",
  "port": 443,
  "tls": 2,
  "transport": "tcp",
  "settings": "{\"flow\":\"xtls-rprx-vision\"}",
  "tls_settings": "{\"server_name\":\"www.google.com\",\"public_key\":\"...\"}"
}
```

---

## 故障排查

### 节点显示离线但实际已连接

**原因**: UniProxy 接口未更新 `last_check_at`

**解决**: 确保后端代码在 `/push` 和 `/alive` 接口中调用 `UpdateLastCheckAt`

### 节点注册失败

**常见原因**:
1. 授权密钥已使用或过期
2. 网络连接问题
3. API 路径错误（应使用 `/api/v2`）

### 协议配置不生效

**检查**:
1. 协议是否已启用 (`enable = 1`)
2. 配置 JSON 格式是否正确
3. 节点是否已重新拉取配置
