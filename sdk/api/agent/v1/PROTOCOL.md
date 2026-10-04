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
identity helpers and metadata keys. Every refusal names its reason in the
`x-anix-error-code` trailer ("Error codes" below). `IssueLinkCertificate`
and `GetLinkTrustBundle` serve a node that negotiated `forward.v1` its
forward link certificate, from a separate CA ("Forward link certificates"
below).

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
| Control → Agent | `ControlToAgent.config` in format `anixops.nodeconfig/v2` (`NodeForwardState`) | `forward.v1` with `config.v1` |
| Agent → Control | `AgentToControl.package_report` of kind `forward.report` (`NodeForwardReport`) | `forward.v1` with `package-reports.v1` |
| Agent → Control | `AgentToControl.maintenance_events` (`MaintenanceEvents`) | `maintenance.v1` |
| Control → Agent | `ControlToAgent.maintenance_ack` (`MaintenanceAck` of `MaintenanceEventResult`) | `maintenance.v1` |
| Control → Agent | `ControlToAgent.alive_list` (`AliveList` of `UserAlive`) | `alive.v1` |
| Agent → Control | `AgentArtifacts.GetPluginManifest`, `DownloadPluginArtifact` (`artifacts.proto`; a service, not a stream payload) | `artifacts.v1` |

`Hello` gains `config_revision` and `users_cursor`, and `HelloAck` gains
`server_capabilities`.

An Agent that negotiates `config.v1`, `users.v1`, `reports.v1`, `alive.v1`
and, when it runs the plugin supervisor, `maintenance.v1` and
`artifacts.v1` needs no legacy path and no node API key: under
`agent_control.mtls: required`, which refuses the legacy paths, each has a
stream equivalent.

| Legacy path | Stream equivalent |
|---|---|
| `POST /api/v2/node/register` | `AgentEnrollment.Enroll` with a one-time `anixagt_` credential |
| `POST /api/v2/node/heartbeat` (`cpu_usage`, `memory_usage`, `disk_usage`, `uptime`) | `NodeStatus` (`reports.v1`); liveness also from `Heartbeat` |
| `POST /api/v2/node/heartbeat` (`online_users`, `upload`, `download`) | `TrafficReport` (`reports.v1`): `online`, and the per-user bytes, which Control adds to the node's counters |
| `POST /api/v2/node/runtime-health` (`healthy`, `error`) | `NodeStatus.runtime_healthy` and `runtime_error` (`reports.v1`) |
| `GET /api/v2/agent/ws`, `/api/v2/node/ws`: `heartbeat`, `pong` | `Heartbeat` |
| the WebSocket's `maintenance_events` and `maintenance_ack` | `MaintenanceEvents` and `MaintenanceAck` (`maintenance.v1`) |
| the WebSocket's `task.assign` (and `GET /api/v2/agent/tasks`, `POST /api/v2/agent/result`) | the `agent.diagnostic` operation, for an Agent that lists that capability: `DesiredOperation`, `OperationAck`, `ObservedState` |
| UniProxy `config`, the WebSocket's `config_update` | `ConfigSnapshot` (`config.v1`) |
| UniProxy `user`, the WebSocket's `user_update` and `user_ban` | `UserDelta` (`users.v1`) |
| UniProxy `push` and `alive`, `NodeLogService.ReportLogs` | `TrafficReport` and `LogBatch` (`reports.v1`) |
| UniProxy `alivelist` | `AliveList` (`alive.v1`) |
| `GET /api/v3/agent/plugin-releases/{plugin_id}/{version}/manifest` and `/artifact` (`X-API-Key`) | `AgentArtifacts.GetPluginManifest` and `DownloadPluginArtifact` by the client certificate (`artifacts.v1`) |
| `POST /admin/agent/tasks`, `/admin/agent/execute` (administrator routes that sent `task.assign` on the WebSocket) | the same routes send the `agent.diagnostic` operation on the stream when the node has no WebSocket and its Agent lists that capability |

### Negotiation

- A capability written `config.v1` is the `Capability` with name `config`
  and version `v1`; `sdk/agentcontrol` names them (`CapabilityConfig`,
  `CapabilityUsers`, `CapabilityReports`, `CapabilityPackageReports`,
  `CapabilityForward`, `CapabilityMaintenance`, `CapabilityAlive`,
  `CapabilityArtifacts`; `DataPlaneCapabilities` lists them).
- The Agent lists the ones it implements in `Hello.capabilities`. Control
  lists the ones it serves in `HelloAck.server_capabilities`.
- A capability is in use on a session only when both lists have it
  (`agentcontrol.Negotiated`). Each side sends a payload only when the other
  advertised its capability:
  - Control sends `ConfigSnapshot` only to an Agent whose `Hello` lists
    `config.v1`, and `UserDelta` only with `users.v1`. It reads
    `Hello.config_revision` and `Hello.users_cursor` only then.
  - The Agent sends `ConfigStatus` only with `config.v1`, and `TrafficReport`,
    `LogBatch` and `NodeStatus` only with `reports.v1`, `PackageReport`
    only with `package-reports.v1`, and `MaintenanceEvents` only with
    `maintenance.v1`, in `HelloAck.server_capabilities`. Control sends
    `AliveList` only with `alive.v1`. The Agent calls `AgentArtifacts`
    only with `artifacts.v1`.
    Without them it keeps the legacy transports.
- Older Agents send none of these capabilities or `Hello` fields and skip
  `server_capabilities`, so nothing changes for them.
