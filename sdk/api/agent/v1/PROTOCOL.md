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
| Agent → Control | `AgentToControl.package_report` (`PackageReport`) | `package-reports.v1` |

`Hello` gains `config_revision` and `users_cursor`, and `HelloAck` gains
`server_capabilities`.

### Negotiation

- A capability written `config.v1` is the `Capability` with name `config`
  and version `v1`; `sdk/agentcontrol` names them (`CapabilityConfig`,
  `CapabilityUsers`, `CapabilityReports`, `CapabilityPackageReports`).
- The Agent lists the ones it implements in `Hello.capabilities`. Control
  lists the ones it serves in `HelloAck.server_capabilities`.
- A capability is in use on a session only when both lists have it
  (`agentcontrol.Negotiated`). Each side sends a payload only when the other
  advertised its capability:
  - Control sends `ConfigSnapshot` only to an Agent whose `Hello` lists
    `config.v1`, and `UserDelta` only with `users.v1`. It reads
    `Hello.config_revision` and `Hello.users_cursor` only then.
  - The Agent sends `ConfigStatus` only with `config.v1`, and `TrafficReport`,
    `LogBatch` and `NodeStatus` only with `reports.v1`, and `PackageReport`
    only with `package-reports.v1`, in `HelloAck.server_capabilities`.
    Without them it keeps the legacy transports.
- Older Agents send none of these capabilities or `Hello` fields and skip
  `server_capabilities`, so nothing changes for them.
