### Removed

- The legacy Agent channels are gone: the node API key on the Agent Control
  stream, `/api/v2/agent/*` (register, heartbeat, tasks, result, monitor,
  WebSocket) and `/api/v2/node/*` (register, heartbeat, runtime-health,
  WebSocket) now answer 404. Agents authenticate with enrolled client
  certificates only; `Enroll` takes only one-time enrollment credentials.
  UniProxy and the v2board gRPC services are unchanged.
- `agent_control.mtls` (`off`, `optional`, `preferred`) and
  `agent_control.legacy_sunset` are removed and ignored with a startup note;
  the `anixops_agent_legacy_requests_total` and `anixops_agent_mtls_mode`
  metrics are gone.
- `GET /api/v4/kernel/agents/transports` no longer reports `mode` or `sunset`.

### Changed

- The admin agent list, task and diagnostic routes (`/api/v2/admin/agent/*`)
  act on the Agent Control stream; `GET /api/v2/admin/agent/monitor` answers
  404 for every node because monitor snapshots came from the retired REST
  channel.
