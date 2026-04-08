# Forward/Tunnel Runtime Operations

This guide documents the real runtime behavior behind the Flux-shaped `/admin/forward*` pages.

Do not treat the UI model as proof that a relay is already attached. Attachment is runtime-specific.

## Runtime Ownership

`v2board_AnixOps` owns:

- forward, tunnel, and forward-node persistence
- the admin UI
- runtime job records in `v2_forward_runtime_job`
- NodeX status, doctor, and job proxy endpoints under `/api/v2/admin/forward/runtime/*`

It does not own the relay execution plane itself.

The execution plane is one of:

- `NodeX/gost`
  - `v2board -> NodeX -> relay gost API`
- local ansible
  - `v2board local job executor -> ansible-playbook -> relay host`
  - recommended backend: `nftables_ansible`
  - legacy backend: `iptables_ansible`

## Current Verified Baseline

The current real-machine validated deployment baseline is:

- `v2board` UI on `3000`
- `v2board` API on `8080`
- NodeX control-plane on `18081`
- relay gost API on `18080`
- `binary + SQLite + systemd`

This matters because the port split is easy to misread:

- `forward.runtime.nodex.base_url` must point to the NodeX control-plane
- the relay host `api_port` is for the relay gost management API
- those are different addresses unless you intentionally co-locate them on one machine

## Current Model Truth

Current `ForwardNode` model fields are centered on target identification and relay API access:

- `host`
- `port`
- `api_port`
- `api_token`

Current `ForwardNode` does not store per-node SSH credentials such as:

- `ssh_user`
- `ssh_password`
- `ssh_key`

For local ansible mode, SSH data comes from:

- ansible inventory files
- playbook environment settings

## NodeX/Gost Mode

### Required prerequisites

All of these must be true before a relay is really attached:

1. NodeX control-plane is running.
2. `forward.runtime.nodex.base_url` is configured and reachable from `v2board`.
3. `forward.runtime.nodex.token` matches the NodeX control-plane token.
4. The relay host already exposes a gost management API on `ForwardNode.api_port`.
5. `ForwardNode.api_token` matches what the relay gost API expects.
6. The selected tunnel has a valid ingress node (`InNodeID`).

### Runtime behavior

When a forward is created, updated, resumed, paused, or deleted:

1. `PanelForwardRuntimeService` resolves backend `gost`.
2. It loads the ingress `ForwardNode` from the tunnel.
3. It writes a `backend='gost'` runtime job for audit.
4. It calls NodeX `/api/v2/internal/forward/runtime/execute`.
5. NodeX calls relay gost `/api/config/services` and `/api/config/limiters`.

### Verification

Check all three layers:

1. `v2board`
  - `/api/v2/admin/forward/runtime/status`
  - `/api/v2/admin/forward/runtime/doctor`
  - `SELECT * FROM v2_forward_runtime_job WHERE backend='gost' ORDER BY id DESC LIMIT 10;`
2. `NodeX`
  - `/health`
  - `/api/v2/internal/forward/runtime/status`
3. relay host
  - `curl -u admin:<TOKEN> http://<HOST>:<API_PORT>/api/config/services`
  - confirm the created service and limiter exist

## Local Ansible Mode

### Required prerequisites

All of these must be true before a relay is really attached:

1. `ansible-playbook` exists on the machine running `v2board`.
2. inventory and playbook paths are valid.
3. the relay host is reachable over SSH through the configured ansible inventory.
4. the selected tunnel is supported by the ansible runtime.
5. the tunnel resolves to an execution node.

Recommended default:

- `nftables_ansible`

Legacy compatibility:

- `iptables_ansible`

### Runtime behavior

When a forward is created, updated, resumed, paused, or deleted:

1. `PanelForwardRuntimeService` resolves the selected local ansible backend.
2. It validates the tunnel and loads the execution node.
3. It writes a pending runtime job (`backend='nftables_ansible'` by default, `backend='iptables_ansible'` for legacy hosts).
4. `PanelForwardRuntimeJobExecutor` polls pending jobs.
5. The executor runs `ansible-playbook`.
6. The playbook writes or removes relay firewall rules (`nftables` by default, `iptables` for legacy hosts).

### Verification

Check all three layers:

1. local executor host
  - confirm `ansible-playbook` exists
  - confirm inventory path exists
2. `v2board`
  - `SELECT * FROM v2_forward_runtime_job WHERE backend IN ('nftables_ansible','iptables_ansible') ORDER BY id DESC LIMIT 10;`
  - confirm job transitions `pending -> running -> success`
3. relay host
  - inspect `nft` ruleset for the default path, or `iptables-save` for the legacy path
  - confirm the expected rule exists or was removed

## Current Misleading Signals

Do not over-read the current UI indicators.

- `ForwardNode` online means only `host:port` TCP reachability
- it does not prove NodeX health
- it does not prove gost API health
- it does not prove SSH login works
- it does not prove runtime state already exists on the relay

## Evidence Chain For "Really Attached"

Do not call a relay "done" until you have evidence from all required layers.

### Control plane evidence

- `/admin/system`
  - confirm the selected runtime backend and NodeX mode values
- `/api/v2/admin/forward/runtime/status`
  - confirm the control plane is reachable
- `/api/v2/admin/forward/runtime/doctor`
  - confirm readiness, warnings, and the currently selected attachment model

### Job plane evidence

- `GET /api/v2/admin/forward/runtime/jobs`
- `SELECT * FROM v2_forward_runtime_job ORDER BY id DESC LIMIT 20;`

What you want to see:

- latest action for the target forward is `success`
- `gost` jobs show NodeX-side success
- local ansible jobs show the playbook succeeded and the relay host was reachable

### Relay plane evidence

- `gost`
  - `curl -u admin:<TOKEN> http://<HOST>:<API_PORT>/api/config/services`
  - `curl -u admin:<TOKEN> http://<HOST>:<API_PORT>/api/config/limiters`
- local ansible
  - `nft list ruleset` for `nftables_ansible`
  - `iptables-save` / `iptables -t nat -S` for `iptables_ansible`

### Network plane evidence

The final proof is not the UI. It is the forwarded port behavior.

- confirm the source target is reachable
- confirm the forwarded port connects
- confirm the forwarded port behaves the same way as the source target

That last step matters for non-HTTP services. In the verified 2026-04-07 example, the target accepted TCP connections but returned an empty reply to HTTP input, and both forwarded ports behaved the same way.

## Verified 2026-04-07 Example

The panel was used to create two forwards against the same target TCP service:

- target service: `155.117.224.30:11111`
- `143.20.204.14:11111` via local ansible
- `143.20.204.14:11112` via `gost`

The final pass condition was not only "job success". It was:

1. panel runtime status reachable
2. latest jobs marked `success`
3. relay-side firewall rule present for `11111`
4. relay-side gost service present for `11112`
5. all three TCP endpoints showed the same connect-and-empty-reply behavior

## Runtime Job Queries

Use these SQL queries as the fastest operator checks:

```sql
SELECT * FROM v2_forward_runtime_job
WHERE backend = 'gost'
ORDER BY id DESC
LIMIT 10;
```

```sql
SELECT * FROM v2_forward_runtime_job
WHERE backend IN ('nftables_ansible', 'iptables_ansible')
ORDER BY id DESC
LIMIT 10;
```

## Related Guides

- relay onboarding: [`forward-relay-onboarding.md`](forward-relay-onboarding.md)
- smoke tests: [`forward-tunnel-smoke-test.md`](forward-tunnel-smoke-test.md)
- NodeX boundary: [`nodex-internal-extension.md`](nodex-internal-extension.md)
