# Architecture

This repository is a Cloudflare Workers control-plane backend for AnixOps Control Center. It is intentionally broad: the same service family covers identity, users, infrastructure control, incident response, notifications, and operational reporting.

## Goals

- Keep the control plane edge-native and fast to deploy.
- Preserve a shared principal model so auth and authorization stay consistent across modules.
- Keep durable state queryable and auditable.
- Use Cloudflare primitives where they fit, but keep the domain model independent from any single transport or service.
- Describe stable boundaries in docs, not transient implementation details.

## Runtime and entrypoints

- Runtime: Cloudflare Workers
- Framework: Hono
- Compatibility: Node.js compatibility is enabled for libraries that need it
- Data plane: D1, KV, R2, and Workers AI
- Optional platform primitive: Vectorize

The source tree contains more than one worker entrypoint. When reading or documenting behavior, distinguish between:

- `src/index.ts` — the full control-plane API surface
- `src/index-with-auth.ts` — the smaller auth-focused worker used for lightweight bootstrap flows

Deployment configuration should always be validated against the actual source tree before a route family is described as live. If the deployment target is narrower than the full app, docs should call that out explicitly.

## Platform surface

The current API surface spans several stable domains:

### Identity and access

- login and logout
- registration and password changes
- token refresh and revocation
- API tokens and session listing
- MFA setup, verification, recovery codes, and admin disable flows
- principal resolution for both JWT and API-key requests

### Users and administrative control

- user profile and self-service updates
- admin user management
- user lockout and unlock flows
- audit log access
- governance policy management

### Infrastructure and operations

- nodes and node groups
- agent registration and heartbeat flows
- playbooks, tasks, schedules, and execution history
- plugins and backup operations
- Kubernetes and other infrastructure adapters where exposed
- load balancing, autoscaling, and related operational controls

### Notifications and observability

- notification records and unread counts
- dashboards and operational summary views
- audit logs
- SSE / realtime delivery
- webhooks and delivery retries
- metrics, health, readiness, liveness, and operational probes

### Incident management

- incident creation, analysis, approval, execution, and recovery
- comments, evidence, links, tags, activity, and timeline events
- runbooks, templates, response playbooks, and automation rules
- maintenance windows, bulk operations, merges, splits, recurrence, snooze, and escalation flows
- responder teams, on-call schedules, SLA calendars, response targets, and breach handling
- attachments, related items, integrations, exports, feedback, cost, and compliance
- war-room collaboration and realtime incident coordination

## Auth and authorization

Requests resolve into a shared principal model so downstream handlers can make authorization decisions consistently.

### Authentication modes

- Bearer JWT
- API key via `X-API-Key`

### Principal model

The shared principal includes:

- user identity (`sub` / user id)
- email
- role (`admin`, `operator`, or `viewer`)
- auth method (`jwt` or `api_key`)
- optional token metadata for API-key requests

### Authorization model

- `authMiddleware` establishes the principal.
- `rbacMiddleware([...])` gates sensitive routes.
- Admin-only actions should remain narrow and explicit.
- Operator access should be reserved for response workflows and operational mutation paths.
- Viewer access should remain read-only unless a route is intentionally broader.

### Auth state and revocation

The auth layer also relies on shared state for:

- JWT revocation checks
- session invalidation
- API token validation and last-used tracking

Docs should treat these as first-class control-plane concerns, not incidental implementation details.

## Data and storage model

### D1

Use D1 for canonical, relational, queryable state.

Typical D1 data includes:

- users, tokens, and session metadata
- incidents and incident history records
- nodes, schedules, tasks, and operational records
- integrations, policies, and other durable control-plane objects
- data that benefits from joins, filters, or relational constraints

### KV

Use KV for low-latency shared state and lightweight collections.

Typical KV data includes:

- token and session revocation state
- caches and lookup snapshots
- small workflow state that is read frequently
- event or index data that can tolerate eventual consistency

### R2

Use R2 for objects and other large payloads.

Typical R2 data includes:

- attachments
- exports
- generated reports
- backup archives
- downloadable bundles

### AI

Use Workers AI as a decision-support layer only.

Typical AI use cases include:

- incident summaries
- root-cause assistance
- recommendations
- similarity or retrieval assistance when the feature is wired up

AI outputs must remain advisory. They should never bypass RBAC, approval gates, or audit expectations.

### Vectorize

Vectorize is optional and currently treated as a future retrieval primitive. When it is used, it should support semantic search over runbooks, incidents, and related knowledge objects, but the docs should not imply that it is required for the current platform to function.

## Response and error conventions

### Envelope

Most handlers return a stable wrapper:

```json
{
  "success": true,
  "data": {}
}
```

Errors typically look like:

```json
{
  "success": false,
  "error": "Human-readable message"
}
```

### Status codes

- `400` — invalid input
- `401` — unauthenticated, expired, or revoked credentials
- `403` — authenticated but not authorized
- `404` — resource not found
- `409` — state conflict or duplicate transition
- `422` — workflow validation failure
- `500` — unhandled backend failure

### Client-facing behavior

- Do not assume `data` is always present.
- Do not infer success from HTTP status alone.
- Treat unknown enum values as forward-compatible.
- Ignore unknown response fields.
- Preserve route state for filters, pagination, and sort order where possible.

## Realtime and async behavior

The platform supports realtime delivery through SSE and other transport adapters.

Stable rules:

- Realtime should be treated as a hint to refresh state, not as the only source of truth.
- Event emission should not block the main request path.
- Clients should tolerate reconnects and should not assume global ordering across reconnect boundaries.
- Important mutations should remain correct even if realtime delivery is delayed or unavailable.

The docs should treat realtime, background jobs, and async orchestration as separate concerns:

- realtime is for visibility
- background jobs are for work that should not block the request path
- orchestration is for multi-step flows that need durable progress tracking

## External integration boundaries

The codebase also exposes routes for systems that should be documented as external integration surfaces rather than core domain state:

- webhooks and delivery retries
- agent registration and command execution
- backup creation, download, restore, and cleanup
- Kubernetes and other infrastructure adapters
- integrations that synchronize external incident systems or notification providers

These surfaces should remain auditable and should not become implicit sources of truth for the main control plane.

## Documentation map

Use the following docs together:

- `docs/cloudflare-integration.md` — runtime, bindings, deployment assumptions, and Cloudflare-specific gaps
- `docs/incident-architecture.md` — incident system design and stable workflow boundaries
- `docs/incident-domain-model.md` — entity groups and storage layout
- `docs/incident-api-reference.md` — endpoint contract summary
- `docs/incident-cloudflare-matrix.md` — binding/service mapping for incident capabilities
- `docs/incident-operations.md` — operator workflow and verification guidance
- `docs/incident-roadmap.md` — future work phases and platform gaps
- `docs/client-baseline.md` — client-facing behavior baseline for future frontend or machine client work
