# Forwarding Design

## Purpose

The forwarding module is a small, auditable control plane for TCP and UDP
forwarding. The panel stores configuration, ownership, quota state, runtime job
history, and operational diagnostics. The relay or agent side performs the
actual network changes.

This design intentionally separates three concerns:

- Panel state: database rows, permissions, quota, audit, and UI.
- Runtime orchestration: NodeX, local Ansible, or clean agent job execution.
- Relay state: gost services, nftables/iptables rules, or agent-applied rules.

Saving a node or forward in the panel is not proof that the relay has been
attached. Runtime attachment requires control-plane, job-plane, relay-plane, and
network-plane evidence.

## Current Components

Core files:

- `internal/model/forward.go`: legacy relay/exit node, rule, log, and hourly stat models.
- `internal/model/forward_panel.go`: Flux-shaped tunnel, user-tunnel grant,
  forward, and port-binding models.
- `internal/model/forward_runtime.go`: runtime job and traffic cursor models.
- `internal/model/forward_clean_agent.go`: clean agent registry.
- `internal/handler/forward.go`: admin node/rule/stat endpoints.
- `internal/handler/forward_panel.go`: Flux-shaped forward and tunnel endpoints.
- `internal/handler/forward_flow.go`: flow and traffic ingestion endpoints.
- `internal/handler/forward_clean_agent.go`: clean agent registration, heartbeat,
  report, and admin token management.
- `internal/service/forward_panel_service.go`: forward, tunnel, user-tunnel,
  quota, diagnosis, and runtime coordination.
- `internal/service/forward_panel_flow.go`: delta and snapshot traffic writes.
- `internal/service/forward_runtime_executors.go`: backend selection semantics.

Related operator guides:

- `docs/guide/flux-forward-contract.md`
- `docs/guide/forward-tunnel-runtime-ops.md`
- `docs/guide/forward-relay-onboarding.md`
- `docs/guide/forward-tunnel-smoke-test.md`
- `docs/forward-clean-room/spec.md`

## Data Model

`ForwardNode` stores relay or execution-node metadata:

- `type`: `relay` or `exit`.
- `host`, `port`: coarse TCP reachability target.
- `api_port`, `api_token`: relay API details for gost-style control.
- `metrics_port`: optional metrics endpoint.
- status and counters: coarse health and aggregate traffic fields.

`ForwardTunnel` stores ingress and optional egress topology for panel forwards:

- `inNodeId`, `outNodeId`
- ingress and egress IP fields
- allowed port range
- protocol, listen addresses, traffic ratio, and status

`ForwardUserTunnel` grants a user access to a tunnel:

- user and tunnel IDs
- traffic quota and used flow
- max forward count
- monthly reset day and expiry time
- optional speed-limit reference

`Forward` stores the user-visible forwarding rule:

- owner, tunnel, input port, remote target, strategy, order, and status
- latest runtime backend/status/message/sync time
- accumulated input/output traffic

`ForwardPortBinding` is the database-level exact socket guard:

- node ID
- transport
- listen address
- input port

The service layer still owns wildcard listen-address overlap checks because a
database unique key cannot express all `0.0.0.0` and concrete-address conflicts.

`ForwardRuntimeJob` records side effects that must happen outside the database:

- backend: `gost`, `nftables_ansible`, `iptables_ansible`, or `clean_agent`
- action: `create`, `update`, `delete`, `pause`, `resume`, or `sync`
- resource references and JSON payload/result/error
- lifecycle timestamps and status

`ForwardTrafficCursor` lets snapshot-based collectors convert cumulative
runtime counters into safe deltas.

## Runtime Backends

### Gost Through NodeX

Backend name: `gost`.

Execution chain:

1. Panel writes or updates `v2_forward` state.
2. Panel writes a `v2_forward_runtime_job` row.
3. Panel calls the NodeX control plane.
4. NodeX calls the relay gost API.
5. Relay gost applies services and limiters.

NodeX mode uses ingress-node semantics. The selected tunnel must resolve to an
ingress `ForwardNode` that has valid gost API details.

### Local Ansible

Backend names:

