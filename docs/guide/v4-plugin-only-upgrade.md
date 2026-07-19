# V4 Plugin-Only Upgrade

This runbook applies to the formal `v4.0.0` package-only Control release.
All supported `/api/v2` business routes resolve from signed package
declarations. There is no request-time fallback to an uninstalled, disabled,
unhealthy, unsigned, or incompatible package.

## Release Preconditions

Do not start an upgrade until the release record contains all sixteen signed
Control package artifacts, manifests, signatures, public keys, and SBOMs:

```text
identity-platform       subscription          proxy-node
plan                    order                 payment
forward                 ticket                notification
knowledge               machine-telemetry     nftables-forward
gost-mesh               nat-egress            wireguard
protocol-runtime
```

The release owner must retain the formal-root verification output, the v2
catalog result, the plugin-only route-gate result, WebSocket relay result, and
both SQLite and PostgreSQL rehearsal results. A missing PostgreSQL result is a
release blocker, not a waived check.

Canary and support approvals are public, fixed-schema declarations signed by
the official release root. They bind the exact sixteen artifact digests; do
not place credentials, API tokens, private host addresses, personal data, or
free-form operational notes in either declaration.

After the formal package directory is built and the canary/support decisions
are approved, create the two declarations from that exact package directory:

```bash
config/scripts/create_v4_approval.py \
  --kind canary \
  --packages-dir artifacts/v4-packages \
  --signing-key "$ANIX_RELEASE_SIGNING_KEY" \
  --trusted-official-public-key "$ANIX_OFFICIAL_PUBLIC_KEY" \
  --output artifacts/canary-approval.json
config/scripts/create_v4_approval.py \
  --kind support \
  --packages-dir artifacts/v4-packages \
  --signing-key "$ANIX_RELEASE_SIGNING_KEY" \
  --trusted-official-public-key "$ANIX_OFFICIAL_PUBLIC_KEY" \
  --output artifacts/support-approval.json
```

`ANIX_RELEASE_SIGNING_KEY` and `ANIX_OFFICIAL_PUBLIC_KEY` in these commands
are filesystem paths, not inline key material. Keep the private-key file mode
at `0600`. `ANIX_OFFICIAL_PUBLIC_KEY` must be a pre-existing root from the
operator trust store whose fingerprint was pinned out of band; never extract it
from the candidate checkout, its configuration, or its evidence archive.

Run the complete rehearsal against the exact candidate before scheduling the
maintenance window:

```bash
config/scripts/run_v4_rehearsal.sh \
  --tag v4.0.0 \
  --signing-key "$ANIX_RELEASE_SIGNING_KEY" \
  --official-public-key "$ANIX_OFFICIAL_PUBLIC_KEY" \
  --postgres-dsn "$ANIX_TEST_POSTGRES_DSN" \
  --canary-evidence artifacts/canary-approval.json \
  --support-evidence artifacts/support-approval.json
```

The command writes a machine-readable evidence file. Verify that file before
approving the candidate with a verifier from a previously verified immutable
Control source tree, not the candidate tree that generated it:

```bash
: "${ANIXOPS_TRUSTED_CONTROL_SOURCE:?set to a previously verified immutable Control source tree}"
python3 "${ANIXOPS_TRUSTED_CONTROL_SOURCE}/config/scripts/verify_v4_evidence.py" \
  --input artifacts/v4-rehearsal-evidence.json \
  --trusted-official-public-key "$ANIX_OFFICIAL_PUBLIC_KEY"
```

## Upgrade Procedure

1. Freeze package and schema changes, then create and verify an application
   and database backup using the deployment runbook for the selected storage
   engine.
2. Upgrade Control using the version-pinned installer in
   [release-installation.md](release-installation.md). Preserve the existing
   configuration and the official package public key.
3. Import only the signed artifacts from the same `v4.0.0` evidence bundle.
   Do not mix package versions, manifests, signatures, or public keys from
   another release.
4. Install all sixteen Control packages while disabled. Confirm the manifest,
   artifact digest, compatibility-route digest, migration index, and Control
   host entrypoint for every package before enabling traffic.
5. Apply additive package migrations and wait for each host to report the
   intended generation and healthy lease. Any migration or host failure stops
   the rollout; do not enable a partial package set.
6. Enable the verified package set as one release generation. Start with the
   agreed canary cohort, then expand only after the route, host, and client
   checks below remain healthy.
7. Preserve the previous signed package bundle and its evidence for the full
   rollback window.

## Acceptance Checks

Run these checks from the release checkout and record their output with the
change request:

```bash
python3 config/scripts/check_v2_package_route_catalog.py
python3 config/scripts/check_plugin_only_routes.py
python3 config/scripts/check_release_stage.py --tag v4.0.0
```

Verify representative public, user, administrator, node, Agent, and
WebSocket routes through normal clients. The active package host must serve
each request. Deliberately disabling a non-canary package in a disposable
environment must return a package-unavailable response; it must not invoke a
legacy request path.

Stop the rollout immediately on a signature/trust-root mismatch, route
declaration mismatch, failed migration, host lease loss, sustained gateway
errors, or an unexpected package-unavailable result. Use the rollback runbook
instead of manually editing package files, database rows, or router settings.
