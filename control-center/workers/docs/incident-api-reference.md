# Incident API Reference

This document summarizes the incident API surface. Paths are grouped by capability family.

Note: route availability can vary by deployment target. Use `docs/cloudflare-integration.md` alongside this reference if a capability is missing in a specific environment.

## Conventions

- `authMiddleware` is required unless noted otherwise.
- `rbacMiddleware(['admin'])` indicates admin-only mutation access.
- `rbacMiddleware(['admin', 'operator'])` indicates operator/admin access.
- Response objects generally use `{ success: boolean, data?: T, error?: string }`.

## Core incident endpoints

### `GET /api/v1/incidents`
- Auth: required
- RBAC: read access via general auth
- Purpose: list incidents with status, severity, source, correlation, and pagination filters
- Notes: supports sorting and paging
- Related code: `src/index.ts` incident list route

### `POST /api/v1/incidents`
- Auth: required
- RBAC: general create access
- Purpose: create a new incident from operational context
- Request: title, source, summary, severity, correlation_id, action_type, action_ref, tags, evidence
- Response: created incident record
- Related code: `src/index.ts` incident creation route

### `GET /api/v1/incidents/:id`
- Auth: required
- RBAC: read access
- Purpose: fetch one incident and full detail

### `GET /api/v1/incidents/statistics`
- Auth: required
- RBAC: read access
- Purpose: operational statistics and aggregate counts

### `GET /api/v1/incidents/report`
- Auth: required
- RBAC: admin/operator
- Purpose: generate report view

### `GET /api/v1/incidents/search`
- Auth: required
- RBAC: read access
- Purpose: text and metadata search

## Lifecycle actions

### `POST /api/v1/incidents/:id/analyze`
- Auth: required
- RBAC: admin/operator
- Purpose: run AI-assisted analysis
- Related code: `src/index.ts` incident analysis route

### `POST /api/v1/incidents/:id/approve`
- Auth: required
- RBAC: admin/operator
- Purpose: approve controlled remediation
- Idempotency: should not double-approve
- Related code: `src/index.ts` incident approval route

### `POST /api/v1/incidents/:id/execute`
- Auth: required
- RBAC: admin/operator
- Purpose: execute approved action
- Idempotency: should not double-run the same action
- Related code: `src/index.ts` incident execution route

### `POST /api/v1/incidents/:id/acknowledge`
- Auth: required
- RBAC: admin/operator

### `POST /api/v1/incidents/:id/escalate`
- Auth: required
- RBAC: admin/operator

### `POST /api/v1/incidents/:id/assign`
- Auth: required
- RBAC: admin/operator

### `DELETE /api/v1/incidents/:id/assign`
- Auth: required
- RBAC: admin/operator

## Comments, evidence, tags, links, and activity

### `GET /api/v1/incidents/:id/comments`
### `POST /api/v1/incidents/:id/comments`
### `PUT /api/v1/incidents/:id/comments/:commentId`
### `DELETE /api/v1/incidents/:id/comments/:commentId`

### `POST /api/v1/incidents/:id/evidence`
- Adds evidence to the incident

### `POST /api/v1/incidents/:id/links`
### `DELETE /api/v1/incidents/:id/links/:linkKind/:linkId`

### `GET /api/v1/incidents/:id/activity`
- Returns incident activity log
- Related code: `src/index.ts` activity route

### `GET /api/v1/incidents/tags`
### `POST /api/v1/incidents/:id/tags`
### `DELETE /api/v1/incidents/:id/tags`
### `PUT /api/v1/incidents/:id/tags`

## Dashboards, statistics, reports, and exports

### `GET /api/v1/incidents/dashboard/metrics`
- Operator/admin dashboard metrics

### `POST /api/v1/incidents/:id/runbooks/execute`
- Execute a runbook for a specific incident

### `GET /api/v1/incidents/:id/runs`
- Run history

### `POST /api/v1/incidents/export`
- Export incidents
- Download and status endpoints exist in the codebase via export handlers

## Playbooks, templates, automation, and post-mortems

### Incident templates
- `GET /api/v1/incidents/templates`
- `GET /api/v1/incidents/templates/:templateId`
- `POST /api/v1/incidents/templates`
- `PUT /api/v1/incidents/templates/:templateId`
- `DELETE /api/v1/incidents/templates/:templateId`
- `POST /api/v1/incidents/templates/:templateId/create`

