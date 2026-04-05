# Flux-panel Clone Workstream

## Current Status

- Base UI and compat endpoints are live for `/forward/*` and `/tunnel/user/tunnel`, with `web/src/views/admin/Forward.vue` and `internal/service/forward_panel_service.go` providing the first pass.
- Response envelope, DTO names, and `ForwardUserTunnel` auth model already align with the Flux-panel reference; these are documented in `docs/guide/flux-forward-contract.md`.
- Remaining gaps are runtime semantics (create/update/delete/pause/resume side effects, diagnose node paths, quota/expiry linkage) and fully featured `UserTunnel` management.
- Dual-runtime admin controls now live in `web/src/views/admin/System.vue` with `forward.runtime_backend`, `forward.runtime.iptables_ansible.config`, and `GET /api/v2/admin/forward/runtime/jobs`; an in-process worker started by `cmd/server/main.go` consumes queued `iptables_ansible` jobs. This remains an extension surface and should not change the Flux-shaped `/admin/forward` page.

## Reference Mapping

| Flux-panel Piece | Local Files | Notes |
|------------------|-------------|-------|
| `ForwardController` / `TunnelController` | `internal/handler/forward_panel.go`, `internal/router/router.go` | Mirror user/admin routes and wrapper `code/msg/ts/data`. |
| `ForwardServiceImpl` / `TunnelServiceImpl` | `internal/service/forward_panel_service.go` | Implements compat DTOs plus `DiagnoseForward`. |
| `TunnelListDto` / `ForwardDto` | `PanelTunnelListItem` / `PanelForwardListItem` | Field names (e.g., `ip`, `type`, `protocol`) preserved per contract. |
| Frontend pages | `web/src/views/admin/Forward.vue`, `web/src/api/admin.js` | Provides the UI flow/flavor for admins; user pages still under work. |

## Status Scale

| Status | Description |
|--------|-------------|
| Not Started | Component/flow has no local analogue yet (e.g., full UserTunnel management UI). |
| Partial | API or UI exists but runtime semantics still rely on panel-only logic or missing validations. |
| Aligned | Reference routes, DTOs, response envelope, and semantics match; runtime actions also executed. |

## Recommended Implementation Order

1. Deliver the Flux-panel `UserTunnel` management/authorization screens and tighten `ForwardUserTunnel` semantics so the system can link tunnels to users exactly as in the reference.
2. Reconcile forward runtime semantics: linking create/update/delete/pause/resume to remote services, and ensuring the node chain state mirrors Flux-panel behavior.
3. Implement diagnose and other node-chain instrumentation so the panel-side diagnostics match reference expectations.
4. Address quota, expiry, and flow-reset behaviors that rely on `UserTunnel` state so the clone remains behaviorally faithful.

## Definition Of Done

- Routes exist in the correct scope (user vs. admin) with `/api/v2/`/`/api/v2/admin/` as needed plus any granted mirrors.
- Request bodies, response envelope (`code/msg/ts/data`), and DTO fields retain the Flux-panel naming/structure.
- UI flow, validation, and dialog sequencing match the reference; transitions trigger the same side effects.
- Runtime effects (remote services, node-level actions, diagnostics) have been implemented or the gap is documented with a mitigation plan.
- Tests cover the new behavior and the contract doc is updated (current doc references being this file and `flux-forward-contract.md`).