- **The offer rule.** Control lists a capability in `server_capabilities`
  only when the Agent's `Hello` lists it at `v1` **and** Control serves it
  to the stream's node: the list is the intersection, so it is the
  session's negotiated set. What Control serves, by node kind:
  - `config.v1`: proxy and forward nodes;
  - `users.v1`: proxy nodes (`v2_node`; a forward node has no user list);
  - `reports.v1`: proxy nodes (a forward node's stream is not offered it
    yet; its reports join with the forward plugins);
  - `package-reports.v1`: proxy nodes, and forward nodes served `forward.v1`
    (their other package reports are refused: they have no plugin
    assignments);
  - `forward.v1` (see "Forwarding"): proxy and forward nodes, when the
    `Hello` lists it with a valid `node_capabilities` attribute and Control
    serves the session `config.v1`;
  - `maintenance.v1` (see "Maintenance events"): proxy nodes, whose node log
    stores the events;
  - `alive.v1` (see "Alive list"): proxy nodes, whose users have device
    limits;
  - `artifacts.v1` (see "Plugin artifacts"): proxy nodes, on a session
    authenticated by client certificate (`AgentArtifacts` reads no node
    API key, so a session on the API key is not offered it).

  `reports.v1` carries one attribute: an Agent that lists it with
  `transient_ack: "v1"` asks for transient report acknowledgements ("Error
  codes"); Control's `reports.v1` echoes the attribute when it sends them.

  Before this rule Control 4.1 listed `users.v1` for every proxy node
  whatever the Agent listed. Nothing changes for any Agent: a capability
  was in use only when both lists had it, so an Agent that did not list
  `users.v1` never used it. An Agent built before advertisement lists no
  data-plane capability, is offered none and ignores the empty list; it
  keeps its legacy transports. An Agent reads `server_capabilities` as the
  negotiated set and must not assume a capability it did not list.
- An Agent that sends a payload the session did not negotiate
  (`config_status` without `config.v1`, or `traffic`, `logs` or `status`
  without `reports.v1`, `package_report` without `package-reports.v1`,
  `maintenance_events` without `maintenance.v1`, in
  both lists) gets `InvalidArgument`, naming the
  capability it lacks, with the code `agent_capability_not_negotiated` in
  the trailer, and the stream ends. A Control built before these payloads existed answers
  them the same way, as an unknown payload ("control message payload is
  required").
- `diag.v1` means the Agent runs node-side diagnostics: the forward checks
  of the `agent.diagnostic` operation (see "Diagnostic operation"). It adds
  no payload. Control records it on the session and sends the forward
  checks only to a session that lists both `agent.diagnostic` and
  `diag.v1`. No separate `diag.*` operation kinds exist.

### Delivery

- **Configuration** (`config.v1`). A `ConfigSnapshot` is the node's whole
  configuration at `config_revision`: Control's desired configuration of the
  node, as stored. `config_hash` is the lowercase hex SHA-256 of the exact
  `config_json` bytes, and `format` names their schema
  (`anixops.nodeconfig/v1`, or `anixops.nodeconfig/v2` for a node whose
  Agent negotiated `forward.v1`, see "Forwarding"). The revision grows by one each time the hash
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

    In each case the Agent drops the batch from its spool. Each refusal
    names its `error_code` ("Codes in acknowledgements"). Control sends no
    `ReportAck` for a batch it cannot record for now (its database failed),
    and keeps the stream open; the Agent resends the batch later. An Agent
    that negotiated `transient_ack` gets `report_unavailable` with a retry
    hint instead. A refused
    or unrecorded batch is not remembered, so a resend after the fault is
    applied.
  - A batch stays on the stream: the Agent never resends it over a legacy
    transport, which has no batch ids and could count it twice.
  - `NodeStatus` is the node's system and runtime health. Each replaces the
    previous one, and Control does not acknowledge it. It replaces both
    `POST /api/v2/node/heartbeat` and `POST /api/v2/node/runtime-health`,
    with their side effects together: the CPU, memory and disk usage and
    the uptime, `last_check_at` and the node's status (online, never
    re-enabling a disabled node), `runtime_healthy`, `runtime_error` (cut
    to 4096 bytes) and `runtime_checked_at`, and a sighting of the
    session's transport in the transport inventory (`mtls-stream` or
    `apikey-stream`, at most one write a minute). Unlike the legacy
    runtime-health report, a `NodeStatus` always carries both: when the
    runtime health changes between two system samples, the Agent sends a
    `NodeStatus` with its current system usage too (zeros would be
    recorded). `online_users` comes from `TrafficReport.online`, and the
    heartbeat's `upload` and `download` from the per-user bytes of
    `TrafficReport`.
  - A report whose envelope `node_id` is not the stream's node ends the
    stream with `PermissionDenied`, as any other message does.
