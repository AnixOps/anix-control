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
- `springboot-backend/src/main/java/com/admin/controller/FlowController.java`
- `springboot-backend/src/main/java/com/admin/service/impl/ForwardServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/TunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/UserTunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/common/dto/TunnelListDto.java`
- `vite-frontend/src/pages/forward.tsx`
- `vite-frontend/src/pages/tunnel.tsx`
- `vite-frontend/src/pages/user.tsx`

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
| `POST /api/v1/tunnel/user/assign` | role-restricted | `POST /api/v2/tunnel/user/assign` | JWT user + `AdminAuth` | partially aligned | route exists; create DTO matches Flux, but UI still lacks speed-limit picker and duplicate filtering |
| `POST /api/v1/tunnel/user/list` | role-restricted | `POST /api/v2/tunnel/user/list` | JWT user + `AdminAuth` | partially aligned | field names match; `inFlow/outFlow`, speed-limit values, and sort order still differ |
| `POST /api/v1/tunnel/user/remove` | role-restricted | `POST /api/v2/tunnel/user/remove` | JWT user + `AdminAuth` | partially aligned | route exists; cascade semantics on affected forwards are still local-only |
| `POST /api/v1/tunnel/user/update` | role-restricted | `POST /api/v2/tunnel/user/update` | JWT user + `AdminAuth` | partially aligned | route exists; runtime speed propagation and detailed UI flow still differ |

## Remaining Clone Gaps

These gaps still block a full 1:1 clone even though the base compat routes now exist:

| Surface | Local Status | Notes |
|------|------|------|------|
| `POST /api/v2/tunnel/user/list` detail semantics | partially aligned | Flux reads `inFlow/outFlow` from `user_tunnel`, joins `speed_limit`, and sorts by relation id ascending; local code still aggregates flow from `v2_forward`, synthesizes `speedLimitName`, and orders by `id DESC` |
| `POST /api/v2/tunnel/user/assign` UI | partially aligned | Flux create flow does not send `status`; it filters already-assigned tunnels and uses a speed-limit selector rather than a raw numeric `speedId` input |
| reset confirmation flow | partially aligned | Flux shows dedicated reset modals with current flow summary for user flow and tunnel flow; local page still uses `confirm/alert` |
| monthly flow reset automation | not cloned | `flowResetTime` is stored and displayed, but automatic reset scheduling is still missing |
| flow side effects in `FlowController` | not cloned | quota exhaustion, expire and disable behavior still do not mirror the reference runtime path |

## Local Extension Surface

The following pieces are local extensions for dual-runtime support and are not part of the upstream Flux `/forward` page contract:

- `GET /api/v2/admin/forward/runtime/jobs`
- system config key `forward.runtime_backend`
- system config key `forward.runtime.iptables_ansible.config`
- background worker started by `cmd/server/main.go` that executes pending `iptables_ansible` jobs

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

- Flux sources `inFlow/outFlow` from `user_tunnel`; local code still derives them by summing forward rows under the same user/tunnel pair.
- Flux joins `speed_limit` to expose display values; local code still synthesizes `speedLimitName` and falls back to user-level speed.

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
| user-tunnel quota | active flow/expire/status linkage | partially cloned; reset route and grant checks exist, but automatic monthly reset and stored relation counters are still missing |
| user-tunnel admin UI | speed-limit selector, reset dialogs, filtered tunnel picker | partially cloned; local page shows used flow/reset action/rate column but still uses simplified controls |
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
