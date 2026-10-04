# Forward v4 API

`/api/v4/forward/*` is the v4.2 forwarding API (forward-sdk.md section 3,
F5a). The forward package serves it on the kernel's `ForwardControl`
(`kernel.forward.v1`). It reads and writes no table itself, and no answer
carries a node credential. The flux-compatible `/api/v2/forward/*` routes
in [`api.md`](api.md) stay until F5d removes them.

- The new UI is F5b ([`docs/guide/forwarding.md`](../guide/forwarding.md)).
- The command line is `anix-control forward ...`, described at the end of
  this page.

## How It Is Served

- **Routing.** The forward package declares the manifest control route
  `/api/v4/plugins/forward/*`. The kernel routes `/api/v4/forward/*` to the
  same route, with the path rewritten to that namespace, and the package
  host answers both. So `GET /api/v4/forward/routes` and
  `GET /api/v4/plugins/forward/routes` are the same call.
- **Who may call.** The routes are in the `/api/v4` group: a JWT, an
  administrator (`AdminAuth`) and the package's `forward.api` permission.
  - The package permission works as for every package control route. While
    no access group grants anything for the forward package, every
    administrator may read and write. Once a group with a `forward.api`
    grant exists, only its members may.
  - Every `DELETE` needs a super administrator: an administrator who is not
    staff (`service.IsSuperAdmin`, the rule route-mode switches and install
    tokens follow). Others get `403 super_admin_required`, and the request
    never reaches the package.
  - The list answers (`GET /routes`, `GET /nodes`, `GET /ansible-machines`)
    carry `can_delete`: whether the caller may `DELETE` (F5b, D7). The
    kernel resolves the rule and passes it to the package as the
    principal's `super_admin`, the way route modes answer `can_switch`.
- **Discovery.** `GET /api/v3/extensions` lists each package's
  `control_routes` to an actor who holds the package's `api` permission. The
  web app shows the forwarding pages when the forward package lists
  `/api/v4/plugins/forward/*`.
- **Audit.** Every `POST`, `PUT` and `DELETE` under `/api/v4/forward/` is
  written to the audit log as module `forward`, with the action derived
  from the path (`create`, `update`, `delete`, `pause`, `resume`, `toggle`,
  `diagnose`) and the redacted body.
  - `POST /routes/preview` is the exception: it stores nothing and the
    route editor sends one a second after each pause in typing, so it is
    logged at debug level only and never written to the audit table (F5b,
    D13).
- **Package version.** The routes exist once the forward package of a
  release that declares the control route is installed. Before that they
  answer `404 plugin_route_not_found`.

## Editions (H23)

Everything on this page is in both editions: routes, load balancing,
failover, the node inventory, statistics and onboarding.

`config/editions.json` reserves these prefixes for the commercial
edition's v4.3 features:

| Prefix | For |
|---|---|
| `/api/v4/forward/self/` | user self-service forwarding |
| `/api/v4/forward/plans/` | forward plans and renewal |
| `/api/v4/forward/multipliers/` | billing multipliers |

- The community edition answers a request under these prefixes as a route
  that does not exist (`404 plugin_route_not_found`), before it reaches the
  package.
- No endpoint is served under them yet.
- In the package, every endpoint carries an edition (`v4api.Endpoints`). A
  commercial endpoint must sit under one of these prefixes, which
  `internal/tests/forwardv4` checks.
- User self-service needs a user-facing route group, because `/api/v4` is
  administrator-only. That is v4.3 work.

## Conventions

- **Bodies.** Requests and answers are JSON. Contract messages (`Route`,
  `NodeSettings`, `ForwardNodeRecord`, `PlanRouteRequest` and the answers'
  `NodeSummary`, `NodeForwardState`, `NodeForwardReport`, `Counters`,
  `UpstreamHealth`, `TrafficBucket`) are protojson from
  `sdk/api/forward/v1`:
  - field names are the proto names (`node_refs`, `expires_at_unix_ms`);
  - enums are their names (`"ENGINE_NFTABLES"`, `"LINK_SECURITY_TLS"`);
  - **64-bit integers are JSON strings** (`"revision": "3"`, byte counts,
    generations, ids of `ForwardNodeRecord`, `*_unix_ms` times), and
    32-bit ones are numbers (ports, hop indexes);
  - unset fields are left out;
  - a request with an unknown field is refused.
