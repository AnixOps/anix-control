# Clean Forward Runtime Spec

## Scope

This runtime is a clean-room replacement for panel-managed forwarding execution.
The panel owns desired state and queues jobs. A separate binary named
`v2forward-agent` pulls jobs, applies local runtime changes, and reports results.

The first implementation in this repository only adds the panel-side contract:

- Runtime backend name: `clean_agent`
- Job queue: `v2_forward_runtime_job`
- Agent registry: `v2_forward_clean_agent`
- Public agent API:
  - `POST /api/v2/forward-agent/register`
  - `POST /api/v2/forward-agent/heartbeat`
  - `POST /api/v2/forward-agent/report`
  - `GET /api/v2/forward-agent/install.sh`
- Admin API:
  - `GET /api/v2/admin/forward/agents`
  - `POST /api/v2/admin/forward/agents`
  - `POST /api/v2/admin/forward/agents/:id/revoke`

No Forwardx source code, protocol implementation, or generated artifacts are
copied into this repository.

## Runtime Model

Set the panel backend to:

```yaml
forward_runtime:
  backend: "clean_agent"
  clean_agent:
    public_url: "https://panel.example.com"
    heartbeat_interval_seconds: 10
    action_timeout_seconds: 120
    token_expire_seconds: 0
```

`clean_agent` uses execution-node semantics. For the first version:

- Only type `1` port-forward tunnels are supported.
- The target agent is selected by the tunnel execution node ID.
- The agent record stores `nodeId`; it only claims pending jobs for that node.
- Jobs remain pending until an online agent pulls them.

## Agent Authentication

Admin creates an agent token:

```bash
curl -X POST "$PANEL/api/v2/admin/forward/agents" \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "Content-Type: application/json" \
  -d '{"name":"relay-1","nodeId":1}'
```

The response contains the token once. Store it in
`/etc/v2board-forward-agent/config.yaml`.

Agent requests may pass credentials in JSON or headers:

- `X-Agent-ID: <agentId>`
- `X-Agent-Token: <token>`

## Agent Flow

1. `POST /api/v2/forward-agent/register`
2. `POST /api/v2/forward-agent/heartbeat`
3. Panel returns queued actions.
4. Agent applies the action locally.
5. `POST /api/v2/forward-agent/report`

Heartbeat response shape:

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "heartbeatInterval": 10,
    "actions": [
      {
        "jobId": 1,
        "action": "create",
        "resourceType": "panel_forward",
        "forwardId": 10,
        "tunnelId": 2,
        "nodeId": 1,
        "payload": {}
      }
    ]
  }
}
```

Report request:

```json
{
  "agentId": 1,
  "token": "<token>",
  "jobId": 1,
  "success": true,
  "result": "applied",
  "upload": 0,
  "download": 0
}
```

On success the panel updates the job to `success` and updates the forward runtime
status. On failure it records the error and marks the forward runtime as failed.

## Deployment Skeleton

Download the generated install scaffold from the panel:

```bash
curl -fsSL "$PANEL/api/v2/forward-agent/install.sh" -o install-v2forward-agent.sh
sudo AGENT_TOKEN="<TOKEN>" NODE_ID="<FORWARD_NODE_ID>" bash install-v2forward-agent.sh
```

The script expects a clean-room `v2forward-agent` binary at:

```text
/usr/local/bin/v2forward-agent
```

This repository does not yet ship that binary. Build it from clean-room source
once the agent implementation is added.

## Production Database Migration

`cmd/server/main.go` runs AutoMigrate only in `development` and `test`. For
production, add the schema explicitly before switching the backend.

Required changes:

```sql
ALTER TABLE v2_forward_runtime_job ADD COLUMN agent_id integer;
ALTER TABLE v2_forward_runtime_job ADD COLUMN claimed_at datetime;
CREATE INDEX idx_v2_forward_runtime_job_agent_id ON v2_forward_runtime_job(agent_id);

CREATE TABLE v2_forward_clean_agent (
  id integer primary key,
  node_id integer,
  name varchar(100) not null,
  token varchar(160) not null,
  version varchar(50),
  hostname varchar(255),
  os varchar(50),
  arch varchar(50),
  kernel varchar(120),
  public_ip varchar(64),
  private_ip varchar(64),
  capabilities text,
  status integer default 0,
  last_seen datetime,
  last_error text,
  revoked_at datetime,
  created_at datetime,
  updated_at datetime
);
CREATE UNIQUE INDEX idx_v2_forward_clean_agent_token ON v2_forward_clean_agent(token);
CREATE INDEX idx_v2_forward_clean_agent_node_id ON v2_forward_clean_agent(node_id);
CREATE INDEX idx_v2_forward_clean_agent_status ON v2_forward_clean_agent(status);
CREATE INDEX idx_v2_forward_clean_agent_last_seen ON v2_forward_clean_agent(last_seen);
```

## Compatibility Notes

- `gost`, `nftables_ansible`, and `iptables_ansible` behavior is unchanged.
- `clean_agent` jobs are not consumed by the local Ansible executor.
- Existing `/api/v2/agent/*` APIs are unrelated and remain unchanged.
- The clean agent API is intentionally small and versionable.
