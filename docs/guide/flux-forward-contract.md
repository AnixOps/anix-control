# Flux Forward Contract

## Scope

This document tracks the concrete forward, tunnel and user-tunnel compatibility surface that has already started cloning `flux-panel`.

Use it before changing:

- `internal/router/router.go`
- `internal/handler/forward_panel.go`
- `internal/service/forward_panel_service.go`
- `internal/model/forward_panel.go`
- `web/src/api/admin.js`
- `web/src/views/admin/Forward.vue`

## Reference Sources

- `springboot-backend/src/main/java/com/admin/controller/ForwardController.java`
- `springboot-backend/src/main/java/com/admin/controller/TunnelController.java`
- `springboot-backend/src/main/java/com/admin/controller/UserController.java`
- `springboot-backend/src/main/java/com/admin/controller/SpeedLimitController.java`
- `springboot-backend/src/main/java/com/admin/controller/FlowController.java`
- `springboot-backend/src/main/java/com/admin/service/impl/UserServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/ForwardServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/TunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/UserTunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/SpeedLimitServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/common/task/ResetFlowAsync.java`
- `springboot-backend/src/main/java/com/admin/common/dto/TunnelListDto.java`
- `vite-frontend/src/pages/forward.tsx`
- `vite-frontend/src/pages/tunnel.tsx`
- `vite-frontend/src/pages/user.tsx`
- `vite-frontend/src/pages/limit.tsx`
- `vite-frontend/src/api/index.ts`

Local reference repo:

- `C:\Users\z7299\AppData\Local\Temp\flux-panel`

## Response Envelope

Expected compat envelope:

```json
{
  "code": 0,
  "msg": "操作成功",
  "ts": 1712300000000,
  "data": {}
}
```

Expected compat error envelope:

```json
{
  "code": -1,
  "msg": "请求失败",
  "ts": 1712300000000,
  "data": null
}
```

## Endpoint Mapping