- **Success** is `{"data": ...}`.
- **Refusal** is:

  ```json
  {"error": {"code": "invalid_route", "message": "...", "violations": [
    {"field": "hops[1].ingress.security", "message": "...", "code": "link_unsupported", "route_id": ""}
  ]}}
  ```

  A violation's `code` is a stable code from `sdk/forward/validate`, so a
  UI acts on it without parsing the message. `route_id` names the stored
  route a violation belongs to; it is empty for the route being created.
  `node_in_use` is raised on node writes.
- **Idempotency.** Every write takes the `Idempotency-Key` header as its
  `ForwardControl` request id (1 to 128 bytes). Without one, the package
  generates an id.
  - A retry with the same key and the same request answers the recorded
    response once.
  - The same key with another request is `409 idempotency_conflict`.
  - Pause, resume and toggle derive their id from the key (`<key>:pause`).
- **Times** in queries (`since`, `until`) are Unix milliseconds. Traffic
  windows default to the 24 hours up to the end of the current hour, and are
  at most 31 days.

### Errors

| HTTP | `code` | When |
|---|---|---|
| 400 | `invalid_request` | malformed body or query, malformed settings or node fields (`INVALID_ARGUMENT` without violations) |
| 400 | `invalid_route` | the route fails validation (`INVALID_ARGUMENT` with violations) |
| 403 | `super_admin_required` | a `DELETE` by an administrator who is not a super administrator (kernel) |
| 404 | `not_found` | unknown route or node, or an unknown path |
| 404 | `plugin_route_not_found` | a commercial prefix in the community edition, or no installed package declares the route (kernel) |
| 405 | `method_not_allowed` | known path, other method |
| 409 | `refused` | the nodes cannot host the route, another route no longer plans, or a node is in use (`FAILED_PRECONDITION` with violations) |
| 409 | `idempotency_conflict` | `Idempotency-Key` reused for another request |
| 409 | `revision_conflict` | `PUT /routes/{id}` with a stale `revision`, or a concurrent change during pause or resume (`ABORTED`) |
| 429 | `rate_limited` | too many route diagnoses run at once (`RESOURCE_EXHAUSTED`) |
| 501 | `not_implemented` | `UNIMPLEMENTED` |
| 503 | `forward_unavailable` | the package has no `ForwardControl` connection, or the kernel refused or could not answer (`UNAVAILABLE`, `PERMISSION_DENIED`) |
| 504 | `timeout` | the kernel did not answer in time |

## Endpoints

All paths are under `/api/v4/forward`.

### Routes

| Method and path | Does | Body and answer |
|---|---|---|
| `GET /routes` | list routes | query `owner`, `node_ref`, `page_size` (100, at most 1000), `page_token`; `{routes: [{route, enforced}], next_page_token, can_delete}` |
| `POST /routes` | create a route | body a `Route` without id; Control assigns id, revision 1 and the times, and `owner` defaults to `admin` (`user:<id>` is refused: self-service is v4.3); `201 {route}` |
| `POST /routes/preview` | plan without storing | body a `PlanRouteRequest` (`{"route": ..., "nodes": [...]}`); `{states, allocations, violations, warnings}` |
| `GET /routes/{id}` | one route | `{route, enforced}`; `enforced` is `quota` or `expired` when Control itself pauses the route |
| `PUT /routes/{id}` | replace a route | body the whole `Route`, whose `revision` is the revision it replaces; `{route}` |
| `DELETE /routes/{id}` | delete a route (super administrator) | `{deleted}`; its hops leave every node in the next generation, and its ports stay held for 10 minutes |
| `POST /routes/{id}/pause` | pause | `{route, enforced}`; the route keeps its ports, marks, counters and quota, and the drivers drop its traffic; a paused route is answered unchanged |
| `POST /routes/{id}/resume` | resume | as pause |
| `GET /routes/{id}/stats` | traffic of one route | query `since`, `until`; `{counters, series, truncated}`: the ledger's totals per hop and node over all time (`Counters`), and the hourly buckets of the window (`TrafficBucket`) |
| `GET /routes/{id}/health` | upstream health | `{health: [UpstreamHealth]}` from the latest reports of the route's nodes |
| `POST /routes/{id}/diagnose` | diagnose the route | optional body `{"timeout_ms": n}` (15000, at most 25000); the `DiagnoseRouteResponse`: `ok`, `steps`, `nodes`, `route_id`, `started_at_unix_ms`, `finished_at_unix_ms`, `cached`. See "Route diagnosis" below |

