# Incident Operations

This document describes how operators should use the incident platform and how the major workflows behave.

## Operating model

The incident platform is designed for two broad client classes:
- human responders using the UI or API directly
- machine clients using authenticated API access for automation and integrations

The same incident record should support both audiences without duplicating state.

## Standard response flow

1. Create or ingest an incident
2. Review evidence, comments, and tags
3. Analyze and summarize the incident
4. Assign owners and responders
5. Apply governance and approval if remediation is required
6. Execute the chosen action or playbook step
7. Confirm resolution or failure
8. Close out with review, feedback, and reporting

## Incident creation and escalation

- Incidents should preserve the original source and correlation context.
- Escalation should update the incident history rather than replacing the original incident.
- Every escalation should be auditable and visible in the timeline.

## Maintenance windows

Maintenance windows are used to suppress noise or temporarily change incident behavior for known work.

Operational expectations:
- windows should be easy to schedule and cancel
- active windows should be queryable by service
- suppression state should be explicit rather than inferred
- windows should not erase alert history

## Auto-remediation and approval gates

Auto-remediation is intended for bounded actions only.

Rules:
- AI can recommend, but it cannot bypass policy
- approval is required when the action is sensitive or high-impact
- auto-remediation should record why it ran and what it changed
- retries must not double-run the same action

## Attachments and exports

- attachments live in R2
- incident metadata should track who uploaded the file and when
- exports should be treated as durable artifacts and stored with retention in mind
- downloads should be permissioned and traceable

## SLA handling

- SLA calendars define business-hour-aware timing
- response targets define the operational expectation by severity and action type
- SLA breaches should be recorded as discrete objects, not only as a boolean flag
- breach acknowledgement should be separate from breach creation

## Merge, split, and recurrence handling

### Merge
Use merge when multiple records represent the same operational event and should be consolidated into one primary record.

### Split
Use split when a single incident is found to contain multiple unrelated problems.

### Recurrence
Use recurrence tracking when a later incident appears to be a repeat of a previously resolved problem.

These workflows should always keep a reference trail so operators can reconstruct what happened.

## Audit and compliance

Every major incident transition should be captured in audit history:
- creation
- analysis
- approval
- execution
- status changes
- escalations
- merges and splits
- attachment and integration changes

Compliance records should remain attached to the incident lifecycle and be updateable without losing the original event trail.

## Manual verification checklist

When validating the system in a staging environment, verify:
- incident create/list/get flows
- analysis and approval paths
- execution results and failure handling
- comments, evidence, links, and tags
- responder team and on-call lookups
- SLA calendar and breach handling
- attachments and export downloads
- maintenance suppression behavior
- merge/split/recurrence paths
- realtime event delivery through the active transport layer
- audit records for each major transition

## Related source files

- `src/handlers/incidents.ts`
- `src/services/incidents.ts`
- `src/index.ts`
- `src/handlers/incidents.test.ts`
- `test/e2e.test.ts`
