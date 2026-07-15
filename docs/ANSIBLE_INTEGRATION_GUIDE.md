# AnixOps Ansible 集成完整指南

## 仓库信息

| 项目 | 仓库地址 | 分支 |
|------|----------|------|
| AnixOps Control | https://github.com/AnixOps/anix-control | `go_dev` |
| AnixOps Agent | https://github.com/AnixOps/anix-agent | `dev_new` |
| Ansible IaC | https://github.com/AnixOps/AnixOps-ansible | `main` |
| 测速工具 | https://github.com/AnixOps/AnixOps-speedtest | `main` |

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          面板管理流程                                         │
│                                                                             │
│  ┌──────────────┐          ┌──────────────┐         ┌─────────────────────┐ │
│  │  管理员 Web   │ ──────▶ │   面板        │         │  中继节点 (Linux)     │ │
│  │  /admin/     │  创建规则 │  :8080        │◀───────▶│  nftables           │ │
│  │  forward     │          │  SQLite       │  SSH 22 │  v2b_forward table  │ │
│  └──────────────┘          └──────────────┘          └─────────────────────┘ │
│                            │                     ▲                          │
│                            │ ansible-playbook    │ 规则生效/清理              │
│                            └─────────────────────┘                          │
│                                    inventory.ini + playbooks                │
└─────────────────────────────────────────────────────────────────────────────┘
```

**两层 Ansible 体系**：

1. **面板内置运行时**（轻量级）：处理转发规则的动态创建/删除，通过 `forward_runtime` 配置
2. **AnixOps-ansible 仓库**（完整 IaC）：全球分布式集群的基础设施管理（系统配置、监控、安全、网络优化）

## 完整数据链路

### 1. 转发规则创建链路

```
管理员在 Web 面板创建转发规则
  │
  ▼
ForwardHandler.CreateRule
  │
  ├─ 保存 ForwardNode (入站端口、协议类型)
  │
  ├─ 保存 ForwardTarget (目标地址、端口、权重)
  │
  ├─ 保存 ForwardRoute (路由策略)
  │
  └─ 调用 forward_runtime backend
       │
       ├─ nftables_ansible (推荐)
       │     │
       │     ├─ 读取 inventory.ini 获取中继主机列表
       │     │
       │     ├─ 渲染 forward_apply_nftables.yml 变量
       │     │     forward.id, forward.inPort, forward.protocol
       │     │     forward.strategy (fifo/round/rand)
       │     │     targets[] = [{host, port, weight}]
       │     │
       │     └─ ansible-playbook → SSH 到中继节点执行
       │           └─ nft add table/chain/rule ...
       │
       └─ iptables_ansible (兼容旧版)
             └─ iptables -t nat -A ...
```

### 2. nftables 规则生成链路

```
ansible-playbook forward_apply_nftables.yml
  │
  ├─ nft add table inet v2b_forward
  │
  ├─ nft add chain inet v2b_forward prerouting_{rule_id}
  │     └─ type nat hook prerouting priority dstnat; policy accept;
  │
  ├─ 根据 protocol 类型:
  │     ├─ tcp:  iif {iface} tcp dport {inPort} dnat to {target}
  │     ├─ udp:  iif {iface} udp dport {inPort} dnat to {target}
  │     └─ both: 同时创建 tcp 和 udp 规则
  │
  ├─ 根据 strategy 分配目标:
  │     ├─ fifo:  dnat to {first_target}
  │     ├─ round: dnat to numgen inc mod {N} map {0=>target0, 1=>target1, ...}
  │     └─ rand:  dnat to numgen rand mod {N} map {0=>target0, 1=>target1, ...}
  │
  └─ nft add chain inet v2b_forward postrouting_{rule_id}
        └─ type nat hook postrouting priority srcnat; policy accept;
           └─ oif {iface} masquerade
```

### 3. 流量转发实际路径

```
用户客户端 ──TCP/UDP──▶ 中继节点:入站端口
                            │
                            ▼
                      nft prerouting
                      (dnat 改写目标地址)
                            │
                            ▼
                      目标服务器:目标端口
                            │
                            ▼
                      nft postrouting
                      (masquerade 改写源地址)
                            │
                            ▼
                      目标服务器响应 ──▶ 中继节点 ──▶ 用户客户端
```

## 完整操作记录

### Step 1: 确认面板配置

```bash
cd C:\Users\z7299\Documents\GitHub\anix-control
```

检查 `config/config.yaml` 中的 `forward_runtime` 配置：

```yaml
forward_runtime:
  backend: "nftables_ansible"
  nftables_ansible:
    inventory: "config/deploy/ansible/inventory.ini"
    apply_playbook: "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
    remove_playbook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
    working_dir: "config/deploy/ansible"
    target_pattern: "{{node.host}}"
    environment:
      ANSIBLE_CONFIG: "config/deploy/ansible/ansible.cfg"
    timeout_seconds: 120