### Automation rules
- `GET /api/v1/incidents/automation-rules`
- `POST /api/v1/incidents/automation-rules`
- `DELETE /api/v1/incidents/automation-rules/:ruleId`
- `PATCH /api/v1/incidents/automation-rules/:ruleId`

### Post-mortems
- `GET /api/v1/incidents/:id/postmortem`
- `POST /api/v1/incidents/:id/postmortem`
- `PUT /api/v1/incidents/:id/postmortem`
- `PATCH /api/v1/incidents/:id/postmortem/action-items/:actionItemId`

## Teams, schedules, SLAs, notifications, escalation, attachments

### Responder teams
- `GET /api/v1/incidents/teams`
- `GET /api/v1/incidents/teams/:teamId`
- `POST /api/v1/incidents/teams`
- `PUT /api/v1/incidents/teams/:teamId`
- `DELETE /api/v1/incidents/teams/:teamId`

### On-call schedules
- `GET /api/v1/incidents/oncall/schedules`
- `GET /api/v1/incidents/oncall/schedules/:scheduleId`
- `GET /api/v1/incidents/oncall/schedules/:scheduleId/current`
- `POST /api/v1/incidents/oncall/schedules`

### SLA calendars and targets
- `GET /api/v1/incidents/sla-calendars`
- `GET /api/v1/incidents/sla-calendars/:calendarId`
- `POST /api/v1/incidents/sla-calendars`
- `GET /api/v1/incidents/response-targets`
- `POST /api/v1/incidents/response-targets`
- `GET /api/v1/incidents/sla`

### Notification templates and rules
- `GET /api/v1/incidents/notification-templates`
- `GET /api/v1/incidents/notification-templates/:templateId`
- `POST /api/v1/incidents/notification-templates`
- incident notification rule endpoints in the route table remain available for general incident notification governance

### Escalation rules
- `GET /api/v1/incidents/escalation-rules`
- `POST /api/v1/incidents/escalation-rules`

### Attachments
- `GET /api/v1/incidents/:id/attachments`
- `POST /api/v1/incidents/:id/attachments`
- `GET /api/v1/incidents/:id/attachments/:attachmentId`
- `DELETE /api/v1/incidents/:id/attachments/:attachmentId`

## Maintenance windows, bulk operations, merges/splits, recurrence

### Maintenance windows
- `GET /api/v1/incidents/maintenance-windows`
- `GET /api/v1/incidents/maintenance-windows/:windowId`
- `POST /api/v1/incidents/maintenance-windows`
- `PATCH /api/v1/incidents/maintenance-windows/:windowId`
- `POST /api/v1/incidents/maintenance-windows/:windowId/cancel`
- Related code: `src/index.ts` maintenance window routes

### Bulk operations
- `GET /api/v1/incidents/bulk-operations`
- `GET /api/v1/incidents/bulk-operations/:operationId`
- `POST /api/v1/incidents/bulk-operations`
- `POST /api/v1/incidents/bulk-operations/:operationId/execute`

### Merge / split / recurrence
- `GET /api/v1/incidents/merges`
- `POST /api/v1/incidents/:id/merge`
- `GET /api/v1/incidents/splits`
- `POST /api/v1/incidents/:id/split`
- `GET /api/v1/incidents/recurrences`
- `POST /api/v1/incidents/:id/detect-recurrence`
- `POST /api/v1/incidents/recurrences/:recurrenceId/resolve`

## Webhooks and integrations

### Incident webhooks
- `GET /api/v1/incidents/webhooks`
- `GET /api/v1/incidents/webhooks/:subscriptionId`
- `POST /api/v1/incidents/webhooks`
- `PATCH /api/v1/incidents/webhooks/:subscriptionId`
- `DELETE /api/v1/incidents/webhooks/:subscriptionId`

### Integrations
- `GET /api/v1/incidents/integrations`
- `GET /api/v1/incidents/integrations/:integrationId`
- `POST /api/v1/incidents/integrations`
- `PATCH /api/v1/incidents/integrations/:integrationId`
- `DELETE /api/v1/incidents/integrations/:integrationId`

## Error model

Across the incident API, common failures include:
- 400 for invalid payloads
- 401 for unauthenticated requests
- 403 for RBAC denial
- 404 for missing resources
- 409 for state conflicts or duplicate transitions
- 422 for invalid workflow state
- 500 for unexpected persistence or runtime failures

## Source files

- `src/handlers/incidents.ts`
- `src/services/incidents.ts`
- `src/index.ts`
