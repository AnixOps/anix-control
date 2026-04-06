# NodeX Internal Extension Boundary

`v2board_AnixOps` 仍然是围绕 Flux `/admin/forward` 页面及其 API 进行开发。NodeX/iptables_ansible 运行时只是内部扩展，用于在面板之外触发实际的转发配置。这个文档描述了它们的边界、必须的配置以及运维人员在 NodeX vs Ansible 之间切换时需要牢记的验证路径。

## 双运行时模式

| 模式 | `forward.runtime_backend` | 控制面 | 执行方式 |
|------|---------------------------|--------|----------|
| NodeX Mode | `gost` | NodeX REST 接口（`forward.runtime.nodex.base_url`/`token`） | 控制面向 `ForwardNode` 推送 HTTP job（`backend=gost`） |
| iptables+Ansible Mode | `iptables_ansible` | 本地 SSH + `ansible-playbook`，`forward.runtime.iptables_ansible.config` 控制 | 将命令推送到 `ForwardNode` 的 SSH 终端（`backend=iptables_ansible`） |

### 配置示例

```env
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=nodex-secret
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=20

FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"/etc/ansible/forward_inventory.ini","playbookApply":"/etc/ansible/forward_apply.yml"}
FORWARD_RUNTIME_ANSIBLE_INVENTORY=/etc/ansible/forward_inventory.ini
```

`InitForwardRuntimeSystemConfigFromEnv` 会把这些环境变量写入系统配置，前端 `/admin/system` 有对应展示，文档处于 `docs/guide/forward-tunnel-runtime-ops.md` 和 `docs/guide/forward-tunnel-smoke-test.md`。

## Proxy Node 与 Forward Node

- **Proxy Node (`model.Node`, `/admin/nodes`)**：暴露给用户做代理连接，支持 VMess/VLESS/Trojan 等协议。NodeX runtime job 不直接作用于它，除非它被新建为 `ForwardNode` 之后。
- **Forward Node (`model.ForwardNode`, `v2_forward_node`)**：只在 runtime job 中使用。NodeX Mode 下提供 `host/api_port/api_token`，Ansible Mode 下提供 SSH 访问字段；其 `type` 字段可区分 `relay`（入口）/`exit`（出口）。
- 运行时 job 的 `node_id` 指向 `ForwardNode`，反映在 `v2_forward_runtime_job.node_id` 字段。

确保文档与操作流程都明确强调这两类节点的分离，避免在 `/admin/nodes` 或 NodeX job 日志中混淆角色。

## 文档连接

- Runtime 操作指南：`docs/guide/forward-tunnel-runtime-ops.md`（NodeX/Ansible 配置、env 示例、字段说明）。
- 手工 Smoke 验证：`docs/guide/forward-tunnel-smoke-test.md`（NodeX job/curl 验证 + Ansible playbook smoke）。
- Flux Clone 相关：`docs/guide/flux-panel-clone.md`、`docs/guide/flux-forward-contract.md`、`docs/guide/api-reference.md`、`readme.md`（同步 runtime 说明）。

## 维护建议

- 每次更改 NodeX control plane 逻辑、ansible inventory 写法或 smoke 流程时，依次更新本文件、`forward-tunnel-runtime-ops.md`、`forward-tunnel-smoke-test.md`。
- 所有与 NodeX Mode 相关的 API 路径（如 `/api/v2/admin/forward/runtime/jobs`）都属于内部运维 surface，不应该被认为是 Flux `/admin/forward` 的一部分。
- 由于这是内部扩展，外部文档（如 README）只需要在“部署”或 “运维” 章节中提及 NodeX/iptables 相关字段，避免在 Flux 页面上暴露这些细节。