```

### Step 2: 准备 Ansible 运行时目录

```bash
# 确认目录结构
ls config/deploy/ansible/
# ├── ansible.cfg
# ├── inventory.ini.example
# ├── playbooks/
# │   ├── forward_apply_nftables.yml
# │   ├── forward_remove_nftables.yml
# │   ├── forward_apply.yml          # iptables 兼容
# │   └── forward_remove.yml         # iptables 兼容
# └── runtime-config.sample.json
```

### Step 3: 创建 inventory 文件

```bash
cp config/deploy/ansible/inventory.ini.example config/deploy/ansible/inventory.ini
```

编辑 `config/deploy/ansible/inventory.ini`：

```ini
[forward_nodes]
# 格式: <主机名> ansible_host=<IP> ansible_user=<用户> ansible_port=<端口> [认证方式]
#
# 密钥认证示例:
relay-01 ansible_host=203.0.113.10 ansible_user=root ansible_port=22 ansible_ssh_private_key_file=config/deploy/ssh/id_ed25519
#
# 密码认证示例:
# relay-02 ansible_host=203.0.113.11 ansible_user=deploy ansible_port=22 ansible_ssh_pass=YourPassword123
```

### Step 4: 配置 SSH 认证

**密钥认证（推荐）**：

```bash
# 生成密钥（如果没有）
ssh-keygen -t ed25519 -C "v2board-ansible" -f config/deploy/ssh/id_ed25519 -N ""

# 将公钥部署到中继节点
ssh-copy-id -i config/deploy/ssh/id_ed25519.pub root@203.0.113.10

# 确保私钥权限正确
chmod 600 config/deploy/ssh/id_ed25519
```

**密码认证**：

在 inventory.ini 中直接指定 `ansible_ssh_pass`，或通过 `extra_vars` 传递。

### Step 5: 验证 Ansible 连通性

```bash
cd config/deploy/ansible
ANSIBLE_CONFIG=./ansible.cfg ansible all -m ping -i inventory.ini
```

期望输出：

```
relay-01 | SUCCESS => {
    "changed": false,
    "ping": "pong"
}
```

### Step 6: 启动面板

```bash
cd C:\Users\z7299\Documents\GitHub\anix-control

go build -o build/v2board.exe -trimpath ./cmd/server
./build/v2board.exe -config config/config.yaml
```

验证：

```bash
curl -s http://localhost:8080/health
# {"status":"ok"}
```

### Step 7: 创建转发规则 (通过 API)

```bash
# 管理员登录
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/api/v2/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.invalid","password":"REDACTED_EXAMPLE_PASSWORD"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

# 创建入站节点 (监听端口 10000)
curl -s -X POST http://localhost:8080/api/v2/admin/forward/nodes \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name":"Relay Node 01","host":"203.0.113.10","port":10000,"protocol":"tcp","server_port":10000,"status":1}'
# → node_id=1

# 创建转发目标
curl -s -X POST http://localhost:8080/api/v2/admin/forward/targets \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"node_id":1,"host":"10.0.0.50","port":443,"weight":1,"status":1}'
# → target_id=1

# 创建转发规则
curl -s -X POST http://localhost:8080/api/v2/admin/forward/rules \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"node_id":1,"name":"HTTPS Relay","protocol":"tcp","strategy":"fifo"}'
# → rule_id=1

# 关联目标到规则
curl -s -X POST http://localhost:8080/api/v2/admin/forward/rules/1/targets \
  -H "Content-Type: application/json" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"target_ids":[1]}'
```

### Step 8: 验证 nftables 规则已应用

**在面板日志中查看**：

面板应输出类似：

```
[forward] Applying nftables rules on 203.0.113.10
[forward] ansible-playbook executed successfully
```

**在中继节点上确认**：

```bash
ssh root@203.0.113.10 "nft list table inet v2b_forward"
```

期望输出：

```
table inet v2b_forward {
    chain prerouting_1 {
        type nat hook prerouting priority dstnat; policy accept;
        iif "eth0" tcp dport 10000 dnat to 10.0.0.50:443
    }
    chain postrouting_1 {
        type nat hook postrouting priority srcnat; policy accept;
        oif "eth0" masquerade
    }
}
```

### Step 9: 测试转发连通性

```bash
# 从中继节点外部测试
curl -v https://203.0.113.10:10000/

# 或使用 speedtest 工具
cd C:\Users\z7299\Documents\GitHub\AnixOps-speedtest
./speedtest.exe -t tcp -p 10000 -s 203.0.113.10
```

### Step 10: 删除转发规则

```bash
# 通过面板 API 删除
curl -s -X DELETE http://localhost:8080/api/v2/admin/forward/rules/1 \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

面板会自动调用 `forward_remove_nftables.yml` 清理 nftables 规则。

在中继节点确认已清理：

```bash
ssh root@203.0.113.10 "nft list tables"
# v2b_forward 表应不存在，或对应 chain 已被删除
```

## 已知问题与解决方案

### 1. Ansible 连接失败

**现象**：面板日志显示 `ansible-playbook execution failed` 或 timeout。

**排查**：