- `nftables_ansible`: recommended default.
- `iptables_ansible`: legacy compatibility.

Execution chain:

1. Panel writes or updates `v2_forward` state.
2. Panel queues a pending runtime job.
3. Local job executor runs `ansible-playbook`.
4. Ansible connects to the relay host using configured inventory.
5. Relay host receives nftables or iptables rules.

`ForwardNode` does not store SSH credentials. SSH users, passwords, keys, and
sudo behavior live in Ansible inventory and config files.

### Clean Agent

Backend name: `clean_agent`.

Execution chain:

1. Admin creates an agent token.
2. Agent registers and heartbeats against `/api/v2/forward-agent/*`.
3. Panel queues clean-agent runtime jobs.
4. Agent heartbeat claims pending jobs for its node.
5. Agent reports success, failure, and optional traffic deltas.

Clean agent mode uses execution-node semantics and is documented separately in
`docs/forward-clean-room/spec.md`.

## Ownership And Permissions

User-facing Flux-shaped endpoints use JWT authentication and only operate on the
current user's own forwards. Admin mirrors are under `/api/v2/admin/*` and use
admin authentication.

Admin-only surfaces include:

- forward nodes
- legacy forward rules
- tunnel creation and mutation
- user-tunnel grant assignment
- runtime job/status/doctor endpoints
- clean-agent token management
- internal traffic ingestion and AppToken-protected APIs

The service layer must preserve ownership checks even when an endpoint exists in
both user and admin route groups.

## Traffic And Quota

The forwarding subsystem currently accepts traffic through three paths:

- Flux-style flow upload: `/flow/upload`.
- Internal delta report: `/api/v2/internal/forward/traffic/report`.
- Internal snapshot report: `/api/v2/internal/forward/traffic/snapshot`.

Delta and snapshot writes update:

- `v2_forward.in_flow` and `out_flow`
- user-tunnel usage
- user usage where applicable
- quota-triggered pause behavior for exhausted users or tunnel grants

Snapshot ingestion uses `ForwardTrafficCursor` so a collector can send cumulative
runtime totals without double-counting. Counter resets become new positive
baselines.

## State Transitions

Forward status values:

- `0`: paused
- `1`: active
- `-1`: error

Runtime job status values:

- `0`: pending
- `1`: running
- `2`: success
- `3`: failed

Runtime updates must be idempotent. Repeated pause or resume requests should not
enqueue duplicate work while a matching pending or running job already exists.

## Current Guarantees

The current implementation has these useful guards:

- exact port binding uniqueness through `ForwardPortBinding`
- service-level wildcard listen-address conflict detection
- runtime job rows for side-effect visibility
- traffic snapshot cursors for cumulative counters
- context-aware forward node health checks
- restricted local runtime command execution to reviewed `ansible-playbook`
  commands
- clean-agent token generation with entropy error handling
- Flux-shaped response envelope for the compatible panel endpoints

## Known Gaps

These are not considered complete yet:

- Full runtime-side speed-limit propagation is still incomplete.
- Flux user state and all `FlowController` side effects are not fully cloned.
- Legacy `ForwardRule` runtime sync still tolerates runtime failures after DB
  persistence to preserve existing behavior.
- `ForwardNode` online state is only `host:port` TCP reachability, not runtime
  attachment.
- Some admin node/rule endpoints still use the older `{data}` or `{error}`
  response shape rather than the Flux envelope.
- Administrator operation audit coverage is not complete for all forwarding
  mutations.
- Race and benchmark coverage exists for selected paths but is not yet a full
  forwarding-module gate.

## Verification Standard

A forwarding change should be treated as done only when evidence exists at the
right scope:

- Service tests for model validation, quota, idempotency, and runtime job writes.
- Handler tests for authentication, ownership, request validation, and response
  shape.
- Race tests for background workers or shared counters.
- Smoke checks for each runtime path:
  - NodeX/gost status, runtime job success, relay gost service, network behavior.
  - local Ansible job success, relay nftables/iptables rule, network behavior.
  - clean agent heartbeat/report, job success, network behavior.
- CI gates for `go test`, `go vet`, `golangci-lint`, and `gosec`.
