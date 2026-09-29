# Incident Cloudflare Matrix

This matrix maps incident capabilities to Cloudflare primitives that are already present or should be added later. It is a planning and validation aid, not a claim that every capability is fully deployed today.

## Service matrix

| Service / binding | Current status | Incident use cases | Missing config in `wrangler.toml` | Planned follow-up work |
| --- | --- | --- | --- | --- |
| D1 (`DB`) | Present | Canonical incident records, comments, workflow rows, integrations, audit-friendly relational data | None | Expand schema and migrations only when a new structured record is needed. |
| KV (`KV`) | Present | Incident indexes, workflow snapshots, rule collections, timeline caches, revocation state, analytics snapshots | None | Keep high-churn or cache-like data here; partition or TTL large collections if growth demands it. |
| R2 (`R2`) | Present | Attachments, exports, downloadable report bundles, backups | None | Standardize object keys, retention rules, and download lifecycle. |
| AI (`AI`) | Present | Incident summaries, likely cause, recommendations, response briefs | None | Add stricter output schemas, prompt/version tracking, and deterministic fallbacks. |
| Vectorize (`VECTORIZE`) | Referenced in docs, not bound in current `wrangler.toml` | Semantic retrieval for runbooks, incidents, historical evidence, similar incidents | Yes | Add the binding only when retrieval/search becomes a production path. |
| Durable Objects | Not configured | War-room coordination, locks, shared incident room state, presence, leader election | Yes | Introduce DO namespaces only if collaboration needs strong coordination semantics. |
| Queues | Not configured | Async notifications, report generation, export jobs, webhook fan-out | Yes | Offload non-blocking work from the request path when work volume justifies it. |
| Workflows | Not configured | Multi-step approval/execution pipelines, scheduled reporting, recurring jobs | Yes | Model long-running orchestration with durable progress tracking. |
| Analytics Engine | Not configured | Trend aggregation, response-time telemetry, SLA breach rollups | Yes | Move heavy aggregate computation out of request-time code paths. |
| Rate Limiter | Not configured | Abuse control, noisy-client protection, webhook/API throttling | Yes | Protect sensitive endpoints and external integrations once traffic or abuse patterns require it. |

## Incident capability map

### Operational control plane
- incidents
- assignments
- acknowledgements
- approvals
- execution
- escalation

### Response assistance
- AI summaries
- runbooks and templates
- response targets
- playbook execution

### History and compliance
- timeline events
- activity logs
- audit trails
- reviews
- compliance records

### Storage split guidance
- durable state in D1
- high-churn caches and small collections in KV
- files and downloads in R2
- future collaborative/session state in Durable Objects
- future asynchronous work in Queues and Workflows

## Current `wrangler.toml` gap summary

The active config currently includes:
- D1
- KV
- R2
- AI

The incident roadmap should treat the following as future additions:
- Vectorize binding
- Durable Objects
- Queues
- Workflows
- Analytics Engine
- Rate Limiter

Also note the deployment-entrypoint mismatch in the current repo state: the worker entrypoint referenced by `wrangler.toml` should be reconciled with the actual source tree before any doc claims the full route surface is live.

## Related source files

- `wrangler.toml`
- `src/types.ts`
- `src/services/incidents.ts`
- `src/handlers/incidents.ts`
- `src/index.ts`
