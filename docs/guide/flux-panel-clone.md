# Flux-panel Clone Guide

## Goal

Future work for forward, tunnel, user-tunnel authorization and adjacent pages must treat [`flux-panel`](https://github.com/bqlpfy/flux-panel) as the source implementation.

"1:1 clone" means all of the following should match the reference:

- request path and HTTP method
- auth scope and role requirements
- request body field names
- response envelope and DTO field names
- page layout and interaction flow
- business semantics and side effects

UI-only similarity is not enough.

## Reference Repositories

### Upstream

- GitHub: `https://github.com/bqlpfy/flux-panel`

### Local Reference Copy

- `C:\Users\z7299\AppData\Local\Temp\flux-panel`

### Files To Read First

Backend:

- `springboot-backend/src/main/java/com/admin/controller/ForwardController.java`
- `springboot-backend/src/main/java/com/admin/controller/TunnelController.java`
- `springboot-backend/src/main/java/com/admin/controller/FlowController.java`
- `springboot-backend/src/main/java/com/admin/service/impl/ForwardServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/TunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/UserTunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/common/dto/`
- `springboot-backend/src/main/java/com/admin/common/dto/ForwardDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/ForwardUpdateDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/ForwardWithTunnelDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/TunnelDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/TunnelUpdateDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/UserTunnelDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/UserTunnelQueryDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/UserTunnelUpdateDto.java`
- `springboot-backend/src/main/java/com/admin/common/dto/UserTunnelWithDetailDto.java`
- `springboot-backend/src/main/java/com/admin/entity/`

Frontend:

- `vite-frontend/src/pages/forward.tsx`
- `vite-frontend/src/pages/tunnel.tsx`
- `vite-frontend/src/pages/user.tsx`
- `vite-frontend/src/api/index.ts`
- `vite-frontend/src/api/`
- `vite-frontend/src/components/`

## Mandatory Workflow

Use this order every time:

1. Read the reference page and API client.
2. Read the reference controller and confirm auth scope.
3. Read the reference service and identify real semantics.
4. Read the reference DTO/entity definitions.
5. Map those pieces to local `router`, `handler`, `service`, `model`, `web/src/views`, `web/src/api`.
6. Implement.
7. Re-check the reference before considering the task done.

Do not start by inventing a local version and then "making it look similar later".

## Forward/Tunnel Endpoint Scope Matrix

This matrix is the minimum route surface that must stay visible in clone planning.

| Area | Flux Endpoint(s) | Flux Auth Scope | Local Status | Notes |
|------|------|------|------|------|
| Forward user routes | `POST /api/v1/forward/create|list|update|delete|force-delete|pause|resume|diagnose|update-order` | JWT user | partial clone | local compat routes exist and are detailed in `flux-forward-contract.md` |
| Tunnel admin routes | `POST /api/v1/tunnel/create|list|update|delete|diagnose` | role-restricted | not started | do not collapse these into user routes |
| UserTunnel admin routes | `POST /api/v1/tunnel/user/assign|list|remove|update` | role-restricted | partial clone | compat routes exist, but list field semantics and UI flow still differ |
| User reset route | `POST /api/v1/user/reset` | role-restricted | partial clone | compat route exists at `/api/v2/user/reset`; success/error wrapper now mirrors Flux |
| Tunnel user route | `POST /api/v1/tunnel/user/tunnel` | JWT user | partial clone | this is the only user-scope route under `TunnelController` |

## Non-negotiable Rules

### Route Scope

- If the reference endpoint is a logged-in user endpoint, the local project must expose a JWT user route.
- If the reference endpoint is an admin-only endpoint, then use `/api/v2/admin/*`.
- Admin mirror routes are allowed for backward compatibility, but they do not replace the real user/admin scope from the reference.

### Response Contract

If the reference uses the standard wrapper, keep it:

```json
{
  "code": 0,
  "msg": "操作成功",
  "ts": 1712300000000,
  "data": {}
}
```

Rules:

- always keep `code`, `msg`, `ts`, `data`
- keep timestamp units consistent with the reference
- wrap error responses the same way

### DTO Fields

- Never drop a reference DTO field just because the current page does not use it yet.
- Never rename `camelCase` fields into local naming conventions for convenience.
- If the reference exposes `ip`, `type`, `protocol`, keep those exact fields even if local code also has `inIp`.

### Runtime Semantics

Before declaring a module "cloned", confirm whether the reference implementation:

- only touches DB state
- or also changes remote runtime state
- or performs node-side diagnosis / chained actions

If local code only matches the DB layer but not runtime side effects, document the gap explicitly.

## Local Mapping

| Flux-panel Piece | Local Project |
|------|------|
| Spring controller | `internal/handler/` + `internal/router/router.go` |
| service impl | `internal/service/` |
| entity | `internal/model/` |
| frontend page | `web/src/views/` |
| frontend api | `web/src/api/` |

## Current Forward/Tunnel Base

As of `2026-04-05`, the local project already has these pieces:

- cloned page:
  - `web/src/views/admin/Forward.vue`
- compat service:
  - `internal/service/forward_panel_service.go`
- compat handler:
  - `internal/handler/forward_panel.go`
- compat routing:
  - `internal/router/router.go`
- frontend request wrapper:
  - `web/src/api/admin.js`
- user management/tunnel grant page:
  - `web/src/views/admin/Users.vue`
- explicit auth relation:
  - `internal/model/forward_panel.go` -> `ForwardUserTunnel`
- base tests:
  - `internal/service/forward_panel_service_test.go`

As of `2026-04-06`, the compat route surface additionally includes:

- `POST /api/v2/user/reset`
- `POST /api/v2/tunnel/user/assign`
- `POST /api/v2/tunnel/user/list`
- `POST /api/v2/tunnel/user/remove`
- `POST /api/v2/tunnel/user/update`

## Current Completion Status

Already aligned:

- forward page main UI
- `/forward/*` compat endpoints
- `/user/reset` compat endpoint
- `/tunnel/user/tunnel` compat endpoint
- `/tunnel/user/assign|list|remove|update` compat endpoints
- response wrapper `code/msg/ts/data`
- explicit `ForwardUserTunnel` auth model
- tunnel list fields required by the current forward page
- admin user page tunnel grant table now shows used flow, monthly reset day, rate-limit column, and manual reset action

Still incomplete:

1. full `UserTunnel` management semantics
    - Flux sources `inFlow/outFlow` from the relation record and joins `speed_limit`
    - Flux sorts the list by relation id ascending
    - create flow should filter already-granted tunnels and use a speed-limit selector
2. forward runtime semantics
    - create/update/delete/pause/resume side effects on remote services
3. tunnel / forward diagnose node-chain semantics
4. quota / expire / flow reset behavior tied to `UserTunnel`
    - monthly automatic reset scheduler still missing
    - reset confirmation dialogs are still simplified `confirm/alert`

## Local Dual-runtime Extension Rules

These items are local extensions added to support a second runtime path (`iptables_ansible`) while the main Flux clone work continues:

- config key: `forward.runtime_backend`
- config key: `forward.runtime.iptables_ansible.config`
- admin runtime job endpoint: `GET /api/v2/admin/forward/runtime/jobs`
- admin UI location: `web/src/views/admin/System.vue`
- runtime worker: started automatically by `cmd/server/main.go` and consumes pending `iptables_ansible` jobs in-process
- Docker bootstrap extension: `install.sh` (`panel_install.sh` wrapper)
- bundled ansible deployment assets: `config/deploy/ansible/`
- env bootstrap keys: `FORWARD_RUNTIME_BACKEND`, `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`

Guardrails:

- do not add runtime backend selectors or runtime job tables to `web/src/views/admin/Forward.vue`
- keep `/admin/forward` aligned with `vite-frontend/src/pages/forward.tsx`
- document runtime-side differences separately instead of treating the extension UI as part of the Flux clone itself

## Module Status Board

| Module | Reference Surface | Local Status | Notes |
|------|------|------|------|
| forward page | `forward.tsx` + `/api/v1/forward/*` | partial clone | main page and compat endpoints exist |
| tunnel selector for forward | `/api/v1/tunnel/user/tunnel` | partial clone | current DTO is aligned for the page |
| user reset | `/api/v1/user/reset` | partial clone | route and wrapper exist; Flux dialog flow still differs |
| user-tunnel admin management | `/api/v1/tunnel/user/assign|list|remove|update` | partial clone | routes and first-pass UI exist, but detail semantics and dialogs still differ |
| flow and quota side effects | `FlowController.java` | not started | runtime pause/disable paths still not cloned |
| diagnose runtime chain | `ForwardServiceImpl.java` and `TunnelServiceImpl.java` | partial clone | panel-side checks exist, node-chain semantics do not |

## DTO Surface That Must Not Drift

| DTO / response shape | Fields that must remain explicit |
|------|------|
| `ForwardDto` | `name`, `tunnelId`, `remoteAddr`, `strategy`, `inPort`, `interfaceName` |
| `ForwardUpdateDto` | `id`, `userId`, `name`, `tunnelId`, `remoteAddr`, `strategy`, `inPort`, `interfaceName` |
| `ForwardWithTunnelDto` | `id`, `name`, `inPort`, `remoteAddr`, `status`, `createdTime`, `updatedTime`, `tunnelName`, `inIp`, `userName`, `userId`, `tunnelId`, `inFlow`, `outFlow`, `strategy`, `inx`, `interfaceName` |
| `TunnelListDto` | `id`, `name`, `ip`, `inNodePortSta`, `inNodePortEnd`, `type`, `protocol` |
| `TunnelDto` | `name`, `inNodeId`, `outNodeId`, `type`, `flow`, `trafficRatio`, `interfaceName`, `protocol`, `tcpListenAddr`, `udpListenAddr` |
| `TunnelUpdateDto` | `id`, `name`, `flow`, `trafficRatio`, `protocol`, `tcpListenAddr`, `udpListenAddr`, `interfaceName` |
| `UserTunnelDto` | `userId`, `tunnelId`, `flow`, `num`, `flowResetTime`, `expTime`, `speedId` |
| `UserTunnelQueryDto` | `userId` |
| `UserTunnelUpdateDto` | `id`, `flow`, `num`, `flowResetTime`, `expTime`, `status`, `speedId` |
| `UserTunnelWithDetailDto` | `id`, `userId`, `tunnelId`, `flow`, `num`, `flowResetTime`, `expTime`, `speedId`, `speedLimitName`, `speed`, `tunnelName`, `tunnelFlow`, `inFlow`, `outFlow`, `status` |

Extra rules:

- `ForwardUpdateDto.userId` must stay in the clone even if local code can infer the user from token context.
- `UserTunnelUpdateDto.status`, `UserTunnelUpdateDto.speedId`, and `ForwardWithTunnelDto.inx` are required fields, not optional conveniences.

## Auth And Permission Semantics

- All nine `ForwardController` routes are user-scope in the reference. They are not admin-only just because the file imports role annotations.
- Under `TunnelController`, only `POST /api/v1/tunnel/user/tunnel` is user-scope. Tunnel CRUD, tunnel diagnose, and user-tunnel management are role-restricted.
- The user tunnel list behavior is not "active relation only." The reference collects the user's tunnel relations and then filters by active tunnel status; relation `status` is not the list filter itself.
- Non-admin `createForward`, `updateForward`, and `resumeForward` do more than check relation existence. The reference also checks user expiry, relation status, relation expiry, user total flow, tunnel flow, user forward count, and per-tunnel forward count.
- `updateForwardOrder` is owner-scoped for normal users and cross-user for admins.
- When admins change someone else's forward and switch tunnels, the reference re-validates that target user's `UserTunnel` grant is still enabled, unexpired, and within quota.

## Frontend-Coupled Request Semantics

- Multi-line `remoteAddr` input is normalized into a comma-separated string before submit.
- If there is only one target address, the frontend forces `strategy` to `fifo`.
- Drag-sort payload stays `{ "forwards": [{ "id": 1, "inx": 0 }] }`.
- The "direct" ordering flow persists order only for the current user's forwards, not the whole dataset.
- Diagnose modals expect top-level fields such as `forwardName`, `timestamp`, and `results[]`, and each result item needs `success`, `description`, `nodeName`, `nodeId`, `targetIp`, `targetPort`, `message`, `averageTime`, and `packetLoss`.
- `POST /api/v1/user/reset` uses `{ "id": <id>, "type": 1|2 }`; success keeps `data = null`, errors use `code = -1` with `data = null`.
- `UserTunnelDto` create flow must not send `status`; status belongs to `UserTunnelUpdateDto`.
- The Flux user page filters already-assigned tunnels from the create selector and uses a speed-limit picker backed by `/speed-limit/list`; a raw numeric `speedId` input is not considered aligned.

## Detailed Runtime Gap Checklist

| Surface | Flux semantics that must be matched | Current local gap |
|------|------|------|
| `forward/create` | save DB state, create runtime, rollback DB on runtime failure | local clone is still mainly DB-layer |
| `forward/update` | rebuild runtime when tunnel/runtime config changes and mark forward error on runtime failure | runtime error state is not fully cloned |
| `forward/delete` | delete runtime before deleting DB record | local delete can still be DB-first semantics |
| `forward/force-delete` | DB-only escape hatch that skips runtime deletion | this branch must stay distinct from normal delete |
| `forward/pause` / `resume` | update runtime services and re-check permissions/quota on resume | remote side effects and resume pre-checks are still incomplete |
| `forward/diagnose` | different node-chain behavior for direct vs tunnel forwarding | local diagnosis is still panel-side |
| `tunnel/update` | replay/update forwards under the tunnel when key runtime fields change | cascade behavior not cloned |
| `tunnel/delete` | block delete if forwards or user-tunnel grants still depend on the tunnel | delete guard semantics not cloned |
| `tunnel/diagnose` | direct and tunnel modes diagnose different network paths | current docs are still too coarse without this reminder |
| `user-tunnel/remove` | stop and remove that user's forwards under the tunnel before removing the grant | cascade delete behavior not cloned |
| `user-tunnel/update` | propagate speed/limit changes to runtime for affected forwards | runtime propagation not cloned |

## Definition Of Done

A `flux-panel` clone task is only done when all relevant items are true:

- routes match the reference
- auth scope matches the reference
- DTO field names match the reference
- response wrapper matches the reference
- page flow matches the reference
- service semantics match the reference
- remaining gaps are explicitly documented
- validation commands pass

## Required Validation

Minimum validation:

```bash
$env:GOWORK='off'; go test ./internal/router ./internal/handler ./internal/service
cd web && npm run build
```

If the task touches forward/tunnel behavior, also verify:

- route scope is still correct
- handler still returns `code/msg/ts/data`
- DTO fields were not silently removed
- user routes still work under JWT user context
- the remaining gap list is updated

## Recommended Next Clone Order

1. `UserTunnel` management and authorization UI
2. forward runtime semantics
3. tunnel / forward diagnose runtime semantics
4. quota / expire / reset-flow linkage

## Documentation Maintenance

Every time a `flux-panel` module is cloned or significantly adjusted, update:

- `AGENTS.md`
- `CLAUDE.md`
- `docs/guide/flux-panel-clone.md`
- `docs/guide/flux-forward-contract.md` when forward/tunnel contracts change
- `docs/guide/api-reference.md` when exposed compat endpoints or DTOs change
- `docs/FEATURE_ROADMAP.md` when clone status or remaining gaps change
- `docs/guide/flux-panel-workstream.md` when the next recommended clone order changes

If the change is partial, update the completion status and gap list anyway.
