# Client API Baseline

This document is the client-facing reference for future frontend or machine client updates. It defines the stable expectations for authentication, request/response shapes, lifecycle flows, realtime updates, and retry behavior.

## Scope

This baseline covers the parts of the API that clients are expected to consume directly:
- auth and session handling
- incident workflows
- nodes, playbooks, tasks, schedules
- notifications, dashboard data, and audit visibility
- SSE / WebSocket / other realtime delivery adapters

For deeper implementation and platform context, see:
- `docs/architecture.md`
- `docs/cloudflare-integration.md`
- `docs/incident-api-reference.md`
- `docs/incident-architecture.md`
- `docs/incident-domain-model.md`
- `docs/incident-operations.md`

## Client roles

Clients should assume one of these authenticated contexts:
- **admin** — full control-plane access, including configuration and destructive operations
- **operator** — operational read/write access for response workflows
- **viewer** — read-only access when the server exposes it
- **API key client** — machine access with the same principal model as JWT clients

The client should never infer permissions from the UI alone. Every action must be treated as server-authorized.

## Base URL

Current production versioned API base:

`https://api.anixops.com/api/v1`

Operational probes such as `/health` and `/metrics` may live outside the versioned namespace and should not be treated as client workflow endpoints.

## Authentication expectations

### JWT login flow

Typical client flow:
1. login with credentials
2. store the returned access token securely
3. attach the token to subsequent requests
4. refresh before expiry when refresh tokens are available
5. sign out and clear local state on logout

### API key flow

Machine clients should:
- send the API key using the server-accepted header format
- treat the key as a secret and never persist it in logs
- assume the same RBAC boundaries as JWT users, but with the permissions attached to the key owner

### Session behavior

Clients should handle:
- expired credentials
- revoked sessions
- disabled users
- authorization failures that require re-authentication

## Deployment awareness

The baseline documents the stable client contract, but route availability can still vary by deployment target. Clients should treat the documented versioned API as the main contract and consult the platform docs if a capability is unavailable in a specific environment.

## Standard response envelope

Most endpoints return a consistent wrapper:

```json
{
  "success": true,
  "data": {}
}
```

Failure responses usually look like:

```json
{
  "success": false,
  "error": "Human-readable message"
}
```

Client expectations:
- do not assume `data` is always present
- do not assume a successful HTTP status if `success` is false
- surface server error text when possible, but keep client-side messages user friendly

## Error handling contract

Clients should map the common HTTP status classes as follows:

- **400** — invalid or missing input
- **401** — unauthenticated or expired auth
- **403** — authenticated but not authorized
- **404** — resource not found
- **409** — state conflict or duplicate transition
- **422** — workflow state or semantic validation failure
- **500** — unhandled backend failure

Recommended UI behavior:
- 400/422: show inline validation feedback
- 401: redirect to login or refresh session
- 403: show an authorization/role message
- 404: show a safe “not found” state
- 409: allow retry only if the UI can re-read fresh state
- 500: show a retryable system error

## Pagination and filtering conventions

Clients should expect common list endpoints to support:
- `page`
- `per_page`
- `sort`
- `order`
- domain-specific filters such as status, severity, source, and tags

Client rules:
- always preserve the active filter state in the URL or route state when possible
- request the next page only when the user scrolls or explicitly paginates
- do not assume sort order unless requested

## Incident client baseline

Incidents are the primary client workflow.

### Incident list

The client should be able to:
- browse incidents
- filter by status, severity, source, and correlation id
- sort by created or updated timestamps
- distinguish open, analyzed, approved, executing, resolved, and failed states

### Incident detail

The detail screen should present:
- core incident metadata
- evidence and linked resources
- comments and activity
- current status and severity
- AI analysis and recommendations
- execution / approval history
- SLA status and related operational objects

### Incident lifecycle actions

Client action order should generally be:
1. create or open an incident
2. review evidence and history
3. request analysis
4. assign or acknowledge
5. approve if required
6. execute remediation
7. monitor result
8. resolve / close / merge / split if the server state requires it

### Retry safety

Clients must treat the following as potentially repeated requests and guard accordingly:
- analyze
- approve
- execute
- bulk operations
- create comment / attachment / webhook configuration where duplication matters

If the UI retries a request because of a transient failure, it should refresh the resource before re-submitting when possible.

## Realtime expectations

The platform supports realtime delivery through SSE and WebSocket-style transports.

Clients should:
- subscribe once per active session or page
- treat realtime as a hint to refresh state, not as the only source of truth
- use the incident id and event type to update only the necessary UI region
- reconnect automatically after temporary connection loss

Recommended event categories:
- incident created
- incident analyzed
- incident approved
- incident executing
- incident resolved
- incident failed
- comments, evidence, and status changes where exposed by the transport layer

Realtime rules:
- the client should remain functional if realtime is unavailable
- the client should always reconcile with a fresh API read after important events
- event ordering should not be assumed across reconnects unless the UI has explicitly buffered them

## Cross-resource client surfaces

### Nodes
Clients should be able to:
- list nodes
- inspect node detail
- start, stop, restart, sync, and test node connections where authorized

### Playbooks and tasks
Clients should be able to:
- browse playbooks
- upload or sync playbooks where authorized
- create and inspect tasks
- review task logs and retry or cancel task execution

### Schedules
Clients should be able to:
- list schedules
- create and edit schedules where authorized
- toggle and run schedules manually

### Notifications and dashboard
Clients should treat notifications and dashboard summaries as secondary surfaces that help operators prioritize work, not as the authoritative source for workflow state.

## Data-model expectations for clients

Clients should rely on stable identifiers and not on display names.

Prefer:
- incident ids
- node ids
- task ids
- schedule ids
- template ids
- team ids
- attachment ids

Avoid:
- using names as database keys
- assuming labels remain unchanged
- deriving permissions from visual labels

## Upgrade expectations

When future client updates are made, the client should assume:
- endpoints may be expanded, but existing response envelopes should stay stable
- new fields may appear without breaking older clients
- new workflow states should be treated as forward-compatible if the client does not recognize them
- server-side permission rules may become stricter over time

Clients should therefore:
- ignore unknown fields
- handle unknown enum values gracefully
- avoid hard-coding response shapes beyond the documented stable fields

## Recommended client implementation checklist

Before shipping a client update, verify:
- auth refresh and logout work correctly
- incident list/detail screens load without errors
- lifecycle actions update state correctly
- realtime reconnects do not duplicate messages
- errors display cleanly for 401/403/404/409/422/500
- pagination and filtering survive route changes
- idempotent actions do not create duplicate side effects

## Related documentation

- `docs/api-contract.md`
- `docs/incident-api-reference.md`
- `docs/incident-architecture.md`
- `docs/incident-operations.md`
