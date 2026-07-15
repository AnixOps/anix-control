# anix.agent.v1 Control Stream

`AgentControlService.ControlStream` is the v3 primary Agent-first control
channel. Node authentication remains the existing `x-node-id` plus node-scoped
`x-api-key` metadata contract.

The first Agent message must be `Hello`. The Control response supplies a
session ID and heartbeat interval. Desired operations use independent
`request_id`, `operation_id`, and monotonic `revision` values. An
`OperationAck` confirms receipt only; `ObservedState` reports applying and
terminal runtime state. Every ACK carries the active `session_id` and the
operation `revision`; every observed state also carries the active
`session_id`. Control rejects messages from a replaced session or messages
whose revision does not match the retained desired operation.

The Agent sends its latest observed revision in the `Hello` envelope. Control
reconciles its revision counter to at least that value before replying, sends
`HelloAck` before publishing the connection, then replays retained operations
as one serialized batch. A concurrently dispatched operation therefore cannot
interleave with that replay batch. The Agent checks revision freshness both
when accepting an operation and immediately before execution; an operation
that became stale in the queue is reported as `SUPERSEDED` without invoking
the runtime handler.

Control retains unobserved operations in memory and replays them after a stream
reconnect. Delivery is at least once, so operation handlers must be idempotent.
A future durable worker adapter must persist desired operations across Control
process restarts, and the Agent's bounded completion cache is not durable
across Agent process restarts.

Control integration points:

- `AgentControlManager.DispatchOperation` sends desired operations and waits
  for receipt ACKs. It rejects an operation unless the current Agent session
  advertised the matching capability name in `Hello`.
- `DesiredOperationDispatcher` is the adapter surface for durable workers.
- `AddObservedStateHandler` is the adapter surface for durable state write-back.

Durable `ForwardRuntimeJob` workers can be connected through the dispatcher and
observed-state adapters, but that write-through is not enabled yet. The current
Agent does not advertise or execute a `forward.task` capability, so legacy
`clean_agent` jobs must keep their existing polling/result path until a real
local forward executor is implemented. REST, the existing v2board gRPC
services, and the legacy WebSocket sync channel remain compatibility/fallback
transports during the v3 migration.

The v3 development template enables the gRPC listener on loopback port `50051`.
Production keeps it disabled until the operator configures server TLS or an
HTTP/2-capable restricted proxy and deliberately changes the bind address;
plaintext `50051` must not be exposed to the public Internet.

The checked-in Go files are generated, not handwritten. Generation used:

```bash
PATH=/tmp/anix-protoc/tool/bin:/tmp/anix-protoc/bin:$PATH \
  protoc -I . \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  api/grpc/agent/v1/agent.proto
```

Verified generator versions: `libprotoc 29.2`, `protoc-gen-go v1.36.11`, and
`protoc-gen-go-grpc 1.6.1`.
