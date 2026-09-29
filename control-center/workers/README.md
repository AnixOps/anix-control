# AnixOps Control Center Workers

Cloudflare Workers API backend for AnixOps Control Center.

This directory is `control-center/workers/` in
[`AnixOps/anix-control`](https://github.com/AnixOps/anix-control). It was imported
from the archived `AnixOps/Anixops-control-center-worker` repository and trimmed to
the routes the Control Center clients use.

## What this directory contains
- Cloudflare Workers API
- D1 migrations (already applied to production; append only)
- Workers-specific release notes

CI runs from `.github/workflows/control-center-workers.yml` at the repository root.

## Local development

```bash
npm install
npm run dev
```

## API base

`https://api.anixops.com/api/v1`

## API surface

Routes are registered in `src/app/register-*.ts` and wired by `src/index.ts`.

- Platform probes (outside `/api/v1`): `/health`, `/health/detailed`, `/readiness`, `/liveness`, `/metrics`
- Auth: `/auth/login`, `/auth/register`, `/auth/refresh`, `/auth/logout`, `/auth/password`, MFA (`/mfa/*`, `/admin/users/:id/mfa/disable`)
- Users: `/users` (admin), `/users/me` (profile, API tokens, sessions), lockout/unlock
- Infrastructure: `/nodes` (incl. bulk, bulk-status, stats, logs, test, sync, install-script), `/node-groups`, `/ssh`, `/agents`
- Automation: `/playbooks`, `/tasks`, `/schedules`, `/plugins`, `/batch`
- Operations: `/dashboard`, `/notifications`, `/audit-logs`, `/logs`, `/backups`
- Realtime: `/sse`, `/ws`

A tail worker (`src/tail.ts`, `wrangler.tail.toml`) is deployed separately with `npm run deploy:tail`.