```bash
# 手动测试 SSH 连通性
ssh -i config/deploy/ssh/id_ed25519 root@203.0.113.10 "echo ok"

# 手动运行 Ansible ping
cd config/deploy/ansible
ANSIBLE_CONFIG=./ansible.cfg ansible all -m ping -i inventory.ini -vvv
```

**常见原因**：

| 原因 | 解决方案 |
|------|----------|
| SSH 密钥权限不对 | `chmod 600 config/deploy/ssh/id_ed25519` |
| 中继节点防火墙阻止 22 端口 | `ufw allow 22` 或 `firewall-cmd --add-port=22/tcp` |
| inventory.ini 路径错误 | 确认 `forward_runtime.nftables_ansible.inventory` 路径指向实际文件 |
| Ansible 未安装在中继控制机 | `pip install ansible` 或使用面板内置 runner |

### 2. nftables 不可用

**现象**：Playbook 执行成功但规则未生效，或报错 `command not found: nft`。

**排查**：

```bash
ssh root@203.0.113.10 "which nft && nft --version"
```

**解决方案**：

```bash
# Debian/Ubuntu
apt-get install nftables
systemctl enable nftables

# CentOS/RHEL
yum install nftables
systemctl enable nftables

# 确认内核模块已加载
ssh root@203.0.113.10 "lsmod | grep nf_tables"
```

### 3. 多目标轮询不生效

**现象**：配置了 `strategy: round` 但流量只到第一个目标。

**原因**：`nft numgen` 需要 nftables 0.9.0+ 版本。

```bash
ssh root@203.0.113.10 "nft --version"
# 期望: nftables v0.9.0 或更高
```

**解决方案**：升级 nftables 或使用 `fifo` 策略作为降级方案。

### 4. postrouting masquerade 导致源 IP 丢失

**现象**：目标服务器看到的来源 IP 是中继节点 IP，不是用户真实 IP。

**原因**：这是 masquerade 的预期行为。SNAT 改写源地址是 NAT 转发的工作原理。

**解决方案**：

- 如果需要保留真实源 IP，使用代理模式（如 HAProxy/Envoy）替代纯 nftables NAT
- 在应用层通过 `X-Forwarded-For` 传递真实 IP

### 5. inventory.ini 被 git 跟踪

**现象**：`git status` 显示 inventory.ini 变更。

**解决方案**：

```bash
# inventory.ini 已在 .gitignore 中，不应被跟踪
# 如果已被跟踪，执行：
git update-index --assume-unchanged config/deploy/ansible/inventory.ini
```

## 配置文件参考

### config/config.yaml (面板)

```yaml
forward_runtime:
  backend: "nftables_ansible"  # 推荐后端
  # backend: "iptables_ansible"  # 旧版兼容
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

### config/deploy/ansible/inventory.ini

```ini
[forward_nodes]
# 密钥认证
relay-01 ansible_host=203.0.113.10 ansible_user=root ansible_port=22 ansible_ssh_private_key_file=config/deploy/ssh/id_ed25519
relay-02 ansible_host=203.0.113.11 ansible_user=root ansible_port=22 ansible_ssh_private_key_file=config/deploy/ssh/id_ed25519

# 密码认证（不推荐，明文密码）
# relay-03 ansible_host=203.0.113.12 ansible_user=deploy ansible_port=2222 ansible_ssh_pass=YourPassword
```

### config/deploy/ansible/ansible.cfg

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

### runtime-config.sample.json (非标准路径覆盖)

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

## Playbook 变量参考

### forward_apply_nftables.yml 输入变量

| 变量 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `forward.id` | int | 规则 ID | `1` |
| `forward.inPort` | int | 入站监听端口 | `10000` |
| `forward.interfaceName` | string | 入站网卡 | `eth0` |
| `forward.protocol` | string | 协议类型 | `tcp` / `udp` / `both` |
| `forward.strategy` | string | 负载均衡策略 | `fifo` / `round` / `rand` |
| `targets` | list | 目标列表 | `[{host:"10.0.0.50", port:443}]` |
| `targets[].host` | string | 目标服务器 IP | `10.0.0.50` |
| `targets[].port` | int | 目标服务端口 | `443` |
| `targets[].weight` | int | 权重（当前保留） | `1` |

## 验证清单

| 检查项 | 命令 | 期望结果 |
|--------|------|----------|
| 面板健康 | `curl localhost:8080/health` | `{"status":"ok"}` |
| Ansible 连通 | `ansible all -m ping -i inventory.ini` | 所有节点 pong |
| SSH 密钥权限 | `ls -la config/deploy/ssh/id_ed25519` | `-rw-------` |
| nft 版本 | `ssh root@relay "nft --version"` | `>= 0.9.0` |
| 转发规则创建 | 面板 API 创建 rule | 返回 rule_id |
| nftables 规则生效 | `ssh root@relay "nft list table inet v2b_forward"` | 有 chain 和 rule |
| 转发流量通过 | `curl -v https://relay_ip:inPort/` | 目标服务响应 |
| 规则删除清理 | 面板 API 删除 rule | chain 被移除 |
| Ansible 超时 | `timeout_seconds: 120` | 大型规则集也能完成 |