| Flux Endpoint | Flux Scope | Local Route | Local Scope | Local Status | Notes |
|------|------|------|------|------|------|
| `POST /api/v1/forward/create` | JWT user | `POST /api/v2/forward/create` | JWT user | available | `/api/v2/admin/forward/create` mirror also exists |
| `POST /api/v1/forward/list` | JWT user | `POST /api/v2/forward/list` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/update` | JWT user | `POST /api/v2/forward/update` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/delete` | JWT user | `POST /api/v2/forward/delete` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/force-delete` | JWT user | `POST /api/v2/forward/force-delete` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/pause` | JWT user | `POST /api/v2/forward/pause` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/resume` | JWT user | `POST /api/v2/forward/resume` | JWT user | available | admin mirror also exists |
| `POST /api/v1/forward/diagnose` | JWT user | `POST /api/v2/forward/diagnose` | JWT user | partially aligned | route exists, runtime semantics still differ |
| `POST /api/v1/forward/update-order` | JWT user | `POST /api/v2/forward/update-order` | JWT user | available | admin mirror also exists |
| `POST /api/v1/user/reset` | role-restricted | `POST /api/v2/user/reset` | JWT user + `AdminAuth` | available | request body `{id,type}` is cloned; success keeps `data: null` |
| `POST /api/v1/tunnel/user/tunnel` | JWT user, not admin-only | `POST /api/v2/tunnel/user/tunnel` | JWT user | available | admin mirror also exists |
| `POST /api/v1/tunnel/user/assign` | role-restricted | `POST /api/v2/tunnel/user/assign` | JWT user + `AdminAuth` | partially aligned | route exists; duplicate-tunnel filtering, tunnel-scoped speed-limit selection, and `speedId` validation are live, but runtime side effects still differ |
| `POST /api/v1/tunnel/user/list` | role-restricted | `POST /api/v2/tunnel/user/list` | JWT user + `AdminAuth` | partially aligned | field names, ascending relation order, and joined speed-limit names are aligned; used flow still depends on compat backfill from `v2_forward` |
| `POST /api/v1/tunnel/user/remove` | role-restricted | `POST /api/v2/tunnel/user/remove` | JWT user + `AdminAuth` | partially aligned | route exists; cascade semantics on affected forwards are still local-only |
| `POST /api/v1/tunnel/user/update` | role-restricted | `POST /api/v2/tunnel/user/update` | JWT user + `AdminAuth` | partially aligned | route exists; request body keeps `speedId`, tunnel-scoped validation exists, but runtime speed propagation and detailed UI flow still differ |

## Adjacent User-page Resources

These resources are part of the real Flux user-page surface even though they are not `/forward/*` routes:

| Flux Resource | Flux Scope | Local Status | Notes |
|------|------|------|------|
| `POST /api/v1/speed-limit/list` | role-restricted | available | local backend now serves `/api/v2/speed-limit/list` with Flux-style `code/msg/ts/data` envelope |
| `SpeedLimit` resource (`/speed-limit/create|list|update|delete`) | role-restricted | partially aligned | backend model/service/handler/routes exist, `/api/v2/speed-limit/tunnels` exists, and the standalone admin page now lives at `/admin/limit`; remaining gaps are exact `limit.tsx` layout parity and runtime-side limiter propagation |
| `ResetFlowAsync` scheduled monthly reset | backend task | partially aligned | local `ForwardFlowResetWorker` now resets users and user-tunnel grants, handles month-end overflow days, pauses expired-user forwards, and disables expired grants; the remaining gap is exact parity with Flux's dedicated user-disable state model |

## Flow Reset Semantics

Upstream Flux behavior:

- `ResetFlowAsync` runs daily at `00:00:05`.
- `flowResetTime = 0` means "do not auto reset".
- `flowResetTime = 1..31` means "reset on that day of the month".
- If the configured day does not exist in the current month, Flux resets on the last day of that month.
- The scheduler resets both user flow and user-tunnel flow, then separately disables expired users and expired user-tunnel grants.

Current local behavior:

- `POST /api/v2/user/reset` supports the same manual `{id,type}` contract.
- `type = 1` resets user flow; `type = 2` resets user-tunnel flow.
- `flowResetTime` is stored and displayed with the Flux-style monthly-day meaning in the admin user page.
- Startup now launches `ForwardFlowResetWorker`, performs one catch-up run, and then schedules daily `00:00:05` local-time monthly-reset scans.
- The worker resets both user flow and user-tunnel flow, and month-end overflow days (`31` on short months) are handled.
- After the reset scan, expired users have their active forwards paused, and expired user-tunnel grants have their active forwards paused before the grant is disabled.
- Expired login is rejected through `auth_service`, so the remaining reset gap is exact parity with Flux's user-disable state mutation rather than the scheduler itself.
- Manual reset only clears flow counters; it must not be documented as proof that quota-triggered pauses or monthly resume semantics are fully cloned.

## Current User-page Linkage

The current local operator surface for Flux user-page work is `web/src/views/admin/Users.vue`.

Already wired:

- `POST /api/v2/user/reset`
- `POST /api/v2/tunnel/user/assign`
- `POST /api/v2/tunnel/user/list`
- `POST /api/v2/tunnel/user/remove`
- `POST /api/v2/tunnel/user/update`
- standalone `/admin/limit` CRUD for `SpeedLimit`
- duplicate-tunnel filtering on create
- read-only tunnel selection while editing an existing grant
- used-flow summary and dedicated confirm modals for user flow and user-tunnel flow resets
- rate-limit column, reset-day column, and used-flow column in the grant table

Still not wired to the full Flux resource graph:

- native relation counter writes for `ForwardUserTunnel.inFlow/outFlow`
- exact `user.tsx` / `limit.tsx` layout and interaction parity
- runtime-side propagation of speed-limit rules beyond CRUD/resource selection

## Remaining Clone Gaps

These gaps still block a full 1:1 clone even though the base compat routes now exist:

| Surface | Local Status | Notes |
|------|------|------|
| `POST /api/v2/tunnel/user/list` detail semantics | partially aligned | Flux reads `inFlow/outFlow` from `user_tunnel`; local code now joins real `speed_limit` data and sorts by relation id ascending, but flow counters still rely on compat backfill from `v2_forward` rather than native relation writes |
| `POST /api/v2/tunnel/user/assign` and `update` UI | partially aligned | duplicate-tunnel filtering, edit-locking, reset dialogs, and tunnel-scoped speed-limit selection are in place; the remaining gap is exact Flux visual/layout parity |
| real speed-limit resource | partially aligned | backend `/api/v2/speed-limit/*` and standalone `/admin/limit` CRUD are live; remaining gaps are exact `limit.tsx` layout parity and runtime-side limiter propagation |
| reset confirmation and reset semantics | partially aligned | dedicated confirm modals, scheduled monthly reset, expired-user forward pause, and expired-grant pause+disable are live; remaining gaps are exact Flux user-state parity and other `FlowController` side effects |
| monthly flow reset automation | partially aligned | `flowResetTime` is stored, displayed, and scheduled; remaining gaps are exact Flux user disable-state parity and deeper quota/runtime linkage |
| flow side effects in `FlowController` | not cloned | quota exhaustion, expire, disable, and runtime pause behavior still do not mirror the reference runtime path |

## Local Extension Surface

The following pieces are local extensions for dual-runtime support and are not part of the upstream Flux `/forward` page contract:

- `GET /api/v2/admin/forward/runtime/jobs`
- system config key `forward.runtime_backend`
- system config key `forward.runtime.iptables_ansible.config`
- optional async executor implementation in `internal/service/forward_runtime_job_executor.go`

Rules for this extension surface:

- keep the main forward clone UI in `web/src/views/admin/Forward.vue` aligned with Flux
- place runtime backend selection and runtime job observability in `web/src/views/admin/System.vue`
- do not describe these extension endpoints as proof that the Flux forward page clone is complete

## Request DTOs

### Create Forward

```json
{
  "name": "Forward-A",
  "tunnelId": 1,
  "inPort": 10001,
  "remoteAddr": "example.com:443",
  "interfaceName": "",
  "strategy": "fifo"
}
```

### Update Forward

```json
{
  "id": 1,
  "userId": 2,
  "name": "Forward-A",
  "tunnelId": 1,
  "inPort": 10001,
  "remoteAddr": "example.com:443",
  "interfaceName": "",
  "strategy": "fifo"
}
```

### Delete / Pause / Resume / Force Delete

```json
{
  "id": 1
}
```

### Diagnose

```json
{
  "forwardId": 1
}
```

### Update Order

```json
{
  "forwards": [
    { "id": 1, "inx": 0 },
    { "id": 2, "inx": 1 }
  ]
}
```

### Reset Flow

```json
{
  "id": 1,
  "type": 1
}
```

`type = 1` resets user flow, `type = 2` resets user-tunnel flow.

### Assign User Tunnel

```json
{
  "userId": 2,
  "tunnelId": 1,
  "flow": 100,
  "num": 10,
  "flowResetTime": 0,
  "expTime": 1712300000000,
  "speedId": null
}
```

### List User Tunnel

```json
{
  "userId": 2
}
```

### Remove User Tunnel

```json
{
  "id": 1
}
```

### Update User Tunnel

```json
{
  "id": 1,
  "flow": 100,
  "num": 10,
  "flowResetTime": 0,
  "expTime": 1712300000000,
  "status": 1,
  "speedId": null
}
```

## Response DTOs

### Tunnel List Item

Fields required by the current clone:

| Field | Meaning | Status |
|------|------|------|
| `id` | tunnel id | aligned |
| `name` | tunnel name | aligned |
| `ip` | ingress IP from tunnel | aligned |
| `inNodePortSta` | ingress node port range start | aligned |
| `inNodePortEnd` | ingress node port range end | aligned |
| `type` | tunnel type | aligned |
| `protocol` | tunnel protocol | aligned |

Current local extras:

- `inIp`
- `status`

Those extras may stay, but new pages must not depend on them instead of the Flux fields above.

### Forward List Item

Current local compat fields:

- `id`
- `name`
- `tunnelId`
- `tunnelName`
- `inIp`
- `inPort`
- `remoteAddr`
- `interfaceName`
- `strategy`
- `status`
- `inFlow`
- `outFlow`
- `createdTime`
- `updatedTime`
- `userName`
- `userId`
- `inx`

### UserTunnel Detail Item

Fields currently required by the Flux-cloned admin user page:

| Field | Meaning | Status |
|------|------|------|
| `id` | user-tunnel relation id | aligned |
| `userId` | owner id | aligned |
| `tunnelId` | tunnel id | aligned |
| `flow` | tunnel grant quota | aligned |
| `num` | forward count quota | aligned |
| `flowResetTime` | monthly reset day | aligned for storage/display |
| `expTime` | relation expiry timestamp | aligned |
| `speedId` | speed-limit rule id | aligned |
| `status` | relation status | aligned on update |
| `tunnelName` | tunnel display name | aligned |
| `tunnelFlow` | tunnel quota | aligned |
| `inFlow` / `outFlow` | used flow counters | partially aligned |
| `speedLimitName` / `speed` | speed-limit display | partially aligned |

Current known differences:

- Flux sources `inFlow/outFlow` from `user_tunnel`; local code still derives those values from aggregated `v2_forward` traffic because no native relation counter writer exists yet.
- Flux joins `speed_limit` to expose display values; local code still synthesizes `speedLimitName` from `speedId` and falls back to user-level or plan-level `speed_limit` for the numeric speed.

## Authorization Semantics

### Tunnel List

- admin: all active tunnels
- non-admin: tunnels referenced by `ForwardUserTunnel`, then filtered by active tunnel status

Important note:

- local list semantics intentionally do not filter by `ForwardUserTunnel.status`
- this matches the Flux `userTunnel()` pattern, which uses the relation to collect tunnel ids and then filters on tunnel status

### Create / Update Forward

- non-admin create/update still requires an active `ForwardUserTunnel` relation
- list visibility and mutation permission are not the same semantic check

Do not "simplify" this distinction unless the reference changes.

## Known Runtime Gaps

| Area | Flux Reference | Local Current State |
|------|------|------|
| create/update/delete | changes Gost / remote runtime | mainly DB compatibility layer |
| pause/resume | runtime side effects + persistence | mainly local status persistence |
| diagnose | node-chain diagnosis | mostly panel-side direct dialing |
| user-tunnel quota | active flow/expire/status linkage | partially cloned; reset route, grant checks, monthly reset scheduling, expired-user forward pause, and expired-grant disablement exist, but stored relation counters and full `FlowController` parity are still missing |
| user-tunnel admin UI | speed-limit selector, reset dialogs, filtered tunnel picker | partially cloned; local pages now include the speed-limit selector and dedicated reset modals, but exact `user.tsx` / `limit.tsx` visual flow still differs |
| flow reporting | controller-driven runtime enforcement | not cloned |

## Future Clone Guardrails

- Do not mark forward/tunnel as "done" while `user-tunnel` detail semantics, reset dialogs, or speed-limit UI still diverge from the reference.
- Do not mark the runtime clone as complete until `FlowController`-driven side effects have a local equivalent.
- If local DTOs add helper fields such as `inIp` or `status`, keep the Flux fields alongside them and document the extras.

## Review Checklist

Before merging a forward/tunnel change, verify:

- reference controller path and auth scope
- reference service semantics
- local route scope
- local wrapper fields `code/msg/ts/data`
- DTO field names still match the reference
- user routes still exist outside admin scope when required
- updated tests cover the changed semantics

## Required Validation Commands

```bash
$env:GOWORK='off'; go test ./internal/router ./internal/handler ./internal/service
cd web && npm run build
```