### Route diagnosis

`POST /routes/{id}/diagnose` runs `ForwardControl.DiagnoseRoute`
(forward-sdk.md section 7.6). It checks in three stages:

1. Control's own records.
2. Probes from the route's nodes, through the `agent.diagnostic`
   operation, where the node's Agent offers them.
3. Dials from Control for what no node could probe.

The answer:

```json
{"data": {
  "ok": false, "route_id": "01J...", "started_at_unix_ms": "1759579200000", "finished_at_unix_ms": "1759579201250",
  "steps": [
    {"node_ref": "forward-11", "kind": "PROBE_KIND_CONFIG", "vantage": "DIAGNOSE_VANTAGE_CONTROL",
     "result": {"ok": true, "status": "PROBE_STATUS_OK", "message": "generation 4 applied", "observed_at_unix_ms": "..."}},
    {"node_ref": "forward-12", "hop_index": 1, "kind": "PROBE_KIND_DELIVERY", "vantage": "DIAGNOSE_VANTAGE_NODE",
     "target": "198.51.100.10:443", "protocol": "L4_PROTOCOL_TCP",
     "result": {"status": "PROBE_STATUS_FAILED", "code": "unreachable", "message": "connect: connection refused", "probe_id": "fwdiag-..."}}
  ],
  "nodes": [{"node_ref": "forward-11", "connected": true, "node_vantage": true},
            {"node_ref": "forward-12", "connected": true, "node_vantage": true}]
}}
```

- **Steps.**
  - `kind` is the stage:
    - `CONFIG`: generations, applied, hop errors, the route paused;
    - `HEALTH`: one upstream's health and breaker;
    - `LISTEN`, `PORT_CONFLICT`;
    - `TCP_CONNECT`, `UDP_EXCHANGE`: to the next hop;
    - `DELIVERY`: from the last hop to the targets.
  - `vantage` is where the step ran: Control or the node.
  - `status` is `OK`, `FAILED`, `INCONCLUSIVE` (it ran and proved nothing,
    such as a UDP probe without a reply) or `SKIPPED` (it did not run).
  - `code` is stable, for a UI to act on: `not_planned`, `never_reported`,
    `not_applied`, `apply_failed`, `hop_error`, `route_paused`,
    `route_enforced`, `healthy`, `unhealthy`, `circuit_open`, `no_health`,
    `node_offline`, `node_vantage_unavailable`, `target_not_allowed`,
    `not_public`, `deadline`, `agent_timeout`, `agent_error`, `reachable`,
    `unreachable`, `control_udp_not_probed`. The Agent adds its own codes
    for node steps, such as `no_reply`, `conflict` and `not_listening`.
- **`ok`** is true when no step `FAILED`. A step `SKIPPED` or
  `INCONCLUSIVE` does not fail a diagnosis, so read `nodes`. A node with
  `node_vantage` false ran no probes: its Agent is offline, or does not
  advertise `agent.diagnostic` and `diag.v1`. Its node steps are `SKIPPED`,
  and Control dialled the entries and the public targets in its place.