- **Forwarding** (`forward.v1`; `docs/architecture/forward-sdk.md` section
  8). The node forwards for Control's routes. `sdk/forward/wire` encodes
  and checks every rule below; the Agent and Control both use it.
  - **Hello.** The Agent lists `forward.v1` with the attribute
    `node_capabilities`: its `anixops.forward.v1.NodeCapabilities` (the
    drivers' `EngineCapabilities`, kernel, cgroup, IPv6, Agent version) as
    protojson with the proto field names, at most 16 KiB, at most 8
    engines, each a known engine once, enums the contract defines, texts
    within 1 KiB; `node_ref`, when set, names the stream's node. An Agent
    that lists `forward.v1` lists `config.v1` and `package-reports.v1` too.
    A missing or malformed attribute is not an error: Control leaves
    `forward.v1` out of `server_capabilities` and the stream goes on.
  - **Inventory.** On each `Hello` Control records the node's capabilities
    and whether the session negotiated `forward.v1`; a `Hello` without it
    clears that flag. A node whose last `Hello` negotiated it is in
    Control's forwarding inventory, and a change of its capabilities
    replans every route before the `Hello`'s snapshot is built.
  - **Desired state.** While the flag is set, the node's configuration
    (every snapshot, whatever triggered it) has the format
    `anixops.nodeconfig/v2`: the `anixops.nodeconfig/v1` document plus the
    member `forward`, the node's `NodeForwardState` as protojson with the
    proto field names (`generation` and other 64-bit numbers as strings).
    `generation` 0 means Control has no state for the node yet (its first
    plan was refused): the Agent keeps what it runs. Otherwise the Agent
    applies the state only when `generation` is newer than the one it
    runs, compares `state_hash` (never contents) and persists the applied
    state. Every plan that moves the node's generation pushes a new
    snapshot at once; `config_revision` grows with it. `ConfigStatus`
    answers the snapshot as for v1 (`applied` false only when the document
    as a whole cannot be applied); hop errors go in the report. An Agent
    without `forward.v1` keeps v1 and never receives forwarding. A v1
    snapshot carries no forwarding and leaves the forwarding state the
    Agent applied as it is; only a v2 state without the hops (an empty
    state removes everything the drivers own) takes them away.
  - **Reports.** The Agent sends its `anixops.forward.v1.NodeForwardReport`
    as a `PackageReport` with `plugin_id` `forward`, `kind`
    `forward.report`, `version` `v1` and `payload_json` its protojson, every
    60 s and after an apply or a health change (at most one every 10 s).
    Control accepts it only on a session that negotiated `forward.v1`, not
    from a plugin release, and refuses (drops, logs and counts; the stream
    stays open) one sent without it (`unnegotiated`), another `version`
    (`invalid`), an oversize or future one, and one whose payload fails the
    checks (`bad_payload`): `node_ref` must be the stream's node; a
    `Counters.node_ref` empty or the same; route ids of 1 to 64 ASCII
    letters and digits; hop indexes below 8; `counter_epoch` 1 to 128
    bytes, each (route, hop, epoch) once; at most 16384 counters, health
    entries and errors; counters within the signed 64-bit range. The
    256 KiB payload cap bounds a report to roughly 800 hops of counters.
  - **Counters.** Cumulative within a `counter_epoch`, which ends only when
    the hop's counter objects are re-created (the driver's rule). Control
    keeps, per route, hop, node and epoch, the largest values reported and
    adds their growth to its traffic ledger; a new epoch counts in full, a
    decrease counts nothing, and a report observed before the stored one
    is dropped whole. A lost or repeated report therefore loses or doubles
    nothing.
  - **Generation recovery.** The report's `generation` and `state_hash`
    are those of the state the Agent holds (the last one it accepted, even
    when hops failed). After Control's database is reset or restored, the
    generations Control stamps can be lower than that, and the Agent
    ignores them. So a report is *ahead* of the node's stored state when
    its `generation` is higher, or equal with another non-empty
    `state_hash`; Control then moves the node's generation to the reported
    one when the `state_hash` is the same, else to the reported one plus
    one, keeping the node's hops, and pushes a new snapshot at once. Every
    later plan stamps above it. The operator command
    `anix-control forward reset-node <node_ref>` forces a generation above
    both. Agents need no change: they only have to keep reporting the
    generation and `state_hash` they hold, and keep ignoring older
    generations. Until the first report after a reset (at most 60 s) the
    Agent ignores the lower generation and keeps running what it runs.
  - **Heartbeat.** `HelloAck.heartbeat_interval_seconds` is 60 for a
    forward node's session that negotiated `forward.v1` (20 otherwise). A
    certificate revoked while the stream is open ends it at the next
    heartbeat, so within a minute on such a session.
  - **Link certificates.** Encrypted links between nodes use the node's
    forward link certificate, never the Agent certificate; see "Forward
    link certificates".
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

    The kind `forward.report` of `plugin_id` `forward` is not authorized by
    a release: it needs `forward.v1` on the session (see "Forwarding").
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
- **Maintenance events** (`maintenance.v1`). The Agent keeps the plugin
  supervisor's health incidents and recoveries in a durable outbox until
  Control has stored them. Before `maintenance.v1` it sent them only on the
  legacy agent WebSocket (`maintenance_events`, answered by
  `maintenance_ack`), which `agent_control.mtls: required` refuses; a gRPC
  Agent opened a WebSocket for them alone.
  - **The batch.** `MaintenanceEvents.version` is the event schema,
    `anixops.maintenance/v1` (`agentcontrol.MaintenanceSchemaV1`), and
    `events_json` holds the events oldest first, each one JSON object of
    that schema, the same object the WebSocket batch carries: at most 16 KiB
    per event, 50 events and 256 KiB per batch
    (`agentcontrol.MaxMaintenance*`). Control checks each event as the
    Agent's outbox does before it queues one
    (`agentcontrol.ParseMaintenanceEvent`: `schema_version` 1, identities,
    error code, times, enumerations, a failure or a recovery that is
    consistent) and that its `node_id` is the stream's node. Fields the
    schema does not know are dropped.
  - **Stored once.** Control stores each event once per node and
    `event_id`, as a row of the node's log (`v2_node_log`, source
    `maintenance`, `trace_id` the event id when it fits, `fields_json` the
    event, `logged_at` its `occurred_at`; level `error` for P0 and P1,
    `warning` for P2, `info` for P3 and recoveries), in the transaction that
    records the event id (kept 7 days).
  - **The answer.** Control answers every batch with one `MaintenanceAck`
    with the `request_id` of the batch, its `version`, and one
    `MaintenanceEventResult` per event, in the batch's order, with the
    `event_id` when the event could be read:
    - `persisted: true`: stored, by this delivery or an earlier one (a
      resend after a lost acknowledgement is not stored again); the Agent
      removes the event from its outbox;
    - `persisted: false` with an `error`: refused for good (malformed,
      another node's, a batch of another `version` or over the bounds, a
      node that no longer exists); Control refuses it again on every
      delivery, so the Agent may drop it;
    - `persisted: false` without an `error`: Control could not store it now
      (its database failed); the Agent keeps it and sends it again, not
      before `retry_after_ms`. The result carries `maintenance_unavailable`.

    Every refusal names its `error_code` ("Codes in acknowledgements").

    A bad event never ends the stream; only `maintenance_events` without
    `maintenance.v1` negotiated does. A result has the fields of the
    WebSocket protocol's per-event acknowledgement (`event_id`,
    `persisted`, `error`), so the Agent's outbox applies it unchanged (it
    removes persisted events only). Control never answered
    `maintenance_events` on the WebSocket: an outbox that had only the
    WebSocket did not drain against it.
  - Metric: `anixops_agent_maintenance_events_total{result}` (`persisted`,
    `duplicate`, `refused`, `unrecorded`).

