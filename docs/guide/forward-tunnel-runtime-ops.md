# Forward/Tunnel Runtime Operations

本指南将 NodeX Mode（`gost` 控制面）和 iptables/ansible Mode（SSH+`ansible-playbook`）作为两套并行的运行时方案，对应 `forward.runtime_backend`。请勿在同一次部署中混用两种模式，所有运维、文档和 smoke 路径都应清晰标注当前模式。

## 双运行时概览

| 运行时模式 | 启用配置 | 控制面 | 核心节点角色 |
|------------|----------|--------|---------------|
| NodeX Mode | `forward.runtime_backend=gost` + `forward.runtime.nodex.base_url`/`forward.runtime.nodex.token` | 走 NodeX REST API（`defaultForwardRuntimeNodeXExecutePath`） | `ForwardNode`（`type=relay`）提供 `host`/`api_port`/`api_token` 供控制面调度；代理 `model.Node` 只做用户连接 |
| iptables/ansible Mode | `forward.runtime_backend=iptables_ansible` + `forward.runtime.iptables_ansible.config`（可选 inventory/playbook/extra vars） | 借助远程 SSH/`ansible-playbook` 执行 `iptables` 设备配置 | `ForwardNode` 提供 SSH 连接（`ssh_host`/`ssh_port`/`ssh_user`/`ssh_password`/`ssh_key`）；代理 `model.Node` 不参与 |

代理节点（`/admin/nodes`）与转发节点（`v2_forward_node`）不是一回事，前者承载用户连接，后者只在 runtime job 中出现。务必在所有调试与文档中标明正在观察的是哪个实体。

## NodeX Mode（`gost` 控制面）

### 核心配置

以下环境变量可通过 `.env`、容器环境或脚本注入，`InitForwardRuntimeSystemConfigFromEnv` 会把它们写入系统配置：

```env
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=nodex-secret
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=20
```

`forward.runtime.nodex.base_url` 必须指向部署好的 NodeX 控制面（通常与 NodeX 执行器同机），`token` 用来与控制面通信并被写进 HTTP header `Authorization`/`X-API-Key`。`forward.runtime.nodex.timeout_seconds` 用于 HTTP client 超时控制。

`ForwardNode`（`model.ForwardNode`）需要包含 `host`/`api_port`/`api_token` 字段，NodeX 控制面将请求推送到这些节点，不再尝试默认用 ingress node 的 SSH 信息。若 `forward.runtime.nodex.base_url` 未配置，请求立刻失败并在 `v2_forward_runtime_job` 中以 `backend=gost` 记录 error。

### 运行时校验

1. 在面板 `/admin/system` 核实 `forward.runtime_backend`、`forward.runtime.nodex.base_url`、`forward.runtime.nodex.token` 处于预期值。
2. 确认相关 `ForwardNode`（`type=relay`）已配置 REST 所需字段，并在 `v2_forward_node` 表中还原出使用中的 job。
3. NodeX 工作流程会写入 `v2_forward_runtime_job.backend=gost` 的记录。可通过 `SELECT * FROM v2_forward_runtime_job WHERE backend='gost' ORDER BY id DESC LIMIT 10` 检查 job 状态/错误。
4. 调试时使用 `curl -H "Authorization: Bearer <token>" <base_url>/api/v2/admin/forward/runtime/jobs?backend=gost` 查看 NodeX job 列表，确认控制面可达。

## iptables/ansible Mode（内置 SSH + Ansible）

### 核心配置

环境变量示例（可通过 `.env`、Docker Compose 或安装脚本注入）：

```env
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"/etc/ansible/forward_inventory.ini","playbookApply":"/etc/ansible/forward_apply.yml","playbookRemove":"/etc/ansible/forward_remove.yml","workingDir":"/etc/ansible"}
FORWARD_RUNTIME_ANSIBLE_INVENTORY=/etc/ansible/forward_inventory.ini
```

配置界面会把这个 JSON 写入 `forward.runtime.iptables_ansible.config`，其中可携带 `targetPattern`、`command`、`extraVars`、`environment` 等字段。`ForwardNode` 需提供 SSH 连接所需 `ssh_host`/`ssh_port`/`ssh_user`/`ssh_password`/`ssh_key`（根据 inventory）字段；Ansible 控制面通过这些字段触发 `ansible-playbook`。

### 运行时校验

1. 在 runtime job 报错前，先在运行时节点或 CI 环境上手动执行 `ansible-playbook`（使用相同 inventory 与 playbook）看是否能连接目标 ForwardNode。
2. 使用 `SELECT * FROM v2_forward_runtime_job WHERE backend='iptables_ansible' ORDER BY id DESC LIMIT 10` 验证 job 状态。
3. 执行 `ansible-playbook -i <inventory> <apply.yml>` 时候，可以加 `-vv` 输出诊断信息，验证 `forward.runtime.iptables_ansible.inventory` 对应 `ForwardNode` 的 SSH host。

## 相关文档

- NodeX Mode 与 iptables/ansible Mode 的详细操作步骤在 `docs/guide/forward-tunnel-smoke-test.md`。
- NodeX 内部扩展边界与代理 vs 转发节点职责在 `docs/guide/nodex-internal-extension.md`。
- Flux clone runtime/401 诊断提醒仍保留在 `docs/guide/forward-tunnel-runtime-ops.md` 之中，任何 runtime 变更请同步更新此文件再通知团队。