- **Public targets only.** Control never dials a private address. A last
  hop is asked to dial a private target only on an administrator's route
  with `TARGET_POLICY_ALLOW_PRIVATE`.
- **Rate.** A diagnosis of the same route from the last 10 seconds, or one
  still running, is answered again with `cached` true. At most 4
  diagnoses run at once per Control process; another is
  `429 rate_limited`.

### Nodes

A node is `forward-<id>` (a forward node, `v2_forward_node`) or `proxy-<id>`
(a proxy node, which can join the forwarding inventory).

| Method and path | Does | Body and answer |
|---|---|---|
| `GET /nodes` | the inventory | query `kind` (`forward`, `proxy`), `transport` (`agent`, `ansible`); `{nodes: [NodeSummary], can_delete}`: every forward node and the proxy nodes in the inventory, with stored settings, the planner's view (`info`, `reserved_ports`), capabilities, the desired generation and hop count, the latest report's generation, `applied` and `hop_errors` |
| `POST /nodes` | add a forward node | body `{"node": ForwardNodeRecord, "settings": NodeSettings}`, settings optional (with settings the node joins the inventory at once); `201 {node}` |
| `GET /nodes/{ref}` | the node view | `{node, state, report}`: the desired `NodeForwardState`, and the latest `NodeForwardReport` without counters (applied generation and state hash, hop errors, upstream health with latency) |
| `PUT /nodes/{ref}` | replace a forward node's fields | body the whole `ForwardNodeRecord`; an unset `transport` keeps the node's; `{node, violations}` |
| `DELETE /nodes/{ref}` | delete a forward node (super administrator) | `{deleted}`; also removes its inventory entry and revokes its Agent certificates |
| `PUT /nodes/{ref}/settings` | replace forwarding settings, on forward or proxy nodes | body `NodeSettings` (`port_range`, `reserved_ports`, `addresses` as IP addresses, `labels` such as `link=iepl`); an empty body resets to the defaults; `{node, violations}` |
| `POST /nodes/{ref}/toggle` | enable or disable a forward node | body `{"enabled": bool}`; `{node, violations}` |

- **Node writes and routes.**
  - A node that a stored route uses cannot be disabled or deleted:
    `409 refused` with one `node_in_use` violation per hop, each naming the
    route.
  - Other changes replan every route. Settings or fields that make the
    stored routes no longer plan are still stored; the answer's
    `violations` say why, and every node keeps its generation until the
    routes plan again.
- **Credentials.** A new Agent node gets the legacy credential the v2 paths
  still read until F5d, but no answer ever carries it, and an update never
  changes it. The node enrolls its Agent with an install token
  (`POST /api/v4/kernel/agents/install-tokens`, `install.sh`).

### Ansible machines

These are the forward nodes on the Ansible transport
(`NODE_TRANSPORT_ANSIBLE`: hosts without an Agent, section 6.3), addressed
by numeric id. Each route behaves as its `/nodes` counterpart, and answers
404 for a node on the Agent transport.

| Method and path | As |
|---|---|
| `GET /ansible-machines` | `GET /nodes?kind=forward&transport=ansible` |
| `POST /ansible-machines` | `POST /nodes`, with the transport forced |
| `GET /ansible-machines/{id}` | `GET /nodes/forward-{id}` |
| `PUT /ansible-machines/{id}` | `PUT /nodes/forward-{id}` |
| `DELETE /ansible-machines/{id}` | `DELETE /nodes/forward-{id}` (super administrator) |
| `POST /ansible-machines/{id}/toggle` | `POST /nodes/forward-{id}/toggle` |

### Statistics and observability

Traffic is the raw metered bytes the nodes report; no billing multiplier is
applied (section 11). The kernel keeps each node's latest health and
latency probe, not their history, so the trend is traffic.