- **Alive list** (`alive.v1`). Every user's number of online devices
  (distinct IPs) across all nodes: what UniProxy `alivelist` answers,
  counted from the same source (the online sets `TrafficReport.online` and
  UniProxy `alive` replace per node). The Agent enforces
  `NodeUser.device_limit` against it; with `users.v1` alone it counted
  only its own node's connections.
  - **The list.** `AliveList.entries` are the users with at least one
    device online, in user id order, each with `alive_count`; a user not
    listed has none. One list may span several messages of at most 10 000
    entries, each with the list's `revision` (it grows by one with every
    list sent on the session); the last has `last_page`. The Agent replaces
    its whole list when `last_page` arrives; an empty list is one empty
    page. `computed_at_unix_ms` is when Control counted.
  - **When.** After `HelloAck`, then whenever the counts change, checked
    once a minute (the default `pull_interval` at which Agents pulled
    `alivelist`). Control counts at most once every 10 seconds for all
    sessions. A lost list is replaced by the next one; a reconnect starts
    with a new list.
  - The list holds every user, not only the node's: it is what
    `alivelist` answers, and the Agent reads the entries of its users.
- **Plugin artifacts** (`artifacts.v1`). `AgentArtifacts`
  (`artifacts.proto`, a separate file of the same package) serves an
  enrolled Agent the signed plugin releases assigned to its node: what
  `GET /api/v3/agent/plugin-releases/{plugin_id}/{version}/manifest` and
  `/artifact` serve to the node API key.
  - **Authentication.** A valid agent client certificate of a proxy node,
    enabled; no `x-api-key` is read. The node is the certificate's.
  - **Authorization**, as the HTTP download's: the node has an enabled
    assignment of `plugin_id` at `version`, the installation is enabled,
    and the release is a signed official AnixOps release still verifying
    against the trust root.
  - **Addresses.** The request names the content address the
    `agent.plugin.install` operation's configuration carries: its
    `manifest` or `artifact` `sha256` and `size`, with `plugin_id` and
    `version` (the HTTP URL's path). The configuration is unchanged: an
    Agent with `artifacts.v1` reads the same fields and calls the service
    instead of the URL. Another address is refused.
  - **Answers.** `GetPluginManifest` returns the canonical manifest bytes
    with `PluginRelease`, the release as the HTTP download's `X-AnixOps-*`
    headers describe it (artifact and manifest digests and sizes,
    `signature`, `signature_algorithm` `ed25519`, `publisher`, `key_id`,
    `plugin_api_version`). `DownloadPluginArtifact` streams the artifact
    (at most 64 MiB) in chunks of at most 1 MiB with their `offset`, the
    first carrying `PluginRelease`. The bytes are the HTTP download's, so
    the Agent verifies them unchanged: the sizes and SHA-256 of both
    documents against the configuration, the manifest's ed25519 signature
    against its trust root, and the artifact's digest in the manifest.
  - A node runs at most 2 downloads at once (`ResourceExhausted`,
    `plugin_release_download_busy`: retry later).
  - The HTTP download stays: `agent_control.mtls: required` does not
    refuse it (it is not a legacy Agent channel), so an Agent that has not
    enrolled yet, or a Control without `artifacts.v1`, still installs over
    it with the node API key. An enrolled Agent no longer holds the key and
    uses `AgentArtifacts`.

## Diagnostic operation

`agent.diagnostic` is a desired operation an Agent runs when it lists the
capability of the same name. It replaces the WebSocket's `task.assign`.

**Payload.** `payload_json` is `{"task": task}`, the WebSocket's
`task.assign` payload:

```json
{"task": {"id": "fwdiag-4f1c...", "type": "diagnostic", "action": "forward.connect",
          "params": {"route_id": "01J...", "hop_index": 1, "generation": 7, "timeout_ms": 2000,
                     "target_policy": "public_only"},
          "timeout": 2}}
```

- `timeout` is in seconds.
- `deadline_unix_ms` is set. Past it, the Agent answers
  `operation deadline exceeded`, as for every operation.
- The Agent refuses an action it does not know: `FAILED`, with a message
  naming it.

**Answer.** The terminal state is `SUCCEEDED` with `state_json`:

```json
{"success": true, "output": "1 of 1 upstreams reachable", "error": "", "duration_ms": 14, "result": {...}}
```

- Control records `success`, `output`, `error` and `duration_ms` for the
  generic actions.
- A forward check also carries `result` (below).
- A check that ran is `SUCCEEDED` whatever it found. `FAILED` means the
  check could not run at all: an unknown action, or malformed parameters.

### Generic actions

The generic actions are those of the administrator routes
(`internal/service/agent_diagnostic_actions.go`). `service` is `gost`, and
nothing else.

| Action | Params | Does |
|---|---|---|
| `service_status` | `service` | the unit's status |
| `service_restart` | `service` | restarts the unit |
| `log_tail` | `service`, `lines` (1 to 1000, default 100) | the unit's last lines |

On a forward node, gost belongs to the forward component (its own unit and
hot updates). An Agent may refuse `service_restart` there as `FAILED`:
`managed by the forward component`.

### Forward diagnostic checks

Since F3c, Control's route diagnosis (`ForwardControl.DiagnoseRoute`,
`docs/architecture/forward-sdk.md` section 7.6) sends four more actions,
one hop of one route each. Only the kernel sends them: the administrator
routes and the KernelNodeOps `agent.diagnostic` kind refuse them
(`service.ForwardDiagnosticChecks`, `ValidateForwardDiagnosticCheck`).
Control sends them only to a session that lists `agent.diagnostic` and
`diag.v1`, and one at a time per node. An Agent lists `diag.v1` only when
it implements all four.

