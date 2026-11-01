# 节点管理

## 概述

AnixOps Control 采用 Agent-first 节点管理架构，支持：
- 节点通过一次性注册凭据接入，之后只走 mTLS 控制流
- 多协议配置（单节点多协议）
- 实时在线状态
- 流量统计

节点接入（节点组、接入令牌、审批、证书）见 [Agent 接入](agent-onboarding.md)。
Agent 只通过 `anix.agent.v1` mTLS gRPC 控制流与 Control 通信；UniProxy、
`v2board` gRPC 服务和旧的注册/心跳 HTTP 接口都已删除，V2bX、XrayR 等第三方
节点程序无法再接入。

---

## 节点状态

节点在线状态来自它的控制流：

| 状态 | 条件 | 说明 |
|------|------|------|
| 在线 | 有控制流会话，或 `last_check_at > now - 300s` | 5 分钟内有心跳 |
| 离线 | `last_check_at <= now - 300s` | 超过 5 分钟无心跳 |
| 待激活 | `status = 0` | 新接入但未配置协议 |

`anix-control agents transports` 列出每个节点的连接方式。

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
| Panabit iWAN | `iwan` | UDP 三层隧道，不加密，见下文 |

### Panabit iWAN

`iwan` 协议由 anix-agent 的 anixcore 内核提供（networkcore_anixops `docs/protocols/iwan.md` §12），接入 iwand、OpenWrt、Panabit 等 iWAN 客户端。用户的流量进入节点后和其他协议一样走「转发 → 分流」的 egress 路由。**iWAN 没有加密和完整性校验**，只适合可信线路；管理端表单会标出这一点。

协议设置（`settings`）：

| 字段 | 默认 | 说明 |
|------|------|------|
| `pool` | `10.66.0.0/16` | 用户地址池，IPv4 /8 到 /30，第一个地址是网关（ping 和 DNS） |
| `mtu` | `1400` | 576 到 1600 |
| `dns_upstream` | `1.1.1.1` | 网关 DNS 转发的上游，IP 或 IP:端口 |
| `allow_encrypt` | `true` | 是否允许客户端请求 XOR 掩码（不是加密） |
| `allow_client_to_client` | `false` | 用户之间能否互访 |
| `fixed_addresses` | `{}` | 面板用户 id → 固定地址，须在地址池内、不是网关，且不能重复 |

用户的登录名是面板用户 id，口令是 UUID 去掉连字符后的前 16 位（小写）。用户在订阅页选择 iWAN 格式（`/s/<token>?type=iwan`）得到 iwand 的配置文件，每个 iWAN 节点一节（服务器、端口、用户名、口令、MTU、加密），固定地址写在注释里；OpenWrt 和 Panabit 客户端按其中的值手工填写。iWAN 节点不出现在其他订阅格式里；只有 iWAN 节点的订阅在 V2Ray / Shadowrocket 格式下会改给 iWAN 格式。

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

应答中的 `key` 只返回这一次，请当场复制（节点页「授权密钥」弹窗中的「生成密钥」同理）。
`GET /api/v2/admin/auth-keys` 中的 `key` 显示为 `********`。

### 查看节点列表

```bash
GET /api/v2/admin/nodes?page=1&size=20
GET /api/v2/admin/nodes?sort=last_check_at&order=desc   # 服务端排序
```

管理端「节点」页的表头排序由服务端完成（`sort` 可取 `id`、`name`、`host`、`cpu_usage`、`last_check_at` 等，
见 `docs/guide/api-reference.md`），排序写进页面 URL（`?sort=&order=`）；显示状态由最近心跳推算、协议数来自另一张表，
这两列不能排序。列表还显示每个节点 Agent 的连接方式（mTLS 流 / API 密钥流 / 旧版 / 第三方 / 离线）和证书状态，
节点页「概览」里有「Agent 连接」分组（连接方式、最近出现、证书到期与续期时间、吊销时间和原因）。

「服务状态」列显示机器遥测插件上报的 systemd 服务汇总：全部运行为「3/3 正常」，有失败为「N 个失败」，
没有失败但不全在运行为「2/3 运行中」，另有「无服务」「等待上报」「不支持」「数据过期」；悬停可看计数和上报时间。
列表加载后对当前页的节点发一次 `GET /api/v3/plugins/machine-telemetry/nodes/services?ids=…` 请求。
未开启服务采集的节点显示「—」；机器遥测包的宿主没有运行（系统设置 → 内置包）
或管理员没有 `machine-telemetry.api` 权限时，整列显示「—」，列表照常使用。这一列可以在表格设置里隐藏。
服务采集按节点在节点页「服务」分区开启，见 [`../architecture/package-reports.md`](../architecture/package-reports.md)。

