# Incident Architecture

## Purpose

The incident platform is the operational control plane for detecting, tracking, analyzing, approving, and executing incident response actions. It is designed for both human operators and machine clients.

The system is intended to:
- capture incidents from operational signals or manual creation
- preserve a durable record of evidence, decisions, and actions
- support AI-assisted triage without letting AI bypass governance
- route remediation through explicit approval and execution steps
- emit audit and realtime events for every important transition
- support post-incident operations such as review, recurrence tracking, and reporting

## Core workflow

A typical incident path is:
1. create incident
2. collect evidence and tags
3. analyze / summarize
4. assign or acknowledge
5. approve remediation if required
6. execute the chosen action
7. resolve, fail, merge, or split as needed
8. retain the full history for audit and reporting

## Incident lifecycle

The current codebase uses a lifecycle centered on states such as:
- `open`
- `analyzed`
- `approved`
- `executing`
- `resolved`
- `failed`

Supporting transitions include:
- acknowledge
- assign / unassign
- escalate
- comment / evidence add
- merge / split
- recurrence detection
- snooze and wake
- maintenance suppression
- correlation and watch flows

## Major feature families

### 1. Core incident operations
- CRUD
- filtering and search
- summary/detail rendering
- timeline and activity
- comments, evidence, links, and tags

### 2. Decision support
- AI analysis
- recommendations
- incident statistics and dashboard metrics
- response analytics and reporting

### 3. Governance and control
- authorization through shared principals
- RBAC for operator/admin flows
- approval gating
- audit logs for each state change

### 4. Response orchestration
- runbooks and response playbooks
- auto-remediation rules
- bulk operations
- maintenance windows
- merges, splits, and recurrence tracking

### 5. Operational extensions
- SLA calendars and breach tracking
- responder teams and on-call schedules
- notification templates and webhooks
- attachments, related items, and integrations
- compliance, cost, and feedback tracking

## Realtime model

Incident changes should emit normalized lifecycle events that can be delivered over SSE, WebSocket, or other transport adapters. The transport should stay decoupled from the underlying event format.

Important characteristics:
- every significant transition should generate an event
- events should carry a stable incident identifier and correlation data
- transports should subscribe to the same canonical event payload
- event emission should not block the main workflow path

## Storage responsibilities

### D1
Use D1 for durable relational state, especially where queryability matters:
- canonical incidents
- users and authenticated entities when relevant
- reportable histories and structured records
- anything that benefits from indexed filtering or joins

### KV
Use KV for lightweight workflow collections and shared state:
- incident index/cache
- timeline/event snapshots when not normalized elsewhere
- runbook or rule collections that are read more often than they change
- analytics snapshots and operational lookups

### R2
Use R2 for binary or large payloads:
- attachments
- exports
- generated reports
- downloadable archives

### AI
Use AI only as a decision-support layer:
- summaries
- likely cause
- recommended actions
- triage assistance

AI output must remain advisory and never bypass approvals or RBAC.

## Auth and RBAC model

The incident platform should treat every request as one of:
- authenticated human operator
- authenticated admin
- authenticated API key client
- future service identity

The handlers should continue to enforce:
- admin-only mutations for sensitive configuration
- operator/admin access for response workflows
- read access where appropriate for viewers and machine clients

## Idempotency expectations

Incident workflows should be safe for retries. In particular:
- approve should not double-approve
- execute should not double-run an action
- bulk operations should record individual item results
- attachment and external-write paths should avoid duplicate side effects

## Current gaps to document clearly

This repository already has broad incident functionality, but the documentation should call out the current platform gaps and next-step dependencies:
- no durable object-backed collaboration room yet
- no queue-backed async pipeline for heavy background work
- no workflow orchestration primitive configured in `wrangler.toml`
- no formal analytics engine binding for long-term trend aggregation
- vector retrieval is referenced but not yet fully documented as a production path
- some long-running jobs are still handled synchronously in service code
- deployment entrypoints and runtime config should always be checked before assuming every source-tree route is live

## Related source files

- `src/types.ts`
- `src/services/incidents.ts`
- `src/handlers/incidents.ts`
- `src/index.ts`
- `wrangler.toml`
