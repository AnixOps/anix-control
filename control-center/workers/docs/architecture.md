# Architecture

This repository is a Cloudflare Workers control-plane backend for AnixOps Control Center. It covers the core control-center surface: identity, users, infrastructure control (nodes, playbooks, tasks, schedules), notifications, realtime delivery, and operational reporting.

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
- Data plane: D1, KV, R2, plus Analytics Engine for scrape telemetry

Entrypoints:

- `src/index.ts` — the API worker. It builds the app via `createApp()` (`src/app/create-app.ts`) and adds the `notFound` / `onError` handlers. `npm run build` bundles it to `dist/index.mjs`, which is what `wrangler.toml` deploys.
- `src/app/register-*.ts` — the only place routes are registered (platform probes, auth, protected core routes, protected system routes).
- `src/tail.ts` — the tail worker (`wrangler.tail.toml`).

Deployment configuration should always be validated against the actual source tree before a route family is described as live.

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

### Infrastructure and operations

- nodes and node groups
- agent registration and heartbeat flows
- playbooks, tasks, schedules, and execution history
- plugins and backup operations
- SSH connection testing and server import
- batch operations and bulk node status

### Notifications and observability

- notification records and unread counts
- dashboards and operational summary views
- audit logs
- log search and ingestion (`/api/v1/logs`)
- SSE and WebSocket realtime delivery
- metrics, health, readiness, liveness, and operational probes

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
- Operator access should be reserved for operational mutation paths.
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
- nodes, node groups, schedules, tasks, and operational records
- notifications, audit logs, and other durable control-plane objects
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

- playbook files
- backup archives
- downloadable bundles

### Analytics Engine

`ANALYTICS` (optional) receives a lightweight data point on each `/metrics` scrape. The worker runs fine without it.

### Database migrations

`migrations/` is append-only: `0001`–`0005` are already applied to the production D1 database. Tables for features that were trimmed from this worker (for example `incidents` from `0005` and `webhooks` from `0002`) are intentionally left in place.

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

The platform supports realtime delivery through SSE (`/api/v1/sse`) and WebSocket (`/api/v1/ws`).

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

- agent registration and command execution
- backup creation, download, restore, and cleanup
- SSH-based server import and connection testing

These surfaces should remain auditable and should not become implicit sources of truth for the main control plane.

## Documentation map

Use the following docs together:

- `docs/api-contract.md` — key compatibility points of the public API
- `docs/cloudflare-integration.md` — runtime, bindings, deployment assumptions, and Cloudflare-specific gaps
- `docs/client-baseline.md` — client-facing behavior baseline for future frontend or machine client work
- `docs/deploy.md` — deployment steps
