# Cloudflare Integration

This repository uses Cloudflare as the runtime, storage, and edge control-plane layer for AnixOps Control Center. Cloudflare is not just the hosting platform here; it is part of the system design.

## Current deployment facts

The current repo state shows two important realities:

- `src/index.ts` contains the full control-plane API surface.
- `src/index-with-auth.ts` contains a smaller auth/bootstrap worker.
- `wrangler.toml` now points `main` at `src/index.ts`, which matches the primary source entrypoint.

Because of that alignment, the docs should still distinguish carefully between:

- what the source tree implements
- what the deployment configuration actually serves
- what is planned but not yet wired into the deployed runtime

## Runtime and platform primitives

- Runtime: Cloudflare Workers
- Framework: Hono
- Compatibility date: `2024-01-01`
- Compatibility flags: `nodejs_compat`
- Route host: `api.anixops.com`
- API namespace: `/api/v1`

## Current bindings

| Binding | Status | Primary use | Notes |
| --- | --- | --- | --- |
| `DB` | Present | Canonical relational data | D1 stores durable control-plane records and queryable workflow state. |
| `KV` | Present | Shared state, revocation data, caches | Used for token/session revocation and other lightweight fast-path state. |
| `R2` | Present | Attachments, exports, backups, bundles | Stores large or binary artifacts that should not live in D1. |
| `AI` | Present | Analysis and summarization | Advisory only; does not replace policy or RBAC. |
| `ANALYTICS` | Present | Scrape/event telemetry | Used for lightweight edge analytics and operational visibility. |
| `VECTORIZE` | Optional / not yet bound | Semantic retrieval | Referenced in the docs as a future retrieval primitive, not a required dependency. |

## What Cloudflare is used for today

The existing platform already uses Cloudflare for:

- request handling at the edge
- auth and session enforcement
- durable and queryable application state
- object storage for files and generated artifacts
- AI-assisted analysis paths
- runtime metrics and event telemetry
- realtime transport adapters and event delivery
- future-ready integration points for vector search and async orchestration

## Public endpoints outside the versioned API

Not every public endpoint lives under `/api/v1`.

Current public surface includes:

- `/health`
- `/health/detailed`
- `/readiness`
- `/liveness`
- `/metrics`

The versioned API remains the main product surface, but operational probes should be treated as first-class runtime behavior.

## Deployment and routing notes

- `[[routes]]` currently targets the custom domain `api.anixops.com`.
- `workers_dev = true` is enabled, so local/preview deployment behavior still matters.
- Auth headers and CORS settings are enforced in application code, not just in Wrangler config.
- The deployment path should be validated whenever the codebase adds or renames worker entrypoints.

If the deployed worker is narrowed intentionally, document that explicitly so clients do not assume the full source tree is live.

## Incident-specific docs

For incident platform design and client behavior, use:

- `docs/incident-architecture.md`
- `docs/incident-domain-model.md`
- `docs/incident-api-reference.md`
- `docs/incident-cloudflare-matrix.md`
- `docs/incident-operations.md`
- `docs/incident-roadmap.md`
- `docs/client-baseline.md`

## Binding gaps to track

Treat the following as future or optional additions unless the code and `wrangler.toml` are updated together:

- Vectorize binding
- Durable Objects
- Queues
- Workflows
- Rate Limiter

## Operational guidance

- D1 should remain the source of truth for durable relational records.
- KV should remain fast and lightweight; do not rely on it as the only durable store for critical business records.
- R2 should own binary files and generated artifacts.
- AI should stay advisory.
- Unknown or missing bindings should be documented as gaps, not silently assumed.