**Parameters**, normalized by Control:

| Param | Meaning |
|---|---|
| `route_id` | the route (1 to 64 printable characters) |
| `hop_index` | the hop, 0 to 7 |
| `generation` | optional: the generation Control wants the node to run |
| `timeout_ms` | each dial or wait, 100 to 10000 (default 3000) |
| `upstream` | `forward.connect` and `forward.udp_probe` only, optional: `host:port` of one upstream of the hop; probe only that one |
| `target_policy` | `forward.connect` and `forward.udp_probe` only: `public_only` (default) or `allow_private`, the most the check may dial among targets |

**The safety rule, mandatory.** The Agent resolves the hop from the
`NodeForwardState` it has applied, the `NodeHop` with that `route_id` and
`hop_index`, and probes only what that hop holds: its listener, and its
upstreams. A check names a hop, never an address to dial.

- When the hop is not in the applied state, the result is `failed` with
  code `hop_not_applied`.
- An `upstream` that is not one of the hop's upstreams (`address:port` as
  rendered) is `failed` with `unknown_upstream`, and nothing is dialled.
- For a target (an upstream without `node_ref`), every address its host
  resolves to passes `validate.CheckTargetAddress` under the stricter of
  the hop's `target_policy` and the `target_policy` parameter, the check
  the forwarding path already makes. A refused address is an item
  `skipped` with `target_not_allowed`, and is not dialled.
- A check never changes the node.

**`result`:**

```json
{"check": "forward.connect", "route_id": "01J...", "hop_index": 1, "generation": 7,
 "status": "failed", "code": "", "message": "1 of 2 upstreams unreachable",
 "items": [
   {"target": "198.51.100.10:443", "protocol": "tcp", "status": "ok", "rtt_us": 912},
   {"target": "198.51.100.11:443", "protocol": "tcp", "status": "failed", "code": "unreachable", "message": "connect: connection refused"}
 ]}
```

- `generation` is the generation the node runs.
- `status` and each item's `status` are `ok`, `failed`, `inconclusive` or
  `skipped`. The result is `failed` when an item failed, else
  `inconclusive` when one is, else `ok` (`skipped` when every item was).
- `code` is a stable reason. Control shows each item as one step;
  without items, the result itself.
- At most 64 items. `state_json` stays under 64 KiB.

| Action | Checks | Items and codes |
|---|---|---|
| `forward.listen` | The hop's own listener holds its port. On `ENGINE_NFTABLES`: the hop's rules are in the driver's table (`inet anixops_fwd`) and Observe reports no error for the hop. On `ENGINE_GOST`: a socket of the managed gost process is bound to the port (TCP listening, UDP bound), per protocol of `listen.protocol`. | one per protocol, target `address:port` (`*` for every address): `ok` `listening`; `failed` `not_listening` or `hop_error` |
| `forward.port_conflict` | No foreign object claims the port: a socket of another process bound to it, and a rule of another nftables table (any family, iptables-nft included, nat table included) that rewrites destinations (`dnat`, `redirect`, `tproxy`, or the `DNAT`, `REDIRECT` and `TPROXY` targets) and matches the port, as the nftables driver's conflict detection (`sdk/forward/driver/nftables`) reads the ruleset. Read only. | one per conflict: `failed` `foreign_listener` (message names the process) or `foreign_nat_rule` (message names the table and chain); none: result `ok`; `nft` missing: item `inconclusive` `nft_unavailable` |
| `forward.connect` | A TCP connect to each upstream within `timeout_ms`, all at once. For an encrypted link the carrier's TCP connect is enough; no handshake is needed. | one per upstream: `ok` `reachable` with `rtt_us`; `failed` `unreachable` or `timeout`; `skipped` `target_not_allowed` |
| `forward.udp_probe` | For each upstream, one datagram of at most 64 bytes (`anixops-diag <task id>`), from a connected UDP socket, and a wait of `timeout_ms` for any datagram back. | `ok` `reply` with `rtt_us`; `failed` `port_unreachable` (ICMP port unreachable); `inconclusive` `no_reply`, never `failed`: a UDP service need not answer; `skipped` `target_not_allowed` |