- Control serves `config.v1`, `users.v1` and `reports.v1`.
  `server_capabilities` lists `config.v1` when the Agent's `Hello` lists it,
  for proxy and forward nodes; `users.v1` for every proxy node (`v2_node`; a
  forward node has no user list and is not offered it); and `reports.v1`
  when the Agent's `Hello` lists it too, for proxy nodes (a forward node's
  stream is not offered it yet; its reports join with the forward plugins).
  It serves `package-reports.v1` when the Agent's `Hello` lists it, for
  proxy nodes; forward nodes are not offered it yet.
  An Agent that sends a payload the session did not negotiate
  (`config_status` without `config.v1`, or `traffic`, `logs` or `status`
  without `reports.v1`, `package_report` without `package-reports.v1`, in
  both lists) gets `InvalidArgument`, naming the
  capability it lacks, and the stream ends. A Control built before these payloads existed answers
  them the same way, as an unknown payload ("control message payload is
  required").
- `diag.v1` is reserved for node-side diagnostics (`diag.*` operations). It
  adds no payload; its rules come with those operations. Control records
  that the Agent advertised it on the session, so a diagnostic may run from
  the node's vantage once those operations exist.

### Delivery

- **Configuration** (`config.v1`). A `ConfigSnapshot` is the node's whole
  configuration at `config_revision`: Control's desired configuration of the
  node, as stored. `config_hash` is the lowercase hex SHA-256 of the exact
  `config_json` bytes, and `format` names their schema
  (`anixops.nodeconfig/v1`). The revision grows by one each time the hash
  changes, and only then.
  - **What it carries.** The document of a proxy node holds its node row,
    `raw_config`, its enabled `protocols` (each `config` is what the v2board
    `GetConfig` of that type renders) and `legacy_pull`: `default`, the
    UniProxy configuration answer for no `node_type`, and `types`, the
    answer for each node type the node serves, by normalized type. A pull
    that UniProxy would refuse is left out. An Agent that applied
    `legacy_pull` runs what the legacy pull would have given it. A forward
    node's document holds its node row, `legacy_rules` (the rules
    `GET /api/v2/forward/agent/rules` serves, with the node's role) and
    `tunnels`. A snapshot carries no secret the legacy pulls do not send
    the node, and never the node's API key, secret or token.
  - **When Control sends one.**
    - After `HelloAck`, when `Hello.config_revision` is not the desired
      revision: 0 (none), older, or newer (a revision from another
      database). The desired configuration is rebuilt from the node's rows
      first. Same revision: nothing is sent.
    - When an administrator's node sync (`node.sync`, the legacy sync
      route) stores a changed configuration, is forced, or finds that the
      Agent has not applied the stored one. Such an Agent is not sent
      `node.reload`; an Agent without `config.v1` still is.
    - When a rebuild from the node's rows, about once a minute per session,
      moves the revision.

    A session is never sent a revision older than one it was already sent.
    A snapshot of the revision the Agent has may be sent again (a forced
    sync).
  - **The answer.** The Agent answers each snapshot with `ConfigStatus` of
    its `config_revision` and `config_hash`: `applied`, or `applied: false`
    with an `error` (also for a hash mismatch or an unknown `format`).
    `config_revision` is required; a status without one, or without the
    payload, ends the stream with `InvalidArgument`. Control records the
    last status per node and judges it against the desired configuration:
    `applied` or `failed` when it names the desired revision and hash,
    `stale` for an older revision, `mismatch` otherwise. Only an applied
    status naming the desired revision and hash becomes the node's applied
    revision.
  - **Node syncs.** A `node.sync` that pushed a snapshot ends on the
    Agent's `ConfigStatus` for that revision and hash, from any session of
    the node: applied ends it `SUCCEEDED`; not applied ends it `FAILED` with
    the Agent's `error`. A stale or mismatched status does not end it. A
    status that Control verified as applied at a newer revision ends it
    too: the node runs a newer desired configuration. An Agent that
    reconnects is sent the snapshot again by the `Hello` rule, so it can
    answer on the new session.
  - The Agent sends the revision it applied as `Hello.config_revision` on
    its next connection.
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
- **Package reports** (`package-reports.v1`). A `PackageReport` is the latest
  observation of one `kind` that an Agent plugin package makes on the node,
  such as the systemd services table of `machine-telemetry`
  (`docs/architecture/package-reports.md`).
  - **Fields.** `plugin_id` is the reporting package. `kind` names the
    payload schema: lowercase dot-separated words, at most 64 bytes
    (`systemd.services`); a new schema version is a new kind. `version` is
    the release of `plugin_id` the Agent runs. `payload_json` is a JSON
    object in the kind's schema, at most 256 KiB
    (`agentcontrol.MaxPackageReportPayloadBytes`). `observed_at_unix_ms` is
    when the plugin took the observation; 0 means when Control received it.
  - **Latest value wins.** Control keeps one report per node, `plugin_id`
    and `kind`: a report replaces the stored one unless that one was
    observed later, and no history is kept. Control does not acknowledge a
    `PackageReport`; the Agent neither spools nor resends it, and sends the
    next observation instead.
  - **Authorization.** Control accepts a report only when the node has an
    enabled assignment of `plugin_id` at `version`, and that release is a
    signed official (AnixOps) Agent release, still verifying against the
    trust root, whose manifest declares the capability the kind requires.
    Control knows each kind it accepts; a package cannot add one.

    | Kind | Capability | Schema |
    |---|---|---|
    | `systemd.services` | `telemetry.systemd.read` | `sdk/telemetry/systemdreport` |
  - **Sanitizing.** Control stores the payload only as the kind's sanitizer
    re-encodes it: unknown fields are dropped, and a payload the sanitizer
    refuses is refused. For `systemd.services` that is a payload with a
    `Description` or `ExecStart` field, more than 512 units, a unit name
    over 256 bytes, or a malformed field.
  - **Refusal.** A report that is malformed, oversize, observed more than a
    minute in the future, of an unknown kind, not authorized or refused by
    its sanitizer is dropped, logged and counted
    (`anixops_agent_package_reports_refused_total{reason}`); so is one
    Control cannot record now (`anixops_agent_package_reports_total{result="unrecorded"}`).
    The stream stays open.
  - Packages read the reports of their own `plugin_id` through the kernel
    API view `kapi_package_report_v1`.

The checked-in Go files are generated, not handwritten. From the repository
root, run:

```bash
bash sdk/api/agent/gen.sh
```

The script maps the virtual path `api/grpc/agent/v1` onto this directory so the
registered file name stays `api/grpc/agent/v1/agent.proto`.

Verified generator versions: `libprotoc 29.2`, `protoc-gen-go v1.36.11`, and
`protoc-gen-go-grpc 1.6.1`.
