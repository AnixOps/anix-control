# Flux-panel Clone Workstream

## Current Status

- Base UI and compat endpoints are live for `/forward/*`, `/user/reset`, `/tunnel/user/tunnel`, and `/tunnel/user/assign|list|remove|update`, with `web/src/views/admin/Forward.vue`, `web/src/views/admin/Users.vue`, and `internal/service/forward_panel_service.go` providing the first pass.
- Response envelope, DTO names, and `ForwardUserTunnel` auth model already align with the Flux-panel reference; these are documented in `docs/guide/flux-forward-contract.md`.
- The admin user page already exposes tunnel grant listing, used-flow display, manual flow reset, and rate-limit columns against the compat routes.
- Remaining gaps are runtime semantics (create/update/delete/pause/resume side effects, diagnose node paths, quota/expiry linkage) plus the last-mile `UserTunnel` details: stored flow counter semantics, speed-limit selector/list joins, duplicate tunnel filtering, and Flux-style reset dialogs.
- Admin-side runtime controls remain outside the Flux-cloned `/admin/forward` page. `web/src/views/admin/System.vue` currently hosts `forward.runtime_backend`, `forward.runtime.iptables_ansible.config`, and `GET /api/v2/admin/forward/runtime/jobs` as the compatibility surface for optional internal backend delegation (`NodeX` in public docs). `install.sh` (`panel_install.sh` wrapper) and the Docker bootstrap keys (`FORWARD_RUNTIME_BACKEND`, `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`) belong to the same deployment/backplane surface and must not change the Flux-shaped page.
- Current internal runtime calls for `panel_forward` and `legacy_rule` now require an explicit NodeX control-plane base URL (`forward.runtime.nodex.base_url` / `FORWARD_RUNTIME_NODEX_BASE_URL`) and no longer fall back to the ingress node transport for the outer control-plane hop; keep this requirement documented outside the Flux-cloned page.

## Reference Mapping

| Flux-panel Piece | Local Files | Notes |
|------------------|-------------|-------|
| `ForwardController` / `TunnelController` | `internal/handler/forward_panel.go`, `internal/router/router.go` | Mirror user/admin routes and wrapper `code/msg/ts/data`. |
| `ForwardServiceImpl` / `TunnelServiceImpl` | `internal/service/forward_panel_service.go` | Implements compat DTOs plus `DiagnoseForward`. |
| `TunnelListDto` / `ForwardDto` | `PanelTunnelListItem` / `PanelForwardListItem` | Field names (e.g., `ip`, `type`, `protocol`) preserved per contract. |
| Frontend pages | `web/src/views/admin/Forward.vue`, `web/src/views/admin/Users.vue`, `web/src/api/admin.js` | Provides the Flux-shaped admin forward flow plus the first pass of user-tunnel grant management. |

## Status Scale

| Status | Description |
|--------|-------------|
| Not Started | Component/flow has no local analogue yet (e.g., full UserTunnel management UI). |
| Partial | API or UI exists but runtime semantics still rely on panel-only logic or missing validations. |
| Aligned | Reference routes, DTOs, response envelope, and semantics match; runtime actions also executed. |

## Recommended Implementation Order

1. Finish the remaining Flux-panel `UserTunnel` semantics: source `inFlow/outFlow` from the relation record, join real speed-limit metadata, filter already-assigned tunnels from the create picker, and replace simplified confirm/alert reset flows with Flux-style dialogs.
2. Reconcile forward runtime semantics: linking create/update/delete/pause/resume to remote services, and ensuring the node chain state mirrors Flux-panel behavior.
3. Implement diagnose and other node-chain instrumentation so the panel-side diagnostics match reference expectations.
4. Address quota, expiry, and flow-reset behaviors that rely on `UserTunnel` state so the clone remains behaviorally faithful.

## Definition Of Done

- Routes exist in the correct scope (user vs. admin) with `/api/v2/`/`/api/v2/admin/` as needed plus any granted mirrors.
- Request bodies, response envelope (`code/msg/ts/data`), and DTO fields retain the Flux-panel naming/structure.
- UI flow, validation, and dialog sequencing match the reference; transitions trigger the same side effects.
- Runtime effects (remote services, node-level actions, diagnostics) have been implemented or the gap is documented with a mitigation plan.
- Tests cover the new behavior and the contract doc is updated (current doc references being this file and `flux-forward-contract.md`).