An Agent without these checks keeps working. Control marks its node steps
`SKIPPED` (`node_vantage_unavailable`) and dials what it can from its own
vantage.

## Forward link certificates

Encrypted links between forward nodes (gost's TLS, WSS, QUIC and gRPC
links) are mutual TLS with each node's link certificate (owner decision H28,
`docs/architecture/forward-sdk.md` sections 6.2 and 14). gost verifies a
certificate chain and the dialled server name, not SPIFFE URIs, and must
never hold the Agent's Control key, so Control runs a dedicated forward link
CA: a self-signed root, separate from the CA of agent, module and kernel
certificates, that signs nothing but link certificates.

**Profile.** A link certificate names exactly one node:

| Field | Value |
|---|---|
| Subject CN and the only DNS SAN | the node's identity name, `proxy-<id>` or `forward-<id>`: the `server_name` the planner gives an encrypted link by default |
| The only URI SAN | the node's SPIFFE ID, `spiffe://anixops/<cluster>/agent/forward-<id>`: the `peer_identity` and `ingress_peers` of the state |
| Extended key usage | `serverAuth` and `clientAuth` |
| Key usage | digital signature (and key encipherment for RSA) |
| Lifetime | 7 days, like the agent certificate; renew after `renew_after_unix`, two thirds of it |
| Issuer | the current forward link CA (ECDSA P-256, name-constrained to `spiffe://anixops` URIs) |

**Authorization.** Both RPCs need a valid agent client certificate: a call
without one, or with only the node API key or forward token, answers
`agent_cert_invalid`; a revoked, expired or foreign certificate answers its
`agent_cert_*` code. `IssueLinkCertificate` also needs the node enabled and
its Agent's last `Hello` to have negotiated `forward.v1` (proxy and forward
nodes alike): otherwise `link_cert_not_negotiated`. The CSR may name only the
node's DNS name and SPIFFE ID (both may be left out; Control assigns both),
no IP address or e-mail name, and its key must not be the agent
certificate's key: otherwise `link_cert_request_invalid`. Control copies
nothing from the CSR but its public key.

**Trust bundle.** `LinkCertificate.trust_bundle_der` and
`GetLinkTrustBundle` answer every link CA a peer may present a certificate
from: the current CA, a next CA (trusted at once, signing only one link
certificate lifetime after the rotation) and a retired CA until the last
link certificate it signed has expired. A node that refreshes the bundle at
least at every renewal therefore trusts a new CA before any peer presents a
certificate it signed.

**Revocation.** Revoking, replacing or disabling a node's credentials,
deleting the node or `RetireNode` revokes its link certificates with its
agent certificates (`v4_kernel_forward_link_certificate.revoked_at`), and
the node gets no new one. Peers do not check revocation (gost has no CRL or
OCSP check): a revoked link certificate verifies until its `not_after`, at
most 7 days later. Until then what keeps a removed node out is its peers'
state: their listeners admit only the `ingress_sources` addresses of the
routes' previous hops. A disabled or deleted node leaves Control's
forwarding inventory, and a plan that still names it is refused, so the
administrator removes it from its routes, whose next plan drops its
addresses from its peers' `ingress_sources` and upstreams.

**What the Agent does (F3b).**

1. **A key of its own.** Generate a separate key pair for the link
   certificate (ECDSA P-256 recommended; P-384, Ed25519 and RSA of at least
   2048 bits are accepted). Never reuse the agent identity key, and never
   give gost the agent key or certificate.
2. **When to ask.** After the agent certificate is enrolled or renewed and
   a `HelloAck` lists `forward.v1`, call `IssueLinkCertificate` over the
   mTLS connection with a CSR for the link key (its SANs, if any, the node's
   DNS name and SPIFFE ID). Control records the `Hello`'s `forward.v1`
   before it sends the `HelloAck`, so the call may follow the `HelloAck`
   at once. Renew at `renew_after_unix` with a new key, and whenever the
   node has no valid link certificate. A node whose `forward.v1` is not
   negotiated has no link certificate to ask for: on
   `link_cert_not_negotiated` (the last `Hello` did not negotiate it, or
   Control could not record it, which it logs) ask again after the next
   `HelloAck` that lists it, reconnecting with backoff if needed; on
   `link_cert_unavailable` keep what it has and retry with backoff; on
   `link_cert_request_invalid` fix the request (an Agent bug); on an
   `agent_cert_*` code handle the agent certificate first.
3. **Where to store it.** Not in the Agent's PKI directory: in gost's
   directory, `/var/lib/anixops-gost/tls` (the gost driver's
   `DefaultLinkCert`, `DefaultLinkKey` and `DefaultLinkCA`):
   `link.crt` (the certificate, PEM), `link.key` (the link key, PKCS#8
   PEM) and `link-ca.crt` (the whole link trust bundle, PEM). The directory
   is owned by the Agent's user with group `anixops-gost`, mode 0750; the
   files are owned by the Agent's user, group `anixops-gost`, mode 0640, so
   only the Agent writes them and only gost reads them. Write each file to
   a temporary file in the same directory and rename it; write the key and
   the certificate before the bundle that must verify the peers.