节点页的「流量」分区画这个节点上传和下载随时间的变化（`GET /api/v4/kernel/nodes/:id/traffic`）：
24 小时、7 天、30 天按小时，90 天、1 年按日；上方是该范围的上传、下载、合计，图下可以切换成数据表。
「流量与监控 → 实时节点」里点一行（或行菜单的「查看流量」）会在侧边栏里显示同一张图。小时数据来自流量日志，
只保留到日志的保留期限（见 [`../reference/traffic-stats-operations.md`](../reference/traffic-stats-operations.md)）；
日数据来自每日统计，不受清理影响。

节点页的「诊断」分区（取代已删除的 NodeX Agents 页）在这台机器的 Agent 上运行允许的检查，走 Agent 控制流，
结果直接显示在页面上：「查看服务状态」「查看日志」（最近 100 / 500 / 1000 行）和「重启服务」（需要确认，
仅超级管理员）。目前允许的服务只有 `gost`。Agent 不在线，或版本不支持诊断时，分区只说明原因，不提供按钮。
转发节点的页面里同一个分区还有四项转发检查（监听、端口冲突、TCP 连接、UDP 探测），选一条这个节点承载的跳来检查。
每一次运行都记入操作日志（module `node_diagnostic`）；接口见 [API 参考](api-reference.md#节点诊断)。

节点页「凭据」分区的「轮换 Agent 凭据」用于节点身份可能泄露的时候（硬盘被盗、密钥留在构建机上）：
它吊销这个节点 Agent 持有的所有证书、注册记录和转发链路证书，签发一个一次性凭据并只显示一次，
节点离线也能轮换。仅超级管理员；停用的节点要先启用。不勾选「同时更换节点的 API 密钥」时，旧密钥仍然有效，
持有它的人仍然可以让 Agent 自行重新注册；勾选后所有仍用旧密钥轮询的进程都会失效，直到换上新密钥
（在同一分区「读取 API 密钥」）。步骤见 [Agent 安装](agent-onboarding.md#rotating-a-nodes-credentials)。

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

### 多入口节点（多线路、IPv6 进 IPv4 出、厂商 NAT）

有些机器不止一个地址能连：例如上海的中转机有 CN2 和 9929 两条线，每条线有机器自带的
原生 IPv6，IPv4 则是厂商用 NAT 映射过来的公网地址（端口不同，要手动填）。这种节点在
节点页「协议」标签下的「入口」里，为每个地址加一个入口：

- **直连**：用户直接连这个地址，端口与协议端口相同（机器自带的 IPv6）。
- **NAT 映射**：厂商把公网地址的端口映射到机器上。端口映射每行一段，公网端口在前：
  `21000-21099 -> 30000-30099`、`20443 -> 443`。

节点有启用的入口时，订阅里每个协议按入口各出一条，名字是 `<节点> - <协议> | <入口>`，
地址和端口用入口的；NAT 入口没映射到的协议端口，这个入口下不出节点。没有入口的节点不变。
启用的协议若没有任何启用的入口覆盖它的端口，订阅里就完全没有它（协议仍显示在线），
「入口」上方会给出警告并列出这些协议；接口的 `uncovered_protocols` 也列出它们。
节点本身的配置不变，Agent（或者没有 Agent 的中转机上的转发程序）照旧监听协议端口。

IPv6 进、IPv4 出的中转机，转发不能用 nftables DNAT（它不能跨 IPv4/IPv6），要用 gost、
realm 这类用户态转发程序监听 `[::]`，再连 IPv4 的落地节点。

```bash
PUT /api/v4/kernel/nodes/proxy-60/entries
{
  "entries": [
    {"label": "CN2 v6", "kind": "direct", "host": "2001:db8::10"},
    {"label": "CN2 NAT", "kind": "nat", "host": "203.0.113.7",
     "port_map": [{"from": 30000, "to": 30099, "external": 21000}]}
  ]
}
```

整个列表一次替换（最多 32 个，空列表表示删掉全部入口），写入操作日志（模块 `node_entry`）。

---

## 故障排查

### 节点显示离线但实际已连接

**检查**: `anix-control agents transports` 中该节点是否为 `mtls`；不是则 Agent 没有用证书连上控制流，
查看 Agent 日志中的 `agent_cert_*` 拒绝码。

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
