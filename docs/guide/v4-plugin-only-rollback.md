# V4 Plugin-Only Rollback

This runbook rolls back a `v4.0.0` package-only release. It restores a
previous verified package generation; it does not re-enable direct legacy v2
business handlers as an emergency bypass.

## Rollback Triggers

Start the rollback procedure when a signed package cannot be verified, a
required host is unhealthy or loses its lease, a package migration fails, a
WebSocket relay cannot be established, or the canary shows an unexplained
contract regression. Treat a package-unavailable response during an expected
healthy route as a stop condition until its package state is understood.

## Procedure

1. Freeze package configuration and schema changes. Record the current
   release tag, package versions, generations, host health, route-gate output,
   and the observed fault.
2. Drain affected package hosts and stop traffic expansion. Keep the current
   artifacts and logs intact for analysis.
3. Select the last known-good evidence bundle. It must contain the complete
   signed package set, formal-root verification, migration records, and the
   matching Control release metadata.
4. Restore every affected package to the prior signed version and wait for its
   expected generation, migration checkpoint, and healthy lease. Keep package
   versions coherent; do not combine releases to repair a single route.
5. Restore application files with the version-pinned installer only when the
   compatible package generation requires the prior Control binary. Preserve
   configuration unless the incident record explicitly calls for a tested
   configuration rollback.
6. Restore a database backup only through the approved SQLite or PostgreSQL
   recovery runbook and only when the reverse-migration evidence authorizes
   it. Never edit package tables or package archives by hand.
7. Re-run the route catalog, plugin-only route gate, package signature checks,
   representative HTTP checks, and WebSocket relay checks before reopening
   traffic.

## Required Evidence

Attach the following to the incident and rollback approval:

- the triggering health, route, migration, or relay failure;
- the original and restored package ids, versions, generations, and digests;
- formal public-key and signature verification output;
- the route-catalog and plugin-only gate output before traffic resumes;
- SQLite and PostgreSQL recovery/rehearsal outcomes; and
- the decision on whether the failed candidate remains quarantined.

Use the same evidence validator that gates a new release. Run it from a
previously verified immutable Control source tree and supply the pinned root
from the operator trust store; neither may come from the rollback candidate:

```bash
: "${ANIXOPS_TRUSTED_CONTROL_SOURCE:?set to a previously verified immutable Control source tree}"
: "${ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY:?set to the separately managed pinned official root}"
python3 "${ANIXOPS_TRUSTED_CONTROL_SOURCE}/config/scripts/verify_v4_evidence.py" \
  --input artifacts/v4-rehearsal-evidence.json \
  --trusted-official-public-key "$ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY"
```

A rollback is complete only after the restored release has passed its own
health and contract checks. A successful process restart alone is not proof
that package-owned v2 routes are safe to reopen.