4. **Keep the bundle current.** Rewrite `link-ca.crt` from every
   `IssueLinkCertificate` answer, and call `GetLinkTrustBundle` at least
   hourly and at start-up; rewrite it whenever the set of CAs changes, not
   only at renewal.
5. **Reload gost.** gost reads the files when it creates its services and
   hops. After any of the three files changed, call the gost driver's
   `ReloadCredentials` (`(*gost.Driver).ReloadCredentials(ctx)`), not the
   supervisor's reload: it checks that the files load, re-creates through
   gost's web API the services with encrypted listeners and replaces the
   hops with encrypted dialers, and records the hops whose services it
   re-created as starting a new `counter_epoch`, handing their last
   counters to the `WithRetiredCounters` hook (report them as for any
   ended epoch). Other hops keep their epochs; established connections run
   on with the old certificate. A node with a mux or QUIC listener
   restarts gost instead: every connection ends and every hop starts a new
   epoch. It does nothing while gost is stopped (a start reads the files)
   and is serialised with `Apply`. A supervisor reload outside the driver
   would leave the epoch unmoved, so Control would see the counters fall
   and count nothing for that interval. Without the three files the gost
   driver carries RAW links only.
6. **On revocation.** When the agent certificate is revoked
   (`agent_cert_revoked`), delete the link key and certificate along with
   it and reload gost; ask again after enrolling anew.

## Agent health metrics

`Heartbeat.metrics` carries plugin telemetry (`plugin.*`, persisted per
assigned plugin) and the Agent's own health: names `agent_control_*`,
`agent_identity_*` and `agent_dataplane_*` (lowercase letters, digits and
`_`, at most 108 bytes), finite values, at most 64. Control keeps the latest
heartbeat's set of these with the live session and shows it in the session
views (the node's agent-control status, the transport inventory); a
heartbeat without any keeps the last set. Other names are dropped.

## Error codes

A refused call names its reason in the `x-anix-error-code` trailer
(`agentcontrol.MetadataErrorCode`), and its status message starts with the
same code. HTTP answers of the legacy agent paths carry it as the `code` of
their JSON body.

| Code | Status | Where | Meaning, and what the Agent does |
|---|---|---|---|
| `agent_mtls_required` | `Unauthenticated` | `ControlStream`, `Enroll`, the legacy HTTP and WebSocket agent paths (HTTP 403) | `agent_control.mtls: required` refuses the node API key (and its `Enroll` bootstrap): enroll with a one-time credential and present the certificate |
| `agent_cert_revoked` | `Unauthenticated` (`PermissionDenied` when the node was found disabled or deleted after the certificate check) | `ControlStream` (at connection, and on an open stream at the next `Heartbeat`), `Renew`, `GetTrustBundle`, `IssueLinkCertificate`, `GetLinkTrustBundle`, the v2board services | the certificate, its enrollment or its node's credentials were revoked, or the node was disabled or deleted, which revokes them: discard it and enroll again |
| `agent_cert_expired` | `Unauthenticated` | the same | the certificate's `not_after` has passed: discard it and enroll again |
| `agent_cert_invalid` | `Unauthenticated` | the same; `Renew`, `GetTrustBundle`, `IssueLinkCertificate` and `GetLinkTrustBundle` without a certificate | not an agent certificate of this Control: unparsable, not chaining to the agent trust bundle, not yet valid, without client-auth usage or exactly one agent SPIFFE ID, or presented to a Control without the agent PKI: enroll again |
| `agent_cert_wrong_cluster` | `Unauthenticated` | the same | an agent certificate of another cluster: enroll with this Control |
| `agent_cert_wrong_node` | `Unauthenticated`, or `PermissionDenied` for an envelope or the v2board services | `ControlStream`, the v2board services | the certificate names another node than `x-node-id` or `x-node-kind`, an envelope's `node_id`, or is a forward node's on the proxy-only v2board services: a configuration error of the Agent; keep the certificate |
| `agent_enrollment_rejected` | `Unauthenticated` | `Enroll` | the bootstrap is unusable (unknown, used, expired or revoked enrollment credential, wrong node key or token, malformed `x-node-id` or `x-node-kind`, none given); one answer for all, so credentials cannot be probed |
| `link_cert_not_negotiated` | `FailedPrecondition` | `IssueLinkCertificate` | the node's Agent did not negotiate `forward.v1` in its last `Hello`: ask again after a `HelloAck` that lists it |
| `link_cert_request_invalid` | `InvalidArgument` | `IssueLinkCertificate` | the CSR is malformed, uses an unsupported key or the agent certificate's key, or names anything but the node's DNS name and SPIFFE ID: an Agent bug; fix the request, with a fresh key |
| `link_cert_unavailable` | `FailedPrecondition` | `IssueLinkCertificate`, `GetLinkTrustBundle` | this Control issues no link certificates (no built-in CA, an external PKI, `agent_control.mtls: off`) or has no link CA yet: keep the current link certificate and retry later |

| `agent_capability_not_negotiated` | `InvalidArgument` | `ControlStream` | a data-plane payload whose capability the session did not negotiate; the stream ends: an Agent bug |
| `invalid_plugin_release_address` | `InvalidArgument` | `AgentArtifacts` | the address lacks `plugin_id`, `version`, a 64-hex `sha256` or a positive `size` |
| `plugin_release_address_mismatch` | `InvalidArgument` | `AgentArtifacts` | the address is not the release's verified content: reconcile the operation again |
| `plugin_release_not_assigned` | `PermissionDenied` | `AgentArtifacts` | the certificate's node has no enabled assignment of the release (another node's release, a forward node, a disabled installation or an unofficial release) |
| `plugin_release_not_found` | `NotFound` | `AgentArtifacts` | the assigned release or its artifact is gone |
| `plugin_release_integrity_failed` | `FailedPrecondition` | `AgentArtifacts` | the stored release no longer verifies; an operator must republish it |
| `plugin_release_download_busy` | `ResourceExhausted` | `AgentArtifacts.DownloadPluginArtifact` | the node runs as many downloads as allowed: retry later |

