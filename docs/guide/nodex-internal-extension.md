# NodeX Internal Extension Boundary

`v2board_AnixOps` still clones the Flux `/admin/forward*` surface.

`NodeX` and local ansible execution are internal extensions behind that surface. They are not part of the public Flux contract.

## Boundary Summary

- `v2board_AnixOps`
  - public control plane
  - admin UI
  - persistence
  - runtime job history
- `NodeX`
  - internal-only execution control plane
  - stateful `gost` runtime orchestration
- local ansible execution (`nftables_ansible` default, `iptables_ansible` legacy)
  - local execution path in `v2board` panel host
  - stateless SSH + playbook driven forwarding

## Two Runtime Modes

| Mode | `forward.runtime_backend` | Execution path | What the relay must expose |
|------|------|------|------|
| NodeX mode | `gost` | `v2board -> NodeX -> relay gost API` | `host`, `api_port`, `api_token` |
| local ansible mode | `nftables_ansible` (recommended) / `iptables_ansible` (legacy) | `v2board local executor -> ansible-playbook -> relay host` | SSH reachability through the configured ansible inventory |

## Proxy Nodes Vs Forward Nodes

- `proxy nodes`
  - model: `model.Node`
  - page: `/admin/nodes`
  - purpose: user proxy traffic
- `forward nodes`
  - model: `model.ForwardNode`
  - page: `/admin/forward/nodes`
  - purpose: forward runtime targeting

Do not mix them in operator docs, UI copy, or runtime assumptions.

## Current `ForwardNode` Truth

Current `ForwardNode` records store runtime targeting metadata such as:

- `host`
- `port`
- `api_port`
- `api_token`

Current `ForwardNode` does not contain per-node SSH credential fields.

That means:

- NodeX mode uses relay API access from the `ForwardNode` record
- local ansible mode uses inventory or env-generated SSH inventory outside the `ForwardNode` schema

## Current Online Semantics

The panel forward-node health check is coarse.

- online means only `host:port` TCP reachability
- online does not prove:
  - NodeX is reachable
  - relay gost API is healthy
  - relay API token matches
  - ansible can SSH into the host
  - runtime rules are already applied

## Operator Surfaces

These endpoints belong to the internal runtime layer, not to the Flux-cloned page contract:

- `/api/v2/admin/forward/runtime/jobs`
- `/api/v2/admin/forward/runtime/status`
- `/api/v2/admin/forward/runtime/doctor`
- `/api/v2/admin/forward/nodex/status`
- `/api/v2/admin/forward/nodex/doctor`

They are intentionally documented as operator surfaces, not as Flux-compatible product APIs.

Current admin UI entry split:

- `/admin/forward/ansible-machines`
  - stateless execution-machine inventory
- `/admin/forward/local`
  - local ansible runtime config and diagnostics (`nftables_ansible` recommended)
- `/admin/forward/nodex`
  - dedicated NodeX operator page (stateful control-plane/agent path)
- `/admin/system`
  - shared system page for runtime overview and compatibility controls

## Related Docs

- runtime overview: [`../reference/runtime.md`](../reference/runtime.md)
- relay onboarding: [`forward-relay-onboarding.md`](forward-relay-onboarding.md)
- runtime operations: [`forward-tunnel-runtime-ops.md`](forward-tunnel-runtime-ops.md)
- smoke tests: [`forward-tunnel-smoke-test.md`](forward-tunnel-smoke-test.md)
