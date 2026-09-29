# Incident Roadmap

This roadmap describes the remaining work after the incident platform has been documented. The documentation baseline already exists; the remaining phases focus on closing platform gaps, improving runtime alignment, and hardening the system for long-term growth.

## Phase 1 — documentation baseline

Status:
- complete, but should stay synchronized with code and config

Goal:
- make the current incident platform understandable end to end

Deliverables:
- incident architecture doc
- domain model doc
- API reference
- Cloudflare binding matrix
- operations guide
- roadmap doc

Ongoing expectation:
- keep the docs aligned with `src/types.ts`, `src/services/incidents.ts`, `src/handlers/incidents.ts`, `src/index.ts`, and `wrangler.toml`

## Phase 2 — binding and environment hardening

Goal:
- close the gap between code assumptions and configured Cloudflare services

Work items:
- reconcile the worker entrypoint in `wrangler.toml` with the actual source tree
- add or confirm missing `wrangler.toml` bindings where needed
- align `Env` typing with actual runtime services
- document which incident features are waiting on future platform primitives
- make deployment/config mismatches explicit instead of implicit

## Phase 3 — async orchestration

Goal:
- move longer incident operations off the request path

Candidates:
- queues for exports, notifications, and webhook fan-out
- workflows for multi-step approval/execution/reporting
- background job handling for report generation and heavy analytics

Design guidance:
- keep the request path fast and deterministic
- preserve idempotency across retries and task replays
- record durable progress for long-running or partially completed work

## Phase 4 — realtime collaboration

Goal:
- improve active incident coordination for responders

Candidates:
- Durable Objects for war rooms and shared incident rooms
- presence and locking semantics
- richer SSE/WebSocket event normalization
- coordinated collaboration state that can survive reconnects and handoffs

Design guidance:
- realtime should be a helper signal, not the only source of truth
- collaboration state should never block core incident mutation
- event ordering should be well-defined within a room, but not assumed globally

## Phase 5 — analytics and retrieval fidelity

Goal:
- improve the quality of search, recommendations, and reporting

Candidates:
- Vectorize-backed semantic retrieval
- Analytics Engine for aggregate trends
- stronger incident similarity / recurrence detection
- more complete SLA and response-time rollups

Design guidance:
- keep derived analytics separate from canonical workflow records
- prefer precomputed snapshots for expensive rollups
- make retrieval systems additive so they can be absent without breaking core workflows

## Phase 6 — operational hardening

Goal:
- make the platform easier to operate and safer to evolve

Candidates:
- rate limiting
- retention policies
- stricter idempotency checks
- deeper test coverage for edge cases
- more complete operator runbooks

Design guidance:
- document every destructive or irreversible workflow clearly
- prefer explicit state transitions over inferred behavior
- make operational failure modes observable and recoverable

## What already exists

The codebase already has a large portion of the incident workflow implemented in handlers, services, and routes. The roadmap above is about documenting what is present and then filling the remaining platform gaps in a controlled order.

## What still needs implementation

- future Cloudflare services not yet bound in `wrangler.toml`
- async orchestration for heavy jobs
- first-class collaborative war-room state
- richer retrieval and analytics primitives
- long-term operational hardening

## Related source files

- `docs/architecture.md`
- `docs/cloudflare-integration.md`
- `src/types.ts`
- `src/services/incidents.ts`
- `src/handlers/incidents.ts`
- `src/index.ts`
- `wrangler.toml`
