# anix.agent.v1 Control Stream

This document is the canonical protocol description for the SDK-owned
`anix.agent.v1` control stream.

Source of truth: this directory of `github.com/AnixOps/anix-control/sdk`
(`sdk/api/agent/v1`, Go package `agentv1pb`). The contract moved here from
`github.com/AnixOps/anix-agent/sdk`, which is frozen at v1.1.0. The move
changed only `option go_package`: the protobuf package, services, messages,
field numbers and types, and the registered file name
`api/grpc/agent/v1/agent.proto` are unchanged, so Agents built against the old
SDK keep working. Since then the contract only grows.
`agent_descriptor_test.go` proves the file descriptor is a superset of
v1.1.0's: every v1.1.0 message, field (name, number, label, type), enum value
and service method is still there unchanged, and only additions and
`go_package` differ. `internal/tests/protocompat` guards every later release
the same way.

`AgentControlService.ControlStream` is the v3 primary Agent-first bidirectional
gRPC control channel. Authentication reuses the configured node ID and node API
key as `x-node-id` and `x-api-key` metadata, or an agent client certificate.

`AgentEnrollment` (`agent_enrollment.proto`, a separate file of the same
package so `agent.proto` keeps its v1.1.0 descriptor) issues those
certificates. Its one URI SAN names the node:
`spiffe://anixops/<cluster>/agent/proxy-<id>` or
`spiffe://anixops/<cluster>/agent/forward-<id>`. `Enroll` takes the node
credential in metadata (`x-node-kind: forward` with a forward node's token)
or a one-time `anixagt_...` `enrollment_credential`; `Renew` and
`GetTrustBundle` need the current certificate. Certificates last 7 days and
are renewed after `renew_after_unix`, two thirds of the lifetime. With a
certificate the stream's node comes from it, every envelope's `node_id`
must name it, and `x-api-key` is not needed. `sdk/agentcontrol` has the
identity helpers and metadata keys.

The client declares capabilities in `Hello`, sends application heartbeats,
automatically reconnects with jittered exponential backoff, acknowledges
desired operations, and reports applying plus terminal `ObservedState`.
Operation execution is injected by the Agent runtime. When a stream drops before
terminal observation, Control replays the same operation ID and revision; the
Agent's completed-operation cache returns the terminal state without executing
a completed operation twice.

Each `OperationAck` carries the active `session_id` and operation `revision`,
and each `ObservedState` carries the active `session_id`. Control rejects a
message from a replaced session or with a revision that does not match its
retained desired operation. On reconnect, the Agent includes its latest
observed revision in `Hello`; Control reconciles its counter before replying
and sends retained operations as one serialized replay batch. The Agent checks
revision freshness both on receipt and immediately before execution, reporting
queued stale work as `SUPERSEDED` without invoking the runtime handler.

Plugin lifecycle operations use a strict JSON envelope inside
`DesiredOperation.payload_json`. Schema `anixops.operation/v1` carries
`operation_id`, `idempotency_key`, `session_id`, `revision`, `plugin_id`,
`target_version`, `config_hash`, and `config`. Duplicated IDs and revisions
must match the protobuf fields. The config hash is SHA-256 over the exact JSON
bytes, and configuration may contain Secret IDs or `*_ref`, never secret values.

An Agent advertises `plugin.inspect/configure/enable/disable/update/rollback/health`
only with explicit `PluginSupervisorEnabled`. Legacy configurations do not
advertise or execute plugin operations. Delivery is at least once, so handlers
must be idempotent. REST polling, v2board gRPC data services, and the legacy
WebSocket synchronization path remain compatibility/fallback transports during
the v3 migration.

## Data plane

The stream can also carry each node's configuration, users and reports, in
place of UniProxy, the v2board gRPC services and the WebSocket. These payloads
are additions to `anix.agent.v1`:

| Direction | Payload | Capability |
|---|---|---|
| Control → Agent | `ControlToAgent.config` (`ConfigSnapshot`) | `config.v1` |
| Agent → Control | `AgentToControl.config_status` (`ConfigStatus`) | `config.v1` |
| Control → Agent | `ControlToAgent.users` (`UserDelta` of `NodeUser`) | `users.v1` |
| Agent → Control | `AgentToControl.traffic` (`TrafficReport` of `UserTraffic` and `OnlineUser`) | `reports.v1` |
| Agent → Control | `AgentToControl.logs` (`LogBatch` of `LogEntry`) | `reports.v1` |
| Agent → Control | `AgentToControl.status` (`NodeStatus`) | `reports.v1` |
| Control → Agent | `ControlToAgent.report_ack` (`ReportAck`) | `reports.v1` |

`Hello` gains `config_revision` and `users_cursor`, and `HelloAck` gains
`server_capabilities`.

### Negotiation

- A capability written `config.v1` is the `Capability` with name `config`
  and version `v1`; `sdk/agentcontrol` names them (`CapabilityConfig`,
  `CapabilityUsers`, `CapabilityReports`).
- The Agent lists the ones it implements in `Hello.capabilities`. Control
  lists the ones it serves in `HelloAck.server_capabilities`.
- A capability is in use on a session only when both lists have it
  (`agentcontrol.Negotiated`). Each side sends a payload only when the other
  advertised its capability:
  - Control sends `ConfigSnapshot` only to an Agent whose `Hello` lists
    `config.v1`, and `UserDelta` only with `users.v1`. It reads
    `Hello.config_revision` and `Hello.users_cursor` only then.
  - The Agent sends `ConfigStatus` only with `config.v1`, and `TrafficReport`,
    `LogBatch` and `NodeStatus` only with `reports.v1`, in
    `HelloAck.server_capabilities`. Without them it keeps the legacy
    transports.
- Older Agents send none of these capabilities or `Hello` fields and skip
  `server_capabilities`, so nothing changes for them.
- Control serves `users.v1` and `reports.v1`, to proxy nodes (`v2_node`):
  `server_capabilities` lists `users.v1` for every proxy node (a forward node
  has no user list and is offered nothing), and `reports.v1` when the
  Agent's `Hello` lists it too (a forward node's stream is not offered it
  yet; its reports join with the forward plugins). `config.v1` is not served
  yet. An Agent that sends a payload the session did not negotiate
  (`config_status`; or `traffic`, `logs` or `status` without `reports.v1` in
  both lists) gets `InvalidArgument`, naming the capability it lacks, and
  the stream ends. A Control built before these payloads existed answers
  them the same way, as an unknown payload ("control message payload is
  required").
- `diag.v1` is reserved for node-side diagnostics (`diag.*` operations). It
  adds no payload; its rules come with those operations. Control records
  that the Agent advertised it on the session, so a diagnostic may run from
  the node's vantage once those operations exist.

### Delivery

- **Configuration** (`config.v1`). A `ConfigSnapshot` is the node's whole
  configuration at `config_revision`. `config_hash` is the lowercase hex
  SHA-256 of the exact `config_json` bytes, and `format` names their schema.
  - Control sends one after `HelloAck` when `Hello.config_revision` is
    older, and whenever the node's desired configuration changes.
  - The Agent answers each with `ConfigStatus`: `applied`, or an `error`
    (also for a hash mismatch or an unknown `format`). A failure becomes the
    node's runtime health.
  - A snapshot replaces what the Agent runs, so a lost or repeated snapshot
    is harmless.
- **Users** (`users.v1`). A `UserDelta` carries the changes after the
  Agent's cursor. The cursor is Control's subscriber change log position
  (`v4_kernel_subscriber_change`); the users are the ones the legacy pulls
  (UniProxy `user`, v2board `GetUsers`) give the node: the active
  subscribers of its plan group.
  - One delta may span several messages. The Agent applies them together
    when the one with `last_page` arrives, then stores `cursor` and sends it
    as `Hello.users_cursor` on its next connection.
  - **Full resync.** With `full`, the pages together are the node's whole
    user set and replace the Agent's. Control sends one, after `HelloAck`,
    when it cannot resume from `Hello.users_cursor`:
    - the cursor is 0 (the Agent has no set);
    - the cursor is ahead of the change log (it came from another log, such
      as a restored database);
    - the change log no longer holds every change after the cursor (rows
      are kept 7 days); also when that happens while the Agent is connected.

    The pages carry the users in id order, at most 500 per page, and each
    the same `cursor`: the log's position read before the listing, so a
    change made during it is sent again as a delta. The last page has
    `last_page`; an empty set is one empty page. An Agent that loses the
    stream before `last_page` has no new cursor and sends its old one again.
  - **Deltas.** Otherwise Control sends, after `HelloAck`, the changes after
    the cursor, and then a delta for each batch of changes as the log
    advances (it is read about once a second), at most 500 changes per
    delta, each with `last_page`. A delta carries each changed user's
    current state: in `upserts` when the node serves the user now, in
    `removed_user_ids` otherwise. A removal may name a user the Agent never
    had, which it ignores. The Agent applies deltas in order.
  - `speed_limit_mbps` and `device_limit` are 0 for no limit, as the legacy
    pulls send them. `extra_json` holds protocol-specific fields, such as
    the WireGuard peer fields when the node's protocol is WireGuard; it is
    empty otherwise. A `NodeUser` never carries the e-mail, the password
    hash or the subscription token.
- **Reports** (`reports.v1`).
  - `TrafficReport` carries per-user bytes and online IPs for one window.
    `LogBatch` carries runtime logs; `fields_json` holds no secrets.
  - Each has a `batch_id`, unique per node and kept when resent:
    `node:<kind>-<id>:<boot id>:<sequence>`, with `<kind>` `proxy` or
    `forward`; at most 128 bytes. Control records a batch once per node,
    in the transaction that applies it, and remembers it for 7 days, longer
    than any spool keeps a batch.
  - Control applies a report where the node's legacy report goes, so a byte
    counts once whichever path carried it: `users` through the transaction
    of the legacy traffic report (the traffic log, the node's counters, the
    server statistics and the subscriber counters, at the node's rate);
    `online` replaces the node's whole alive set, as the legacy online
    report does, so a report without `online` entries clears it; `entries`
    into the node log, as the legacy log batch; `NodeStatus` into the
    node's heartbeat, system and runtime-health columns, as the legacy
    status report and runtime-health report.
  - Control answers each `TrafficReport` and `LogBatch` with a `ReportAck`
    for its `batch_id`, with the `request_id` of the message it answers:
    - `applied: true` when this delivery recorded it;
    - `applied: false` and no `error` when a committed delivery had
      recorded it before; nothing is counted again;
    - `applied: false` and an `error` when Control refuses it for good: no
      `batch_id` or one over 128 bytes, a `user_id` of 0, bytes beyond the
      64-bit counter range, `fields_json` that is not JSON, or a node that
      no longer exists.

    In each case the Agent drops the batch from its spool. Control sends no
    `ReportAck` for a batch it cannot record for now (its database failed),
    and keeps the stream open; the Agent resends the batch later. A refused
    or unrecorded batch is not remembered, so a resend after the fault is
    applied.
  - A batch stays on the stream: the Agent never resends it over a legacy
    transport, which has no batch ids and could count it twice.
  - `NodeStatus` is the node's system and runtime health. Each replaces the
    previous one, and Control does not acknowledge it.
  - A report whose envelope `node_id` is not the stream's node ends the
    stream with `PermissionDenied`, as any other message does.

The checked-in Go files are generated, not handwritten. From the repository
root, run:

```bash
bash sdk/api/agent/gen.sh
```

The script maps the virtual path `api/grpc/agent/v1` onto this directory so the
registered file name stays `api/grpc/agent/v1/agent.proto`.

Verified generator versions: `libprotoc 29.2`, `protoc-gen-go v1.36.11`, and
`protoc-gen-go-grpc 1.6.1`.
