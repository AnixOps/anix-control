# anix.agent.v1 Control Stream

This document is the canonical protocol description for the SDK-owned
`anix.agent.v1` control stream.

Source of truth: this directory of `github.com/AnixOps/anix-control/sdk`
(`sdk/api/agent/v1`, Go package `agentv1pb`). The contract moved here from
`github.com/AnixOps/anix-agent/sdk`, which is frozen at v1.1.0. The move
changed only `option go_package`: the protobuf package, services, messages,
field numbers and types, and the registered file name
`api/grpc/agent/v1/agent.proto` are unchanged, so Agents built against the old
SDK keep working. `agent_descriptor_test.go` proves the file descriptor equals
v1.1.0's apart from `go_package`, and `internal/tests/protocompat` guards it
from now on.

`AgentControlService.ControlStream` is the v3 primary Agent-first bidirectional
gRPC control channel. Authentication reuses the configured node ID and node API
key as `x-node-id` and `x-api-key` metadata.

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

The checked-in Go files are generated, not handwritten. From the repository
root, run:

```bash
bash sdk/api/agent/gen.sh
```

The script maps the virtual path `api/grpc/agent/v1` onto this directory so the
registered file name stays `api/grpc/agent/v1/agent.proto`.

Verified generator versions: `libprotoc 29.2`, `protoc-gen-go v1.36.11`, and
`protoc-gen-go-grpc 1.6.1`.
