# Forwarding API

This document is the API contract for the current forwarding implementation. It
records what exists now, which routes are Flux-compatible, and which routes are
local extensions.

## Response Shapes

Flux-shaped panel endpoints return:

```json
{
  "code": 0,
  "msg": "操作成功",
  "ts": 1712300000000,
  "data": {}
}
```

Flux-shaped errors return HTTP 200 with:

```json
{
  "code": -1,
  "msg": "error message",
  "ts": 1712300000000,
  "data": null
}
```

Clean-agent endpoints return:

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

Older admin node/rule endpoints still return standard HTTP status codes with
`data`, `message`, or `error` fields. Do not document those as Flux-compatible.

## Authentication

- Public clean-agent install script: no authentication.
- Public clean-agent register/heartbeat/report: agent token in body or
  `X-Agent-Token`; heartbeat/report may use `X-Agent-ID`.
- User forward routes: JWT user authentication.
- Admin forward routes: JWT plus admin authentication.
- Internal traffic routes: application token authentication.
- Legacy `/flow/upload`: application token authentication.

## Flux-Shaped User Routes

Base path: `/api/v2`

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/forward/create` | Create a forward for the current user. |
| `POST` | `/forward/list` | List forwards visible to the current user. |
| `POST` | `/forward/update` | Update an owned forward. |
| `POST` | `/forward/delete` | Delete an owned forward after runtime cleanup. |
| `POST` | `/forward/force-delete` | Delete an owned forward even when runtime cleanup cannot complete. |
| `POST` | `/forward/pause` | Pause an owned forward. |
| `POST` | `/forward/resume` | Resume an owned forward. |
| `POST` | `/forward/diagnose` | Diagnose one owned forward. |
| `POST` | `/forward/update-order` | Update owned forward ordering. |
| `POST` | `/tunnel/user/tunnel` | List tunnels available to the current user. |

### Create Forward

```json
{
  "name": "ssh-home",
  "tunnelId": 1,
  "inPort": 10022,
  "remoteAddr": "10.0.0.10:22",
  "interfaceName": "",
  "strategy": "fifo"
}
```

`inPort` may be omitted when the tunnel has a usable configured port range.

### Update Forward

```json
{
  "id": 1,
  "userId": 2,
  "name": "ssh-home",
  "tunnelId": 1,
  "inPort": 10022,
  "remoteAddr": "10.0.0.10:22",
  "interfaceName": "",
  "strategy": "fifo"
}
```

Non-admin callers are still constrained to their own records.

### ID Actions

Used by delete, force-delete, pause, and resume:

```json
{
  "id": 1
}
```

### Diagnose Forward

```json
{
  "forwardId": 1
}
```

### Update Order

```json
{
  "forwards": [
    { "id": 1, "inx": 0 },
    { "id": 2, "inx": 1 }
  ]
}
```

## Flux-Shaped Admin Routes

Base path: `/api/v2/admin`

Admin mirrors exist for the user forward actions:

- `POST /forward/create`
- `POST /forward/list`
- `POST /forward/update`
- `POST /forward/delete`
- `POST /forward/force-delete`
- `POST /forward/pause`
- `POST /forward/resume`
- `POST /forward/diagnose`
- `POST /forward/update-order`

Admin tunnel routes:

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/tunnel/create` | Create a tunnel. |
| `POST` | `/tunnel/list` | List admin tunnel records. |
| `POST` | `/tunnel/update` | Update a tunnel. |
| `POST` | `/tunnel/delete` | Delete a tunnel. |
| `POST` | `/tunnel/diagnose` | Diagnose a tunnel. |
| `POST` | `/tunnel/user/tunnel` | List user-visible tunnels. |
| `POST` | `/tunnel/user/assign` | Grant a user access to a tunnel. |
| `POST` | `/tunnel/user/list` | List tunnel grants for a user. |
| `POST` | `/tunnel/user/remove` | Remove a user-tunnel grant. |
| `POST` | `/tunnel/user/update` | Update a user-tunnel grant. |

### Create Tunnel

```json
{
  "name": "hk-ingress",
  "inNodeId": 1,
  "outNodeId": null,
  "type": 1,
  "flow": 2,
  "trafficRatio": 1,
  "interfaceName": "",
  "protocol": "tcp",
  "tcpListenAddr": "0.0.0.0",
  "udpListenAddr": "0.0.0.0"
}
```

### Update Tunnel

```json
{
  "id": 1,
  "name": "hk-ingress",
  "flow": 2,
  "trafficRatio": 1,
  "interfaceName": "",
  "protocol": "tcp",
  "tcpListenAddr": "0.0.0.0",
  "udpListenAddr": "0.0.0.0"
}
```

### Assign User Tunnel

```json
{
  "userId": 2,
  "tunnelId": 1,
  "flow": 100,
  "num": 10,
  "flowResetTime": 0,
  "expTime": 1712300000000,
  "speedId": null
}
```

### List User Tunnels

```json
{
  "userId": 2
}
```

### Update User Tunnel

