# Forwarding Compatibility

This document records compatibility boundaries for the forwarding module. It is
not a claim that the upstream product is fully cloned.

## Clean-Room Rule

The forwarding product direction may use external panels as product references,
but this repository must not copy AGPL implementation code, assets, styles, or
proprietary text. Local code and docs must stay clean-room and independently
implemented.

Existing clean-room notes:

- `docs/forward-clean-room/provenance.md`
- `docs/forward-clean-room/spec.md`

## Flux-Shaped Surface

The current Flux-shaped compatibility target is centered on:

- forward CRUD
- tunnel list and admin tunnel management
- user-tunnel grants
- user flow reset
- speed-limit CRUD
- monthly flow reset semantics
- Flux-style response envelope

The canonical detailed contract remains:

- `docs/guide/flux-forward-contract.md`

## Route Mapping

Local compatible routes use `/api/v2`, not the original `/api/v1` prefix.

User routes:

- `/api/v2/forward/create`
- `/api/v2/forward/list`
- `/api/v2/forward/update`
- `/api/v2/forward/delete`
- `/api/v2/forward/force-delete`
- `/api/v2/forward/pause`
- `/api/v2/forward/resume`
- `/api/v2/forward/diagnose`
- `/api/v2/forward/update-order`
- `/api/v2/tunnel/user/tunnel`

Admin mirrors and admin resources:

- `/api/v2/admin/forward/*`
- `/api/v2/admin/tunnel/*`
- `/api/v2/tunnel/user/assign`
- `/api/v2/tunnel/user/list`
- `/api/v2/tunnel/user/remove`
- `/api/v2/tunnel/user/update`
- `/api/v2/user/reset`
- `/api/v2/speed-limit/*`

Admin mirror routes are local convenience surfaces. Keep authorization semantics
explicit when adding or changing mirrors.

## Response Envelope

Flux-shaped routes return:

```json
{
  "code": 0,
  "msg": "操作成功",
  "ts": 1712300000000,
  "data": {}
}
```

Errors return:

```json
{
  "code": -1,
  "msg": "error message",
  "ts": 1712300000000,
  "data": null
}
```

Routes outside the compatibility surface may still use older local response
shapes. Do not rely on a Flux envelope unless the handler uses `panelSuccess` or
`panelError`.

## DTO Compatibility

The local DTOs intentionally preserve Flux-style field names where the UI expects
them:

- `tunnelId`
- `inPort`
- `remoteAddr`
- `interfaceName`
- `userId`
- `flowResetTime`
- `expTime`
- `speedId`
- `runtimeBackend`
- `runtimeStatus`
- `runtimeMessage`
- `lastRuntimeSyncTime`

Keep these names stable for the existing Vue pages and compatibility clients.
If a new local field is needed, add it without renaming existing fields.

## Behavior Compatibility

Currently aligned or partially aligned:

- User forward CRUD and ordering.
- User and admin forward route mirrors.
- Tunnel listing for user-visible grants.
- Tunnel grant assignment, listing, update, and removal.
- Duplicate tunnel grant filtering.
- Flow reset day semantics, including last-day handling for short months.
- Expired user-tunnel grant disabling with affected forward pause behavior.
- Speed-limit CRUD and tunnel-scoped selection.

Not fully aligned:

- Exact Flux UI layout parity.
- Full upstream user-disable state model.
- All flow controller side effects.
- Native relation counter writes for every user-tunnel flow path.
- Runtime-side speed-limit propagation.
- Exact diagnose semantics across all runtime paths.

Do not describe partially aligned behavior as complete clone parity.

## Local Extension Surface

The following surfaces are local extensions, not Flux clone proof:

- `GET /api/v2/admin/forward/runtime/jobs`
- `GET /api/v2/admin/forward/runtime/status`
- `GET /api/v2/admin/forward/runtime/doctor`
- `GET /api/v2/admin/forward/local/status`
- `GET /api/v2/admin/forward/local/doctor`
- `GET /api/v2/admin/forward/nodex/status`
- `GET /api/v2/admin/forward/nodex/doctor`
- `POST /api/v2/admin/forward/sync-backend`
- `/api/v2/forward-agent/*`
- `/api/v2/admin/forward/agents*`
- `/api/v2/internal/forward/traffic/*`
- clean-agent bridge and diagnostic routes

These are necessary for the v2board runtime architecture, but they should stay
outside the cloned page contract unless the UI clearly labels them as local
operator tools.

## Runtime Compatibility

Runtime backend names are stable local API values:

- `gost`
- `nftables_ansible`
- `iptables_ansible`
- `clean_agent`

Mode semantics:

- `gost` means panel to NodeX to relay gost API.
- `nftables_ansible` means local panel executor to Ansible to relay nftables.
- `iptables_ansible` means local panel executor to Ansible to legacy iptables.
- `clean_agent` means agent pulls jobs and applies local changes on its node.

Compatibility warning:

`ForwardNode.status = online` only proves coarse `host:port` reachability. It is
not compatible with, or equivalent to, runtime attachment state.

## Migration Notes

When migrating clients from Flux-shaped `/api/v1` expectations:

1. Use `/api/v2` local route prefixes.
2. Preserve request field names documented in `docs/forwarding/api.md`.
3. Treat admin mirror routes as admin-only even when the shape matches user
   routes.
4. Verify response envelopes per route group.
5. Verify runtime attachment through job and relay evidence, not only list
   responses.
6. Do not assume speed-limit runtime enforcement is complete just because CRUD
   exists.

## Completion Criteria For Future Clone Work

A compatibility item can be marked complete only when there is evidence for all
of these:

- route path, method, auth scope, and request fields match the chosen local
  contract
- response envelope and DTO field names are stable
- service behavior matches the documented semantics
- UI flow consumes the route as intended
- tests cover success, validation failure, auth failure, and ownership failure
- docs identify any intentional local divergence
