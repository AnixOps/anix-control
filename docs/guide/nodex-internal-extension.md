# NodeX Internal Extension Boundary

## Purpose

`v2board_AnixOps` remains the public-facing product surface. In forward-related docs, `NodeX` is a sanitized name for an optional internal backend/execution plane that can carry out runtime actions outside the Flux-panel clone contract.

The primary public contract is still the Flux-compatible `/admin/forward` UI and API surface.

## Contract Boundary

- Flux clone surface: `/admin/forward`, its matching DTOs, response envelope, and user/admin interaction flow.
- Internal backend compatibility surface: `forward.runtime_backend`, `forward.runtime.iptables_ansible.config`, `GET /api/v2/admin/forward/runtime/jobs`, deployment assets under `config/deploy/ansible/`, and installer/bootstrap environment values such as `FORWARD_RUNTIME_BACKEND` and `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`.
- Compatibility naming: `iptables_ansible` is retained in config and env names for backward compatibility. In public docs, interpret it as an internal backend adapter name, not as a promise about topology or branding.

## Deployment Guidance

- Docker images may bundle `ansible-playbook` to support this optional internal backend.
- One-click installers may preseed backend settings before the admin UI is used.
- Backend switching, job observability, and bootstrap controls belong in system/deployment surfaces, not inside the Flux-cloned `/admin/forward` page.

### NodeX Control Plane

- 系统配置 `forward.runtime.nodex.base_url` 是 NodeX 控制面基础地址（例如 `https://nodex.example.com`）。当前 `panel_forward` 与 `legacy_rule` 的 NodeX 调用都会使用它，接口不会再默认回退到节点本身的 `host:apiPort`。
- `forward.runtime.nodex.token` 用来签发 `Authorization: Bearer` 与 `X-API-Key`，可在控制面/环境变量中预设；仅在该值为空时，客户端才会尝试使用请求里节点的 `apiToken`。应通过 `FORWARD_RUNTIME_NODEX_BASE_URL`/`FORWARD_RUNTIME_NODEX_TOKEN` 环境变量在部署阶段同步。
- 请求超时可通过 `forward.runtime.nodex.timeout_seconds` 或环境变量 `FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS` 调整，默认 15 秒。

## Documentation Rule

- When backend semantics change, update `docs/DEPLOYMENT.md`, `docs/guide/api-reference.md`, and `docs/guide/flux-panel-workstream.md`.
- When `/admin/forward` contract or interaction changes, verify against `flux-panel` first and update the clone docs before treating the work as aligned.
- Public docs should avoid implementation-topology language for NodeX and keep Flux clone rules primary.