```json
{
  "id": 1,
  "flow": 100,
  "num": 10,
  "flowResetTime": 0,
  "expTime": 1712300000000,
  "status": 1,
  "speedId": null
}
```

## Runtime Admin Routes

Base path: `/api/v2/admin`

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/forward/runtime/jobs` | List runtime jobs, filtered by `backend`, `status`, `forward_id`, and `limit`. |
| `GET` | `/forward/runtime/status` | Runtime status for the configured backend. |
| `GET` | `/forward/runtime/doctor` | Runtime readiness diagnostics for the configured backend. |
| `GET` | `/forward/local/status` | Local Ansible runtime status. |
| `GET` | `/forward/local/doctor` | Local Ansible runtime diagnostics. |
| `GET` | `/forward/nodex/status` | NodeX runtime status. |
| `GET` | `/forward/nodex/doctor` | NodeX runtime diagnostics. |
| `POST` | `/forward/sync-backend` | Queue or execute sync for one backend. |

`/forward/sync-backend` request:

```json
{
  "backend": "gost"
}
```

Allowed backend values:

- `gost`
- `nftables_ansible`
- `iptables_ansible`
- `clean_agent`

## Admin Node Routes

Base path: `/api/v2/admin`

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/forward/nodes` | List nodes with `type`, `scope`, `status`, `page`, and `page_size` filters. |
| `POST` | `/forward/nodes` | Create a forward node. |
| `GET` | `/forward/nodes/:id` | Get one node. |
| `PUT` | `/forward/nodes/:id` | Update one node. |
| `DELETE` | `/forward/nodes/:id` | Delete one node. |
| `POST` | `/forward/nodes/:id/check` | Run a TCP reachability check. |
| `POST` | `/forward/nodes/:id/toggle` | Enable or disable one node. |
| `POST` | `/forward/nodes/:id/sync-stats` | Sync node statistics. |
| `POST` | `/forward/test-connection` | Test gost API connectivity. |

`ForwardNode` online status is only coarse TCP reachability to `host:port`.
Runtime attachment must be verified through runtime jobs and relay state.

## Admin Legacy Rule Routes

Base path: `/api/v2/admin`

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/forward/rules` | List legacy relay/exit rules. |
| `POST` | `/forward/rules` | Create a legacy rule. |
| `GET` | `/forward/rules/:id` | Get one legacy rule. |
| `PUT` | `/forward/rules/:id` | Update one legacy rule. |
| `DELETE` | `/forward/rules/:id` | Delete one legacy rule. |
| `POST` | `/forward/rules/:id/toggle` | Enable or disable one legacy rule. |
| `GET` | `/forward/stats` | Aggregate forwarding stats. |

These routes predate the Flux-shaped panel API. Keep behavior-compatible changes
small and covered by tests.

## Internal Traffic Routes

Base path: `/api/v2/internal`

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/forward/traffic/report` | Upload traffic deltas. |
| `POST` | `/forward/traffic/snapshot` | Upload cumulative traffic totals. |
| `POST` | `/forward/traffic/upload` | Upload Flux-style flow data. |

Delta report request:

```json
{
  "records": [
    { "forwardId": 1, "upload": 1024, "download": 2048 }
  ]
}
```

Snapshot report request:

```json
{
  "records": [
    {
      "forwardId": 1,
      "backend": "gost",
      "uploadTotal": 1048576,
      "downloadTotal": 2097152
    }
  ]
}
```

Flux-style flow upload request:

```json
{
  "n": "forward_1_2_3",
  "u": 1024,
  "d": 2048
}
```

## Clean Agent Routes

Base path: `/api/v2/forward-agent`

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/install.sh` | Render an installation script. |
| `POST` | `/register` | Register or refresh an agent with a token. |
| `POST` | `/heartbeat` | Mark an agent online and claim pending work. |
| `POST` | `/report` | Report a claimed job result and optional traffic. |

Admin token routes:

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/api/v2/admin/forward/agents` | List clean agents. |
| `POST` | `/api/v2/admin/forward/agents` | Create an agent token. |
| `POST` | `/api/v2/admin/forward/agents/:id/revoke` | Revoke an agent. |

Create token request:

```json
{
  "name": "relay-agent-1",
  "nodeId": 1
}
```

Register request:

```json
{
  "name": "relay-agent-1",
  "token": "agent-token",
  "nodeId": 1,
  "version": "0.1.0",
  "hostname": "relay-1",
  "os": "linux",
  "arch": "amd64",
  "kernel": "6.1",
  "publicIp": "203.0.113.10",
  "privateIp": "10.0.0.10",
  "capabilities": ["tcp", "udp"]
}
```

Heartbeat request:

```json
{
  "agentId": 1,
  "token": "agent-token",
  "limit": 10
}
```

Report request:

```json
{
  "agentId": 1,
  "token": "agent-token",
  "jobId": 100,
  "status": 2,
  "success": true,
  "result": "applied",
  "error": "",
  "upload": 1024,
  "download": 2048
}
```
