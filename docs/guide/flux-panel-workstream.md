# Flux-panel Clone Workstream

## Current Status

- Base compat endpoints are live for `/forward/*`, `/user/reset`, `/tunnel/user/tunnel`, and `/tunnel/user/assign|list|remove|update`, with `web/src/views/admin/Forward.vue`, `web/src/views/admin/Users.vue`, and `internal/service/forward_panel_service.go` providing the current local surface.
- Response envelope, request DTO names, and the `ForwardUserTunnel` auth model are already aligned at the route-contract level; use `docs/guide/flux-forward-contract.md` as the detailed boundary document.
- The current local user-page linkage lives inside `web/src/views/admin/Users.vue`: it already shows used flow, reset day, and rate-limit columns, filters duplicate tunnels on create, locks tunnel selection on edit, and uses dedicated reset-confirmation dialogs against `/api/v2/user/reset`.
- The real Flux speed-limit resource now exists locally as `/api/v2/speed-limit/create|list|update|delete|tunnels`, with a dedicated `SpeedLimit` model/service/handler, Flux-style `code/msg/ts/data` envelope, and a standalone admin page at `/admin/limit`. The remaining gap is exact `limit.tsx` layout parity plus runtime-side limiter propagation.
- Upstream Flux monthly reset semantics are now partially cloned through `ForwardFlowResetWorker`: startup performs a catch-up run, then daily `00:00:05` local-time scans reset user flow and user-tunnel flow, handle month-end overflow days, pause expired-user forwards, and disable expired user-tunnel grants. The remaining gap is exact parity with Flux's dedicated user-disable state model.
- Admin-side runtime controls remain outside the Flux-cloned `/admin/forward` page. `web/src/views/admin/System.vue` currently hosts `forward.runtime_backend`, `forward.runtime.iptables_ansible.config`, and `GET /api/v2/admin/forward/runtime/jobs` as the compatibility surface for optional internal backend delegation (`NodeX` in public docs). `install.sh` (`panel_install.sh` wrapper) and the Docker bootstrap keys (`FORWARD_RUNTIME_BACKEND`, `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`) belong to the same deployment/backplane surface and must not change the Flux-shaped page.
- Current internal runtime calls for `panel_forward` and `legacy_rule` now require an explicit NodeX control-plane base URL (`forward.runtime.nodex.base_url` / `FORWARD_RUNTIME_NODEX_BASE_URL`) and no longer fall back to the ingress node transport for the outer control-plane hop; keep this requirement documented outside the Flux-cloned page.

## Reference Mapping

| Flux-panel Piece | Local Files | Notes |
|------------------|-------------|-------|
| `ForwardController` / `TunnelController` | `internal/handler/forward_panel.go`, `internal/router/router.go` | Mirror user/admin routes and wrapper `code/msg/ts/data`. |
| `UserController.reset` / `UserServiceImpl.reset` | `internal/handler/admin.go`, `internal/router/router.go` | Manual flow reset is aligned at the request/response envelope level. |
| `ForwardServiceImpl` / `TunnelServiceImpl` | `internal/service/forward_panel_service.go` | Implements compat DTOs plus `DiagnoseForward`. |
| `UserTunnelServiceImpl` | `internal/service/forward_panel_service.go`, `web/src/views/admin/Users.vue` | Grant lifecycle is partially cloned through the admin user page. |
| `SpeedLimitController` / `SpeedLimitServiceImpl` / `limit.tsx` | `internal/model/speed_limit.go`, `internal/service/speed_limit_service.go`, `internal/handler/speed_limit.go`, `web/src/views/admin/Limit.vue` | Backend resource and standalone admin page are live; remaining gaps are exact page parity and runtime-side limiter semantics. |
| `ResetFlowAsync` | `internal/service/forward_flow_reset_worker.go`, `cmd/server/main.go` | Scheduled monthly reset plus expiry pause/disable side effects are running locally; remaining gap is exact Flux user-disable state parity. |
| Frontend pages | `web/src/views/admin/Forward.vue`, `web/src/views/admin/Users.vue`, `web/src/api/admin.js` | Current clone surface is split across the forward page and the admin users page rather than a full standalone `user.tsx` clone. |

## Status Scale

| Status | Description |
|--------|-------------|
| Not Started | Component/flow has no local analogue yet. |
| Partial | API or UI exists but runtime semantics still rely on local-only logic or missing resources. |
| Aligned | Reference routes, DTOs, response envelope, semantics, and runtime actions match. |

## Current Boundaries

| Area | Local Status | Notes |
|------|------|------|
| Forward compat routes | Partial | Base routes exist; runtime and diagnose semantics still differ. |
| User reset compat route | Partial | Manual `{id,type}` reset is cloned and automatic monthly reset plus expiry pause/disable side effects run in background, but full Flux user-state parity is not complete. |
| User-tunnel CRUD compat routes | Partial | Create/list/update/remove routes exist and drive `Users.vue`, but list semantics and side effects still differ. |
| User-page tunnel grant UI | Partial | Used flow, reset flow, rate-limit columns, confirm dialogs, duplicate-tunnel filtering, and tunnel-scoped speed-limit selection are present; exact Flux layout parity still differs. |
| Real speed-limit resource | Partial | Backend resource and standalone admin page exist, but runtime-side limiter semantics and exact page parity are still not cloned. |
| Automatic monthly reset | Partial | Worker-driven monthly reset plus expiry pause/disable side effects are live; exact Flux user-disable-state parity is still missing. |

## Recommended Implementation Order

1. Replace the current grant-flow aggregation/backfill with native `ForwardUserTunnel` traffic counters so `inFlow/outFlow` semantics stop depending on `v2_forward` aggregation.
2. Reconcile forward runtime semantics: create/update/delete/pause/resume side effects, diagnose node paths, quota/expiry-driven runtime pauses, and runtime-side speed-limit propagation.
3. Close the remaining user-state parity gap between local `expired_at` handling and Flux's dedicated disable semantics.
4. Finish the last-mile `user.tsx` / `limit.tsx` visual and interaction parity after the remaining runtime gaps are closed.

## Definition Of Done

- Routes exist in the correct scope (user vs. admin) with `/api/v2/`/`/api/v2/admin/` as needed plus any justified mirrors.
- Request bodies, response envelope (`code/msg/ts/data`), and DTO fields retain the Flux-panel naming and structure.
- Adjacent resources consumed by the Flux user page, especially `speed-limit/list` and scheduled reset semantics, are either cloned or explicitly documented as gaps.
- UI flow, validation, and dialog sequencing match the reference; transitions trigger the same side effects.
- Runtime effects (remote services, node-level actions, diagnostics, quota pauses, scheduled resets) have been implemented or the gap is documented with a mitigation plan.
- Tests cover the new behavior and the contract docs are updated together with implementation changes.