| Method and path | Does | Answer |
|---|---|---|
| `GET /stats` | ledger totals | query `route_id`, `node_ref`, `since`, `until`; `{totals: [per route, hop and node], nodes: [per node], series: [TrafficBucket], truncated}` |
| `GET /observability/targets` | every route target with its health | query `node_ref`; `{targets: [{route_id, route_name, paused, key, host, port, weight, priority, state, healthy, reports, rtt_us, checked_at_unix_ms}], total, truncated}`; `state` is the worst one the last hop's nodes report |
| `GET /observability/topology` | the graph | `{nodes: [{id, kind, name, in_inventory, reported, applied, hop_errors, lagging}], edges: [{from, to, route_id, hop_index, engine, security, paused}], truncated}`; targets are nodes of kind `target` |
| `GET /observability/trend` | hourly traffic | query `route_id`, `node_ref`, `since`, `until`; `{points: [{hour_start_unix_ms, up_bytes, down_bytes, new_conns}], truncated}` |

The observability views read at most 2000 routes (`truncated`). A traffic
answer holds at most 20000 buckets.

## The 19 Rewritten v2 Routes

forward-sdk.md section 10 lists the 19 v2 node management routes. They map
as follows; the v2 routes stay until F5d.

| v2 route | v4 |
|---|---|
| `GET`/`POST /api/v2/admin/forward/nodes` | `GET`/`POST /nodes` |
| `GET`/`PUT`/`DELETE /api/v2/admin/forward/nodes/:id` | `GET`/`PUT`/`DELETE /nodes/forward-:id` |
| `POST .../nodes/:id/toggle` | `POST /nodes/forward-:id/toggle` |
| `POST .../nodes/:id/check` | `GET /nodes/forward-:id`. Control no longer dials the node: the Agent's report says whether it applied its desired generation, its hop errors and upstream health |
| `POST .../nodes/:id/sync-stats` | `GET /stats?node_ref=forward-:id`. Control no longer pulls gost's metrics: the nodes push counters into the ledger |
| the same eight under `/api/v2/admin/forward/ansible-machines` | `/ansible-machines` |
| `GET .../observability/targets` | `GET /observability/targets` |
| `GET .../observability/topology` | `GET /observability/topology` |
| `GET .../observability/trend` | `GET /observability/trend` (traffic rather than latency) |

## Command Line

`anix-control forward` runs on Control's database through the kernel's
forwarding state, without the package:

```text
anix-control forward routes list [--owner <owner>] [--node <node_ref>] [--json]
anix-control forward routes get <route_id>
anix-control forward routes create -f <route.json> [--request-id <id>]
anix-control forward routes delete <route_id> --yes [--request-id <id>]
anix-control forward routes pause <route_id>
anix-control forward routes resume <route_id>
anix-control forward routes diagnose <route_id> [--timeout <ms>] [--json]
anix-control forward nodes list [--kind forward|proxy] [--json]
anix-control forward nodes set <node_ref> (-f <settings.json> | [--port-range 30000-39999] [--reserved 80,443] [--address <ip>]... [--label key=value]... | --defaults)
anix-control forward stats [--route <route_id>] [--node <node_ref>] [--since <RFC 3339 or ms>] [--until ...] [--json]
anix-control forward reset-node <node_ref>
```

- **Input.** `routes create` reads a `Route` and `nodes set -f` a
  `NodeSettings`, both protojson.
- **Refusals** print each violation with its code.
- **`nodes set`** replaces the node's settings.
- **`routes diagnose`** prints the steps and the nodes, and exits non-zero
  when a step failed. The command line holds no Agent sessions, so it
  answers Control's records and Control's own dials. The node probes are
  `SKIPPED` (`node_vantage_unavailable`); the HTTP endpoint on the running
  Control runs them.
- **Audit.** Writes and diagnoses go to the audit log as `system/cli`
  (`forward.route_*` including `forward.route_diagnose`,
  `forward.node_settings`, `forward.reset_node`).
- **When nodes see a change.** The running Control sends the nodes their new
  state at its next configuration refresh, within a minute.