On `AgentArtifacts` the certificate codes above apply too (`agent_cert_*`,
and `agent_cert_revoked` with `PermissionDenied` for a disabled or deleted
node), and `agent_cert_invalid` answers a call without a certificate.

A transient failure (`Unavailable`: the certificate check or the release
could not be read from the database) carries no code: retry.
`sdk/agentcontrol` names the codes (`ErrorCode*`).

### Codes in acknowledgements

The data plane's answers carry a machine-readable `error_code` next to
their text `error`, which stays for people. A code is lowercase words joined
by `_`, at most 64 bytes; a later Control may add codes, and an Agent treats
an unknown refusal code as it treated a non-empty `error` before codes
existed. `sdk/agentcontrol` names them (`ReportErrorCode*`,
`MaintenanceErrorCode*`, `ConfigErrorCode*`).

| Code | In | Meaning, and what the Agent does |
|---|---|---|
| `report_batch_id_invalid` | `ReportAck` | no `batch_id`, or one over 128 bytes: refused for good, drop the batch |
| `report_invalid` | `ReportAck` | a malformed entry (a `user_id` of 0, `fields_json` that is not JSON): drop |
| `report_counter_overflow` | `ReportAck` | bytes beyond the 64-bit counter range: drop |
| `report_node_gone` | `ReportAck` | the node no longer exists: drop |
| `report_unavailable` | `ReportAck` (`applied` false, `error` empty, `retry_after_ms` set) | Control could not record the batch now: keep it, send it again not before `retry_after_ms` |
| `maintenance_schema_unsupported` | `MaintenanceEventResult` | the batch's `version` is not `anixops.maintenance/v1`: refused for good |
| `maintenance_batch_too_large` | `MaintenanceEventResult` | more than 50 events or 256 KiB in the batch: refused; send smaller batches |
| `maintenance_event_invalid` | `MaintenanceEventResult` | the event is not a valid `anixops.maintenance/v1` event: refused for good |
| `maintenance_event_wrong_node` | `MaintenanceEventResult` | the event's `node_id` is not the stream's node: refused for good |
| `maintenance_node_gone` | `MaintenanceEventResult` | the node no longer exists: refused for good |
| `maintenance_unavailable` | `MaintenanceEventResult` (`persisted` false, `error` empty, `retry_after_ms` set) | Control could not store the event now: keep it, send it again not before `retry_after_ms` |
| `config_format_unsupported` | `ConfigStatus` (set by the Agent) | the snapshot's `format` is unknown to the Agent |
| `config_hash_mismatch` | `ConfigStatus` (Agent) | `config_hash` is not the SHA-256 of `config_json` |
| `config_invalid` | `ConfigStatus` (Agent) | the document does not parse or fails the Agent's checks |
| `config_apply_failed` | `ConfigStatus` (Agent) | the Agent could not run the configuration |

- **Every refusal has a code.** A `ReportAck` with an `error` and a
  `MaintenanceEventResult` with an `error` always carry their code.
- **Transient answers.** A `MaintenanceEventResult` that is neither
  persisted nor refused carries `maintenance_unavailable` and
  `retry_after_ms` (30 s). Its `error` stays empty, so an Agent built
  before codes keeps the event, as before.
- **Transient report acknowledgements are negotiated.** Before codes,
  Control sent no `ReportAck` for a batch it could not record, because an
  Agent drops a batch on any `ReportAck`. Control keeps that silence for
  every Agent except one whose `Hello` lists `reports.v1` with the
  attribute `transient_ack: "v1"`
  (`agentcontrol.ReportsAttributeTransientAck`); Control echoes the
  attribute on its `reports.v1` (`agentcontrol.TransientReportAcks`) and
  then answers such a batch with `applied: false`, no `error`,
  `report_unavailable` and `retry_after_ms` (15 s). Such an Agent keeps a
  batch answered with `report_unavailable`.
- **Configuration.** Control sends nothing back for a `ConfigStatus`; the
  Agent sets `error_code` when `applied` is false. Control keeps it with
  the node's last status (`v4_kernel_node_config_status.reported_error_code`;
  a malformed code is not kept, unknown well-formed ones are) and a
  `node.sync` that fails on the status reports it at the start of its
  message (`config_apply_failed: <error>`).

The checked-in Go files are generated, not handwritten. From the repository
root, run:

```bash
bash sdk/api/agent/gen.sh
```

The script maps the virtual path `api/grpc/agent/v1` onto this directory so the
registered file name stays `api/grpc/agent/v1/agent.proto` (and
`agent_enrollment.proto`, `artifacts.proto` beside it).

Verified generator versions: `libprotoc 29.2`, `protoc-gen-go v1.36.11`, and
`protoc-gen-go-grpc 1.6.1`.
