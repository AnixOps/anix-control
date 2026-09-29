# Cloudflare Integration

This repository uses Cloudflare as the runtime, storage, and edge control-plane layer for AnixOps Control Center. Cloudflare is not just the hosting platform here; it is part of the system design.

## Current deployment facts

- `src/index.ts` is the single API entrypoint. It wires `createApp()` (routes live in `src/app/register-*.ts`) plus the `notFound` / `onError` handlers.
- `npm run build` bundles `src/index.ts` into `dist/index.mjs`; `wrangler.toml` sets `main = "dist/index.mjs"` and deploys with `--no-bundle`.
- `src/tail.ts` is a separate tail worker deployed via `wrangler.tail.toml` and attached through `[[tail_consumers]]`.

The docs should still distinguish carefully between:

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
| `R2` | Present | Playbook files, backups, bundles | Stores large or binary artifacts that should not live in D1. |
| `ANALYTICS` | Present (optional in code) | Scrape/event telemetry | `/metrics` writes a data point when the binding exists. |

The `AI` (Workers AI) binding was removed together with the AI, vector-search, and incident features; no remaining code references it.

## What Cloudflare is used for today

The existing platform already uses Cloudflare for:

- request handling at the edge
- auth and session enforcement
- durable and queryable application state
- object storage for playbooks and backup archives
- runtime metrics and event telemetry
- realtime delivery over SSE and WebSocket

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

## D1 migrations

`migrations/0001`–`0005` are already applied to the production `anixops-db` database and must not be edited or removed. Some of their tables are no longer read or written by this worker after the trim — `incidents` (`0005`), `webhooks` (`0002`), and the tenant/role tables from `0004` (the `tenant_id` columns `0004` added are still used by audit logging) — and are intentionally left in place.

## Binding gaps to track

Treat the following as future or optional additions unless the code and `wrangler.toml` are updated together:

- Durable Objects
- Queues
- Workflows
- Rate Limiter

## Operational guidance

- D1 should remain the source of truth for durable relational records.
- KV should remain fast and lightweight; do not rely on it as the only durable store for critical business records.
- R2 should own binary files and generated artifacts.
- Unknown or missing bindings should be documented as gaps, not silently assumed.
