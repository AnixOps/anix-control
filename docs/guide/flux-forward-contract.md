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
| `POST /api/v1/tunnel/user/tunnel` | JWT user, not admin-only | `POST /api/v2/tunnel/user/tunnel` | JWT user | available | admin mirror also exists |

## Next-phase Reference Endpoints

These endpoints exist in the Flux reference and should be treated as pending clone scope, not optional extras:

| Flux Endpoint | Flux Scope | Local Status | Notes |
|------|------|------|------|
| `POST /api/v1/tunnel/user/assign` | role-restricted | not cloned | user-tunnel grant flow |
| `POST /api/v1/tunnel/user/list` | role-restricted | not cloned | user-tunnel admin list |
| `POST /api/v1/tunnel/user/remove` | role-restricted | not cloned | removing grants also affects forwards |
| `POST /api/v1/tunnel/user/update` | role-restricted | not cloned | quota, expire, status and rate updates |
| flow side effects in `FlowController` | node/runtime path | not cloned | quota exhaustion, expire and disable behavior |

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
| user-tunnel quota | active flow/expire/status linkage | not fully cloned |
| flow reporting | controller-driven runtime enforcement | not cloned |

## Future Clone Guardrails

- Do not mark forward/tunnel as "done" while `user-tunnel` management endpoints are still missing.
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
