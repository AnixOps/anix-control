# Forward/Tunnel Manual Smoke Tests

本文件围绕两套运行时（NodeX Mode/iptables_ansible Mode）提供手工验证流程。每个步骤都要明确检查的是 **proxy Node (`model.Node`)** 还是 **Forward Node (`model.ForwardNode`)**，因为它们分别服务于用户连接和 runtime job。

## 1. NodeX Mode Smoke（`forward.runtime_backend=gost`）

1. **确认配置**：在 `/admin/system` 或 `.env` 中确保 `forward.runtime_backend=gost`、`forward.runtime.nodex.base_url`、`forward.runtime.nodex.token`、`forward.runtime.nodex.timeout_seconds` 已配置且指向可以访问的 NodeX 控制面。代理节点（`/admin/nodes`）可以离线，只要转发节点 (`v2_forward_node`，`type=relay`) 在线即可。
2. **ForwardNode 可达**：读取 `ForwardNode` 的 `host`/`api_port`/`api_token`，在 NodeX 控制面机器直接执行 `curl http://<forwardNode.host>:<api_port>/health` 或 `nc` 检测 TCP 端口，确保 NodeX 能向该转发节点发出 job。
3. **提交 runtime job**：使用 panel 提供的 `/api/v2/admin/forward/runtime/jobs` 创建 job，或者通过 `ForwardService` 调用 `Apply` 触发 job。接着在 NodeX 控制面执行：

   ```bash
   curl -H "Authorization: Bearer <token>" "<base_url>/api/v2/admin/forward/runtime/jobs?backend=gost"
   ```

   确认控制面能返回 `status`、`result` 字段，且 job 列表包含刚才的新增 job。
4. **观察数据库**：`SELECT * FROM v2_forward_runtime_job WHERE backend='gost' ORDER BY id DESC LIMIT 5`，确认 `status`（pending/running/success/error）与 `result` 字段有输出，出错时查看 `error`。`NodeX` job 的 `node_id` 应指向相关 `ForwardNode`。
5. **验证控制面重试**：将 Base URL 临时置空，重复 job 确认校验函数会立刻报错（`forward.runtime.nodex.base_url` 必填的错误），说明控制面不再尝试 fallback 到 ingress node。

## 2. Ansible Mode Smoke（`forward.runtime_backend=iptables_ansible`）

1. **确认配置**：`forward.runtime_backend=iptables_ansible`，`forward.runtime.iptables_ansible.config` 中必须包含 `inventory`、`playbookApply`/`playbookRemove`、`workingDir`。`ForwardNode` 需要配置 SSH 访问字段（`ssh_host`/`ssh_port`/`ssh_user`/`ssh_password`/`ssh_key`）。
2. **手动运行 Ansible**：在部署环境复制相同的 inventory 和 `ansible-playbook` 命令，执行 `ansible-playbook -i /path/to/inventory /path/to/playbook.yml`，确认 SSH 凭证可用，且 `ForwardNode` 所指的机器能响应。
3. **触发 runtime job**：通过面板 UI/接口创建 forward job。在数据库中执行 `SELECT * FROM v2_forward_runtime_job WHERE backend='iptables_ansible' ORDER BY id DESC LIMIT 5`，确认 job 进入 `status=pending` 并且 `payload` 包含 `ansibleRuntime`。
4. **查看 Ansible 日志**：使用 `ansible-playbook` 日志或 `forward_runtime_job_executor` 日志（`v2_forward_runtime_job` + `error` 字段）确认 job 没有连接超时或权限问题。
5. **SSH 角色区分**：代理 `Node` 只需要保持在线服务用户连接，runtime job 访问的 SSH 目标是 `ForwardNode`。如果 job 失败，请同时确认两个角色中的节点都处于预期状态。

## 3. 参考配置与文档位置

- NodeX Mode 的操作细节：`docs/guide/forward-tunnel-runtime-ops.md`
- Ansible Mode 的配置与 inventory 范例：同一文件中的 Ansible 小节
- 手工 Smoke/认证路径：本文件
- 边界说明与 NodeX 内部扩展：`docs/guide/nodex-internal-extension.md`

## 4. Quick Checklist

| 项目 | NodeX Mode | Ansible Mode |
|------|------------|--------------|
| control plane | `forward.runtime.nodex.base_url`/`token` | `forward.runtime.iptables_ansible.config` + inventory |
| runtime target | `ForwardNode` with `api_port`/`api_token` | `ForwardNode` with `ssh_*` |
| verification | `curl <base_url>/admin/forward/runtime/jobs?backend=gost` + `v2_forward_runtime_job` | `ansible-playbook` + `v2_forward_runtime_job backend=iptables_ansible` |
| proxy node involvement | Not required | Not required |
