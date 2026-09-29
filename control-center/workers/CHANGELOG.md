# Changelog

## Unreleased
- Trimmed the worker to the core control-center routes (platform probes, auth/MFA, users, nodes, node-groups, playbooks, tasks, schedules, notifications, dashboard, audit-logs, ssh, plugins, agents, logs, backups, batch, SSE, WebSocket) plus the tail worker.
- Removed route groups and their handlers, services, types, tests, and docs: incidents, governance, webhooks, kubernetes, lb (load balancing), mesh (Istio), scaling (autoscaling), ai, vectors (Vectorize), web3, ipfs, and `/internal` developer mode.
- Removed unused services (discovery, elk, resilience, tracing), the tenant middleware/handlers/service, and the unused `src/index-with-auth.ts` entrypoint.
- `src/index.ts` now only wires `createApp()` plus the `notFound`/`onError` handlers; it previously re-registered every protected route a second time.
- `GET /api/v1/dashboard` no longer includes `developer_readiness_summary` (it came from the removed developer-mode readiness manifest).
- SSE/WebSocket no longer accept `incident:<id>` channel subscriptions.
- Removed the `AI` binding from `wrangler.toml`.
- Removed the generated endpoint visualizer report (`endpoint-visualizer-report.html`/`.json`) and the `test:visualize` / `test:devmode` scripts.
- D1 migrations are unchanged; tables used only by removed features (for example `incidents`) remain in place.

## 1.0.0
- Initial independent Workers repo bootstrap.
