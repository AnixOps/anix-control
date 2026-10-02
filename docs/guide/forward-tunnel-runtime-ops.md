# Forward/Tunnel Runtime Operations

This guide documents the real runtime behavior behind the Flux-shaped `/admin/forward*` pages.

Do not treat the UI model as proof that a relay is already attached. Attachment is runtime-specific.

## Runtime Ownership

`anix-control` owns:

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

### `nftables_ansible` rule layout

The panel renders each forward's ruleset
(`internal/service/forward_nftables_plan.go`) and sends it to the playbooks as
the `nftables` extra var. `config/deploy/ansible/playbooks/files/v2b_forward_nft.sh`
then replaces the forward's rules in one `nft -f` transaction, so a failed
apply leaves the previous rules in place.

All forwards share one table, `inet v2b_forward`, with three base chains:

- `prerouting` (nat, priority dstnat): one rule per forward and protocol that
  matches the in port (plus `iifname` and the tunnel listen address when set)
  and jumps to `v2b_fwd_<id>_<proto>`.
- `postrouting` (nat, priority srcnat): `masquerade` for the forward's
  connections, IPv4 and IPv6.
- `forward` (filter, priority filter): jumps to the accounting chain
  `v2b_acct_<id>_<proto>` for every packet of the forward's connections.

Rules in the base chains are found by their comment
(`v2b-forward-<id>-<proto>-prerouting|forward|postrouting`). Example for
forward 12, TCP, `round` over two IPv4 targets and one IPv6 target
(`nft list table inet v2b_forward`):

```
table inet v2b_forward {
	counter fwd_12_tcp_up {
		packets 0 bytes 0
	}

	counter fwd_12_tcp_down {
		packets 0 bytes 0
	}

	chain prerouting {
		type nat hook prerouting priority dstnat; policy accept;
		tcp dport 8080 jump v2b_fwd_12_tcp comment "v2b-forward-12-tcp-prerouting"
	}

	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		ct status dnat meta l4proto tcp ct original proto-dst 8080 masquerade comment "v2b-forward-12-tcp-postrouting"
	}

	chain forward {
		type filter hook forward priority filter; policy accept;
		ct status dnat meta l4proto tcp ct original proto-dst 8080 jump v2b_acct_12_tcp comment "v2b-forward-12-tcp-forward"
	}

	chain v2b_fwd_12_tcp {
		meta nfproto ipv4 numgen inc mod 2 vmap { 0 : goto v2b_fwd_12_tcp_v4_0, 1 : goto v2b_fwd_12_tcp_v4_1 }
		meta nfproto ipv6 meta l4proto tcp dnat ip6 to [2001:db8::1]:443
	}

	chain v2b_fwd_12_tcp_v4_0 {
		meta l4proto tcp dnat ip to 10.0.0.1:80
	}

	chain v2b_fwd_12_tcp_v4_1 {
		meta l4proto tcp dnat ip to 10.0.0.2:80
	}

	chain v2b_acct_12_tcp {
		ct direction original counter name "fwd_12_tcp_up"
		ct direction reply counter name "fwd_12_tcp_down"
	}
}
```

Requirements: Linux 5.2 or later and nft 0.9.1 or later on the relay (NAT
chains in an `inet` table). If the host firewall drops forwarded packets
(for example an iptables `FORWARD` policy of `DROP`, as Docker sets), allow
the forwarded traffic there; an accept in this table does not override a drop
in another table.

#### Traffic accounting

- `fwd_<id>_<proto>_up` counts the conntrack original direction (client to
  target) and is reported as upload; `fwd_<id>_<proto>_down` counts the reply
  direction (target to client) and is reported as download. This is the same
  meaning as the gost path's `u`/`d`, and both go through the same
  `traffic_ratio` and one-way/two-way (`flow`) conversion, so
  `out_flow` += upload and `in_flow` += download as for gost.
- The counters are table objects, not rule counters: re-applying a forward
  flushes and rebuilds its chains but keeps the counters. Pause keeps them;
  delete removes them. After a relay reboot the counters start again from
  zero; the traffic cursor treats a lower total as a reset and counts the new
  total.
- The counters count whole IP packets (headers included), so totals are a
  few percent higher than gost's payload byte counts for the same traffic.
- `ForwardAnsibleStatsWorker` runs `forward_collect_stats_nftables.yml` every
  60 seconds and records the totals under the cursor key
  `nftables_ansible:ct`.

#### IPv6

- Targets can be `host:port` or `[v6]:port`. Host names are treated as IPv4
  targets (nft resolves them when it loads the rules).
- An IPv4 client can only be forwarded to an IPv4 target and an IPv6 client
  to an IPv6 target (nft cannot translate between families). A target list
  with both families is handled per family: IPv4 clients use the IPv4 targets
  and IPv6 clients the IPv6 targets, each with the forward's strategy.
- The tunnel listen address selects the ingress family: empty, `::` or `[::]`
  accepts both; `0.0.0.0` accepts IPv4 only; a concrete address matches only
  that address. When no target matches an accepted family, the apply fails
  with a clear error and the forward is marked as an error.
- `net.ipv6.conf.all.forwarding=1` is set only when the forward has an IPv6
  target. Turning it on makes the kernel ignore router advertisements on
  interfaces with `accept_ra=1`; on a relay that gets its IPv6 default route
  by SLAAC, set `accept_ra=2` on that interface first.

#### Known limits of the nftables path

- `round` and `rand` balance new connections over the targets of a family
  (`numgen inc` / `numgen random`).
- `fifo` (主备) and `hash` use only the first target of each family. There is
  no health check or failover, and no source-hash balancing.
- Speed limits (`speedId` on a user tunnel) are not enforced on this path.
  The limiter is still sent in the payload, but no rule uses it; only the
  gost path applies speed limits.
- Rules do not survive a relay reboot; re-apply the forwards (pause and
  resume, or save them again) after a reboot.

#### Migration from the `ip v2b_forward` table

Before v4.1.0-rc.5 the playbooks used an IPv4-only `ip v2b_forward` table.
The first apply of a forward after the upgrade removes that forward's rules
from the old table in the same transaction that adds the new ones, so a
connection is never translated twice; the old table is deleted together with
the last forward that used it. Pause and delete clean both tables. Until a
forward is re-applied, the stats worker keeps reading its old nat-chain
counter (upload only, as before).

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
