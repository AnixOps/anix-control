# Runtime Modes

This page is the reference entrypoint for the forward runtime split in `v2board_AnixOps`.

## One Important Truth

Creating a `ForwardNode` under `/admin/forward/nodes` does not mean the relay has already joined the execution plane.

A `ForwardNode` record only stores metadata used later by runtime jobs. Real attachment happens only when the selected runtime path can execute successfully:

- `NodeX/gost`
  - `v2board -> NodeX control-plane -> relay gost API`
- `iptables_ansible`
  - `v2board local executor -> ansible-playbook -> SSH/inventory -> relay host`

## Control Plane Vs Execution Plane

- `v2board_AnixOps`
  - public control plane
  - admin UI
  - persistence
  - `/api/v2/admin/forward/runtime/*` diagnostics and job views
- `NodeX`
  - internal-only execution control plane
  - stateful `gost` runtime
  - doctor, version, and operator tooling
- `iptables_ansible`
  - stateless execution path
  - local background executor inside `v2board`
  - `ansible-playbook` + SSH + playbook driven forwarding

## Mode Split

### NodeX Mode

Use this when you want the private stateful runtime.

Required bootstrap:

```env
FORWARD_RUNTIME_NODEX_MODE=true
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18081
FORWARD_RUNTIME_NODEX_TOKEN=replace-with-shared-token
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=15
```

Operational implications:

- `InNodeID` ingress semantics are valid only in this mode
- `forward.runtime.nodex.base_url` and `forward.runtime.nodex.token` are required
- in the current verified single-UI deployment, NodeX uses `18081` while the relay gost API uses `18080`
- the relay `ForwardNode` must expose `host`, `api_port`, and `api_token`
- the relay host must already be running a reachable gost management API
- attachment is not complete until NodeX can create or update relay services and limiters

Actual flow:

1. `v2board` builds a runtime request from the selected forward and tunnel.
2. `v2board` calls NodeX at `/api/v2/internal/forward/runtime/execute`.
3. NodeX uses the relay `ForwardNode.host`, `ForwardNode.api_port`, and `ForwardNode.api_token`.
4. NodeX writes to relay gost endpoints such as `/api/config/services` and `/api/config/limiters`.

If `v2board` runs inside Docker, do not point `FORWARD_RUNTIME_NODEX_BASE_URL` at `127.0.0.1` unless NodeX is in the same network namespace. Use the actual reachable host or service name instead.

### `iptables_ansible` Mode

Use this when you want stateless relay execution without NodeX.

Required bootstrap:

```env
FORWARD_RUNTIME_NODEX_MODE=false
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"config/deploy/ansible/inventory.ini","playbookApply":"config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"environment":{"ANSIBLE_CONFIG":"config/deploy/ansible/ansible.cfg"}}
```

Operational implications:

- no NodeX control-plane is required
- no node-side self-register flow is required
- the real dependency is `ansible-playbook` on the `v2board` executor host
- SSH credentials are provided by inventory files or env-generated inventory, not by `ForwardNode` table columns
- the selected execution target is resolved from the tunnel, then targeted through inventory or `targetPattern`
- attachment is complete only after the local executor runs the playbook successfully and the relay host has the expected `iptables` rules

Actual flow:

1. `v2board` stores a pending runtime job in `v2_forward_runtime_job`.
2. The local background executor polls pending `iptables_ansible` jobs.
3. The executor runs `ansible-playbook`.
4. The playbook uses inventory or env-generated inventory to reach the relay host.
5. The relay host receives or removes `iptables` rules.

## Current Health Semantics

The current forward-node online indicator is intentionally limited.

- panel health check only tests TCP reachability to `ForwardNode.host:ForwardNode.port`
- a green node in the UI does not prove:
  - NodeX is reachable
  - gost API on `api_port` is healthy
  - `api_token` matches the relay
  - ansible can SSH into the host
  - the host already has active forward rules

Treat `/admin/forward/nodes` online status as a coarse reachability signal, not as runtime attachment proof.

## Operator Entry Points

Use these together:

- startup and config: [`startup-config.md`](startup-config.md)
- config source-of-truth: [`configuration.md`](configuration.md)
- control-plane evidence:
  - `/admin/system`
  - `/api/v2/admin/forward/runtime/status`
  - `/api/v2/admin/forward/runtime/doctor`
  - `/api/v2/admin/forward/runtime/jobs`
- relay onboarding: [`../guide/forward-relay-onboarding.md`](../guide/forward-relay-onboarding.md)
- runtime operations: [`../guide/forward-tunnel-runtime-ops.md`](../guide/forward-tunnel-runtime-ops.md)
- manual smoke tests: [`../guide/forward-tunnel-smoke-test.md`](../guide/forward-tunnel-smoke-test.md)
- NodeX boundary: [`../guide/nodex-internal-extension.md`](../guide/nodex-internal-extension.md)

## Resource Naming Rule

Do not mix these resource types in docs or UI copy:

- `/admin/nodes`
  - proxy-node inventory
- `/admin/forward/nodes`
  - forward execution nodes
- `/admin/forward/tunnel`
  - tunnel inventory bound to forward execution behavior
