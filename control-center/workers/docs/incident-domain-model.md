# Incident Domain Model

This document describes the incident entities implemented in the codebase and where each group should live.

## Core incident record

### Incident
Purpose: the canonical workflow object for operational incidents.

Typical fields:
- identifiers and titles
- severity, status, source, and correlation data
- requested/approved/assigned actor metadata
- evidence, recommendations, links, analysis, execution result
- tags and timestamps

Owner subsystem:
- service logic in `src/services/incidents.ts`
- HTTP validation and contract shaping in `src/handlers/incidents.ts`

Storage:
- D1 for canonical durable state
- KV for lookup/index support where needed

### Incident summary and detail views
Purpose: present operator-friendly and machine-friendly representations of the same incident.

Owner subsystem:
- `src/types.ts`
- `src/services/incidents.ts`

Storage:
- derived from canonical incident state

## Activity and audit-related entities

### Timeline events
Purpose: append-only lifecycle and operational history.

Stored in:
- incident-scoped collections
- realtime event feeds
- audit logs for action provenance

### Activity log
Purpose: queryable operational history with actor and metadata context.

### Comments
Purpose: human discussion and coordination.

### Watches / subscriptions
Purpose: user preferences and notification subscriptions for incident lifecycle changes.

### Links and tags
Purpose: connect incidents to external resources and classify them for search, dashboards, and automation.

## Response orchestration entities

### Playbooks and response playbooks
Purpose: structured response plans and step-by-step execution.

### Templates
Purpose: incident instantiation shortcuts and standardized incident creation.

### Automation rules and notification rules
Purpose: event-driven actions and notifications based on incident state or metadata.

### Auto-remediation rules
Purpose: direct operational actions when conditions are met.

### Run history and execution records
Purpose: preserve the outcome of each automated or manual response path.

## Response team and scheduling entities

### Responder teams
Purpose: define ownership groups, service responsibilities, and escalation paths.

### On-call schedules
Purpose: determine current responder assignment and routing.

### SLA calendars
Purpose: business-hour-aware timing calculations and breach detection.

### Response targets
Purpose: define acknowledge/assign/resolve/first-response targets by severity.

## Operational control entities

### Maintenance windows
Purpose: suppress or slow incident creation/alerts during planned work.

### Bulk operations
Purpose: batch status, severity, assignment, tagging, close, or escalation actions.

### Merges and splits
Purpose: manage incident consolidation and decomposition when root causes or scopes change.

### Recurrence records
Purpose: track repeat incidents, detect patterns, and connect related outages.

### SLA breaches
Purpose: record target misses and follow-up actions.

## External and attachment entities

### Attachments
Purpose: store uploaded files and supporting material.

Storage:
- R2 for blobs
- metadata in D1/KV depending on query patterns

### Related items
Purpose: link logs, metrics, traces, runbooks, docs, code, config, and alerts.

### Integrations
Purpose: represent external incident systems and notification destinations.

### Webhook subscriptions
Purpose: deliver incident lifecycle events to external systems.

## Analytics and reporting entities

### Analytics snapshots
Purpose: precomputed incident metrics for dashboards and reporting.

### Reports
Purpose: generated operational summaries and exports.

### Incident reviews
Purpose: post-resolution or scheduled review records with attendees, agenda, notes, and action items.

### Feedback, cost, and compliance records
Purpose: measure business impact and evidence compliance operations.

## Storage matrix by entity group

- D1: canonical incidents, comments, workflow records, integrations, compliance records where queryability matters
- KV: indexes, caches, snapshots, small collections, feature state, and workflow queues where appropriate
- R2: attachments, exports, report bundles
- Future services: Durable Objects for collaboration, Queues for async work, Workflows for orchestration

## Relationship notes

- One incident can have many comments, timeline events, evidence records, related items, attachments, and action logs.
- A response team can own multiple services and schedules.
- One SLA calendar can back multiple response targets.
- Bulk operations and recurrence records should reference the incident IDs they affect rather than duplicate the full incident body.

## Source files

- `src/types.ts`
- `src/services/incidents.ts`
- `src/handlers/incidents.ts`
