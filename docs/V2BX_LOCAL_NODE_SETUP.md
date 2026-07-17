# Legacy V2bX 本机节点接入链路（2.x 归档）

> 本文保留旧 V2bX/UniProxy 调试步骤用于兼容排障。v3 新部署使用
> [AnixOps Agent](https://github.com/AnixOps/anix-agent) 与
> [AnixOps Control](https://github.com/AnixOps/anix-control)，不要照搬本文的旧二进制名和路径。

## 仓库信息

| 项目 | 仓库地址 | 分支 |
|------|----------|------|
| AnixOps Control | https://github.com/AnixOps/anix-control | `go_dev` |
| AnixOps Agent（旧 V2bX 兼容） | https://github.com/AnixOps/anix-agent | `dev_new` |
| 测速工具 | https://github.com/AnixOps/AnixOps-speedtest | `main` |
| Ansible IaC | https://github.com/AnixOps/AnixOps-ansible | `main` |

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          本机开发环境 (Windows)                              │
│                                                                             │
│  ┌──────────────┐          ┌──────────────┐         ┌─────────────────────┐ │
│  │  用户客户端    │ ──────▶ │   面板        │         │  V2bX 节点           │ │
│  │  (Mihomo)    │  HTTP   │  :8080        │◀───────▶│  监听 0.0.0.0:9000  │ │
│  │  +Speedtest  │          │  SQLite       │  AutoReg│  VMess/TCP          │ │
│  └──────────────┘          └──────────────┘  +Heart  └─────────────────────┘ │
│         │                        │                              ▲             │
│         │ GET /s/{token}         │ GET /UniProxy/user           │ 代理流量     │
│         │ (订阅)                  │ POST /UniProxy/push          │ (出站)      │
│         │                        │ GET  /UniProxy/alive         │             │
│         └────────────────────────┴──────────────────────────────┘             │
│                         vmess:// 配置 + 真实代理流量                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 完整数据链路

### 1. 订阅生成链路 (用户 → 节点配置)

```
GET /s/{user.token}
  │
  ▼
Subscribe handler
  │
  ├─ user.token → 查找 User (ID, UUID, transfer_enable)
  │
  ├─ User → UserSubscriptionGroup → SubscriptionGroup (ID=4)
  │
  ├─ SubscriptionGroup → SubscriptionGroupNodeProtocols → NodeProtocol
  │     └─ protocol.type, protocol.port, protocol.transport
  │
  ├─ NodeProtocol → Node (host, name, status)
  │
  ├─ SubscriptionGroup → SubscriptionTemplate
  │
  └─ 拼接 vmess:// 链接返回
```

### 2. V2bX ↔ 面板通信链路

```
V2bX 启动
  │
  ├─ POST /api/v2/node/register (AuthKey) → 返回 api_key, secret, node_id
  │
  ├─ GET /api/v2/server/UniProxy/config (X-API-Key + node_id) → 返回节点配置
  │     └─ { type, host, port, network, tls, ... }
  │
  ├─ GET /api/v2/server/UniProxy/user (X-API-Key + node_id) → 返回用户列表
  │     └─ { users: [{ id, uuid, speed_limit, device_limit }] }
  │
  ├─ 加载用户到 Xray Core (AddUsers)
  │
  ├─ 开始监听 inbound (0.0.0.0:9000)
  │     └─ 用户连接 → VMess 解码 → 验证 UUID → 代理出站
  │
  ├─ POST /api/v2/server/UniProxy/push → 上报用户流量
  │     └─ { uid: [upload, download] }
  │
  └─ GET /api/v2/server/UniProxy/alivelist → 心跳保活
```

### 3. 用户有效性判定 (GetActiveUsersForNode)

V2bX 拉取用户列表时，面板执行以下 SQL 过滤（`user_service.go:90`）：

```sql
SELECT * FROM v2_user
WHERE banned = 0
  AND (expired_at IS NULL OR expired_at > NOW)
  AND (upload + download) < transfer_enable     -- 关键：流量未用完
  AND group_id = ?                               -- 关键：匹配节点分组
```

**任何一个条件不满足，用户都会被过滤掉**，V2bX 会返回空用户列表，所有代理连接报 `user do not exist`。

## 完整操作记录 (2026-04-29)

### Step 1: 启动面板

```bash
cd C:\Users\z7299\Documents\GitHub\v2board_AnixOps

# 确保 config/config.yaml 已配置 api_token
go build -o build/v2board.exe -trimpath ./cmd/server
rm -f config/data/v2board.db
./build/v2board.exe -config config/config.yaml
```

验证：
```
$ curl -s http://localhost:8080/health
{"status":"ok"}
```

### Step 2: 生成 AuthKey

```bash
curl -s -X POST http://localhost:8080/api/v2/internal/auth-keys \
  -H "Content-Type: application/json" \
  -H "X-API-Key: internal-api-token-for-v2bx-2026" \
  -d '{"name":"local-v2bx-node"}'
```

返回：
```json
{"data":{"id":1,"key":"9199ff27...71cf","key_hash":"sha256..."}}
```

### Step 3: 构建 V2bX

```bash
cd C:\Users\z7299\Documents\GitHub\V2bX_AnixOps

# 修复 qpack 依赖冲突
go get github.com/sagernet/quic-go@latest

# 构建（必须加 xray build tag，否则没有核心）
GOEXPERIMENT=jsonv2 go build -tags xray -o build/V2bX.exe -trimpath .
```

### Step 4: 创建 V2bX 配置

```json
{
  "Log": { "Level": "debug" },
  "Cores": [{
    "Type": "xray",
    "Log": { "Level": "debug", "Timestamp": true },
    "NTP": { "Enable": true, "Server": "time.apple.com" }
  }],
  "Nodes": [{
    "Core": "xray",
    "ApiHost": "http://127.0.0.1:8080",
    "AutoRegister": true,
    "AuthKey": "9199ff277f04a131e612a8cdb92e4169edcab99ff1d69145773f8e79142271cf",
    "NodeName": "local-v2bx-node",
    "NodeHost": "127.0.0.1",
    "NodePort": 9000,
    "CredentialFile": "data/credential.json",
    "HeartbeatInterval": 30,
    "EnableSign": true,
    "EncryptCredential": true,
    "CertConfig": { "CertMode": "none" }
  }]
}
```

### Step 5: 启动 V2bX

```bash
cd C:\Users\z7299\Documents\GitHub\V2bX_AnixOps
rm -f data/credential.json.enc   # 清除旧凭据
GOEXPERIMENT=jsonv2 ./build/V2bX.exe -c config.local.json
```

启动日志：
```
Auto register mode enabled
Registration successful (encrypted): NodeID=2
Xray Core Version: 25.12.1
No users found for this node, will continue running    ← 空用户列表
```

### Step 6: 通过面板 API 完成数据链路

```bash
# 管理员登录
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/api/v2/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.invalid","password":"__REDACTED_EXAMPLE_PASSWORD__"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

# 注册测试用户
curl -s -X POST http://localhost:8080/api/v2/register \
  -H "Content-Type: application/json" \
  -d '{"email":"testuser@local.dev","password":"TestPass123!"}'
# → user_id=2

# 创建套餐 (10GB)
curl -s -X POST http://localhost:8080/api/v2/admin/plans \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name":"Local Test Plan","content":"Monthly 10GB","renew_price":29.9,"transfer_enable":10737418240,"device_limit":3,"speed_limit":0,"show":1,"sell_duration":30,"renew_method":"on","with_package":0,"sort":1}'
# → plan_id=2

# 创建订阅分组
curl -s -X POST http://localhost:8080/api/v2/admin/subscription/groups \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name":"Local Test Group","show":1}'
# → group_id=4

# 用户关联分组
curl -s -X POST http://localhost:8080/api/v2/admin/subscription/users/2/groups \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"group_id":4}'

# 套餐关联分组
curl -s -X POST http://localhost:8080/api/v2/admin/subscription/plans/2/groups \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"group_id":4}'

# 节点协议关联分组
PROTOCOL_ID=$(curl -s http://localhost:8080/api/v2/admin/nodes/2/protocols \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  | python -c "import sys,json; print(json.load(sys.stdin)['data'][0]['id'])")

curl -s -X POST http://localhost:8080/api/v2/admin/subscription/groups/4/protocols \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"protocol_ids\":[$PROTOCOL_ID]}"
```

### Step 7: 修复数据库 (关键排障步骤)

**问题发现**：V2bX 日志显示 `No users found for this node`，所有代理连接报 `user do not exist`。

**排查过程**：

```bash
# 1. 直接测试 UniProxy 用户接口
API_KEY=$(sqlite3 config/data/v2board.db \
  "SELECT api_key FROM v2_node WHERE id = 2;")

curl -s "http://localhost:8080/api/v2/server/UniProxy/user?node_id=2" \
  -H "X-API-Key: $API_KEY"
# → {"users":[]}   ← 空！

# 2. 查数据库找到根因
python -c "
import sqlite3
conn = sqlite3.connect('config/data/v2board.db')
cur = conn.cursor()
cur.execute('SELECT id, email, group_id, transfer_enable FROM v2_user WHERE id = 2')
u = cur.fetchone()
print(f'User: id={u[0]} group_id={u[2]} transfer_enable={u[3]}')
cur.execute('SELECT id, name, group_id FROM v2_node WHERE id = 2')
n = cur.fetchone()
print(f'Node: id={n[0]} group_id={n[2]}')
"
# → User: id=2 group_id=None transfer_enable=0    ← 根因
# → Node: id=2 group_id=None                       ← 根因
```

**根因分析**：
- `transfer_enable = 0` → `(0+0) < 0` 为 false，用户被 SQL 过滤
- `user.group_id = NULL` → 与节点分组不匹配
- `node.group_id = NULL` → 未关联到订阅分组

**修复**：

```bash
python -c "
import sqlite3
conn = sqlite3.connect('config/data/v2board.db')
cur = conn.cursor()
cur.execute('UPDATE v2_user SET transfer_enable = 10737418240, group_id = 4 WHERE id = 2')
cur.execute('UPDATE v2_node SET group_id = 4 WHERE id = 2')
conn.commit()
conn.close()
"
```

### Step 8: 重启 V2bX 验证

```bash
# 杀掉旧进程，重启
GOEXPERIMENT=jsonv2 ./build/V2bX.exe -c config.local.json
```

成功日志：
```
Loaded encrypted credential: NodeID=2, APIKey=5954****49d7
Raw node config response: {"type":"vmess","server_port":9000,...}
listening TCP on 0.0.0.0:9000
Added 1 new users                                          ← 用户加载成功
Total 1 online users
```

### Step 9: 获取订阅链接

```bash
USER_SUB_TOKEN=$(curl -s http://localhost:8080/api/v2/user/profile \
  -H "Authorization: Bearer $USER_TOKEN" \
  | python -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

curl -s http://localhost:8080/s/$USER_SUB_TOKEN | base64 -d
```

返回：
```
vmess://eyJhZGQiOiIxMjcuMC4wLjEiLCJhaWQiOjAsImlkIjoiOGUzMWI4MzktNTBlYS00ZDdkLTk3ZGEtNzNiOTU2YzhjNjk0IiwibmV0IjoidGNwIiwicG9ydCI6OTAwMCwicHMiOiJsb2NhbC12MmJ4LW5vZGUgLSBEZWZhdWx0IFZNZXNzIiwic2N5IjoiYXV0byIsInRscyI6IiIsInR5cGUiOiJub25lIiwidiI6IjIifQ==
```

### Step 10: AnixOps-speedtest 连通性验证

```bash
cd C:\Users\z7299\Documents\GitHub\AnixOps-speedtest

cat > test_local.yaml << 'EOF'
proxies:
  - alterId: 0
    cipher: auto
    name: local-v2bx-node
    port: 9000
    server: 127.0.0.1
    type: vmess
    uuid: 8e31b839-50ea-4d7d-97da-73b956c8c694
    network: tcp
    skip-cert-verify: true
EOF

./speedtest.exe -f test_local.yaml -no-ats -no-sign -format table
```

**实测结果**：

```
#    Name                  Type    HTTP(ms)  TCP(ms)   Speed     Unlocks                                          IP Risk    DNS Region
---------------------------------------------------------------------------------------------------------------------------------------------------
1    local-v2bx-node       vmess   91        1         201.0     Netflix,YouTube Premium,Disney+,OpenAI,           15(low)    GB
                                                                Spotify,Tiktok,Wikipedia
```

| 指标 | 值 | 说明 |
|------|-----|------|
| HTTP 延迟 | 91ms | 通过代理访问微软/谷歌的延迟 |
| TCP 延迟 | 1ms | 本机回环 TCP 握手 |
| 下载速度 | 201.0 MB/s | 本机回环链路 |
| IP 风险 | 15 (low) | 机房 IP |
| DNS 地区 | GB | Mihomo 内建 DNS 解析 |
| 流媒体解锁 | 全通过 | 7 个平台 |

**V2bX 侧日志确认代理流量经过**：

```
received request for tcp:www.msftconnecttest.com:80
connection opened to tcp:www.msftconnecttest.com:80, local endpoint 192.168.42.143
Total 1 online users, 0 Reported
```

## 已知问题与解决方案

### 1. 用户流量/分组配置缺失 (关键排障记录)

**现象**：V2bX 启动成功，但日志显示 `No users found`，代理连接报 `user do not exist`。

**排查路径**：
1. 直接调用 `GET /api/v2/server/UniProxy/user?node_id=2` 返回空数组
2. 查询数据库发现 `v2_user.transfer_enable = 0` 且 `v2_user.group_id = NULL`
3. 过滤条件 `(u+d) < transfer_enable` 和 `group_id = ?` 同时拦截

**修复**：
```sql
UPDATE v2_user SET transfer_enable = 10737418240, group_id = 4 WHERE id = 2;
UPDATE v2_node SET group_id = 4 WHERE id = 2;
```

**预防**：创建套餐后购买会自动分配流量；或通过 API 购买套餐后再关联用户。

### 2. V2bX WebSocket 401 回退到轮询模式

**现象**：`websocket: bad handshake` → `fallback polling mode`

**原因**：WebSocket 认证路径 `/api/v2/agent/ws` 和 `/api/v2/node/ws` 需要额外的认证逻辑

**影响**：无功能性影响，轮询模式完整支持获取配置、上报流量、心跳

### 3. Go 依赖冲突 (qpack/sing-box)

**现象**：`too many arguments in call to qpack.NewDecoder`

**原因**：`sagernet/quic-go@v0.55.0-sing-box-mod.2` 与 `qpack v0.5.1` API 不兼容

**解决**：`go get github.com/sagernet/quic-go@latest`

### 4. V2bX 构建缺少核心

**现象**：`unknown core type`

**原因**：构建时未加 build tag，`core/imports/xray.go` 有 `//go:build xray` 条件编译

**解决**：`go build -tags xray ...`

## 配置文件

### 面板 config/config.yaml

```yaml
env: "development"
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"
database:
  driver: "sqlite"
  database: "config/data/v2board.db"
app:
  api_token: "internal-api-token-for-v2bx-2026"   # V2bX 内部 API 认证
  subscribe_path: "s"
admin:
  email: "admin@example.invalid"
  password: "__REDACTED_EXAMPLE_PASSWORD__"
```

### V2bX config.local.json

```json
{
  "Log": { "Level": "debug" },
  "Cores": [{
    "Type": "xray",
    "Log": { "Level": "debug", "Timestamp": true }
  }],
  "Nodes": [{
    "Core": "xray",
    "ApiHost": "http://127.0.0.1:8080",
    "AutoRegister": true,
    "AuthKey": "<生成>",
    "NodeName": "local-v2bx-node",
    "NodeHost": "127.0.0.1",
    "NodePort": 9000,
    "CredentialFile": "data/credential.json",
    "HeartbeatInterval": 30,
    "EnableSign": true,
    "EncryptCredential": true,
    "CertConfig": { "CertMode": "none" }
  }]
}
```

### Speedtest test_local.yaml

```yaml
proxies:
  - alterId: 0
    cipher: auto
    name: local-v2bx-node
    port: 9000
    server: 127.0.0.1
    type: vmess
    uuid: "<从订阅解码获取>"
    network: tcp
    skip-cert-verify: true
```

## 关键 API 端点

### 面板端 (v2board)

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | `/health` | 健康检查 | 无 |
| POST | `/api/v2/register` | 用户注册 | 无 |
| POST | `/api/v2/login` | 用户登录 | 无 |
| GET | `/s/{token}` | 获取订阅 | 用户 token |
| GET | `/api/v2/user/profile` | 用户信息 | Bearer JWT |
| POST | `/api/v2/internal/auth-keys` | 生成 AuthKey | X-API-Key |
| GET | `/api/v2/admin/nodes` | 节点列表 | Bearer JWT (admin) |
| POST | `/api/v2/admin/nodes` | 创建节点 | Bearer JWT (admin) |
| GET | `/api/v2/admin/nodes/{id}/protocols` | 节点协议列表 | Bearer JWT (admin) |
| POST | `/api/v2/admin/subscription/groups` | 创建订阅分组 | Bearer JWT (admin) |
| POST | `/api/v2/admin/subscription/groups/{id}/protocols` | 关联协议到分组 | Bearer JWT (admin) |
| POST | `/api/v2/admin/subscription/users/{uid}/groups` | 关联用户到分组 | Bearer JWT (admin) |
| POST | `/api/v2/admin/subscription/plans/{pid}/groups` | 关联套餐到分组 | Bearer JWT (admin) |

### V2bX ↔ 面板 (UniProxy)

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/api/v2/node/register` | 节点自动注册 | AuthKey |
| GET | `/api/v2/server/UniProxy/config` | 获取节点配置 | X-API-Key + node_id |
| GET | `/api/v2/server/UniProxy/user` | 获取用户列表 | X-API-Key + node_id |
| POST | `/api/v2/server/UniProxy/push` | 上报流量 | X-API-Key + node_id |
| GET | `/api/v2/server/UniProxy/alivelist` | 心跳在线数 | X-API-Key + node_id |

## Ansible 集成 (流量转发)

面板通过 Ansible 在远端中继节点上管理 nftables 转发规则，实现流量的动态 NAT 转发。

### 架构关系

```
┌──────────────────────────────────────────────────────────────┐
│                        面板 (v2board)                         │
│                                                              │
│  管理员创建转发规则 ──▶ forward_runtime backend               │
│                          │                                   │
│                          ▼                                   │
│                   ansible-playbook                           │
│                          │                                   │
│                    SSH (22)                                  │
└──────────────────────────┼───────────────────────────────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │    中继节点 (Linux)      │
              │   nftables v2b_forward  │
              │   prerouting/postrating │
              │   nat chain             │
              └────────────────────────┘
```

### 相关仓库

| 项目 | 仓库地址 | 分支 |
|------|----------|------|
| AnixOps Control | https://github.com/AnixOps/anix-control | `go_dev` |
| Ansible IaC | https://github.com/AnixOps/AnixOps-ansible | `main` |

**AnixOps-ansible** 是独立的 GitOps IaC 仓库，用于全球分布式服务器集群的基础设施管理（基础配置、监控、网络、安全）。面板内置的 Ansible playbooks 是轻量级运行时路径，专门处理 nftables 转发规则的动态创建/删除。

### 面板配置

在 `config/config.yaml` 中配置转发运行时：

```yaml
forward_runtime:
  backend: "nftables_ansible"  # 或 "iptables_ansible"
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
    become: true  # 非 root SSH 用户时启用 sudo
```

### 快速设置步骤

**1. 创建 inventory 文件**

```bash
cd C:\Users\z7299\Documents\GitHub\v2board_AnixOps
cp config/deploy/ansible/inventory.ini.example config/deploy/ansible/inventory.ini
```

**2. 编辑 inventory.ini**

```ini
[forward_nodes]
relay-01 ansible_host=203.0.113.10 ansible_user=root ansible_port=22 ansible_ssh_private_key_file=/path/to/ssh/key
relay-02 ansible_host=203.0.113.11 ansible_user=deploy ansible_port=2222 ansible_ssh_pass=your_password
```

**3. 放置 SSH 密钥**

将 SSH 私钥放在 `config/deploy/ssh/` 目录下（可选，inventory 中也可指定路径）：

```bash
cp /path/to/your/id_ed25519 config/deploy/ssh/
chmod 600 config/deploy/ssh/id_ed25519
```

**4. 验证 Ansible 连通性**

```bash
cd config/deploy/ansible
ANSIBLE_CONFIG=./ansible.cfg ansible all -m ping -i inventory.ini
```

期望返回每个节点的 `pong` 响应。

**5. 确认 Ansible 配置**

`config/deploy/ansible/ansible.cfg` 默认配置：

```ini
[defaults]
inventory = ./inventory.ini
host_key_checking = False
retry_files_enabled = False
interpreter_python = auto_silent
stdout_callback = default
timeout = 20

[ssh_connection]
pipelining = True
```

### Playbook 行为

**forward_apply_nftables.yml**（创建转发规则）：

- 在远端节点创建 `nft add table inet v2b_forward`
- 创建 prerouting/postrating NAT 链
- 根据协议类型（tcp/udp/both）添加 dnat/snat 规则
- 根据负载均衡策略（fifo/round/rand）分配目标：
  - `fifo`: 按顺序选择第一个可用目标
  - `round`: 使用 `nft numgen inc` 轮询分配
  - `rand`: 使用 `nft numgen rand` 随机分配

**forward_remove_nftables.yml**（删除转发规则）：

- 删除对应的 nftables 链和规则
- 清理 `v2b_forward` table 中的条目

### 运行时配置 (非标准路径)

如果使用 Docker 或非标准部署路径，通过 `runtime-config.json` 覆盖默认路径：

```json
{
  "inventory": "/app/config/deploy/ansible/inventory.ini",
  "playbookApply": "/app/config/deploy/ansible/playbooks/forward_apply_nftables.yml",
  "playbookRemove": "/app/config/deploy/ansible/playbooks/forward_remove_nftables.yml",
  "workingDir": "/app/config/deploy/ansible",
  "targetPattern": "{{node.host}}",
  "timeoutSeconds": 120,
  "become": true,
  "environment": {
    "ANSIBLE_CONFIG": "/app/config/deploy/ansible/ansible.cfg",
    "ANSIBLE_HOST_KEY_CHECKING": "False"
  }
}
```

### 密码认证

如果不使用 SSH 密钥，可以在 inventory 中直接指定密码：

```ini
relay-01 ansible_host=203.0.113.10 ansible_user=root ansible_ssh_pass=YourPassword123
```

或在 `extra_vars` 中传递。

### 与 AnixOps-ansible 的关系

面板内置的 Ansible 运行时是轻量级的转发规则管理器。如果需要更完整的基础设施管理（系统基线配置、Prometheus/Grafana 监控、安全加固、网络优化等），可以使用 [AnixOps-ansible](https://github.com/AnixOps/AnixOps-ansible) 仓库中的角色和 playbook 进行批量节点初始化。

典型工作流：

1. 使用 AnixOps-ansible 初始化中继节点（系统配置、安全策略、nftables 基础规则）
2. 面板通过内置 Ansible 运行时动态创建/删除具体转发规则
3. 两者互补：AnixOps-ansible 管基础设施，面板管业务规则

## 验证清单

| 检查项 | 命令 | 期望结果 |
|--------|------|----------|
| 面板健康 | `curl localhost:8080/health` | `{"status":"ok"}` |
| V2bX 监听 | `netstat -ano \| findstr :9000` | LISTENING |
| 用户 API | `GET /UniProxy/user?node_id=N` | `{"users":[{"id":2,"uuid":"..."}]}` |
| 订阅返回 | `GET /s/{token}` | 非空 vmess:// 链接 |
| V2bX 用户 | V2bX 日志 `Added N new users` | N > 0 |
| 代理流量 | V2bX 日志 `received request for tcp:...` | 有出站连接记录 |
| Speedtest | `./speedtest.exe -f test_local.yaml` | HTTP/TCP latency 有值 |
| Ansible 连通 | `ansible all -m ping` | 所有节点 pong |
| nftables 规则 | `ssh root@relay nft list table inet v2b_forward` | 有 chain 和 rule |
