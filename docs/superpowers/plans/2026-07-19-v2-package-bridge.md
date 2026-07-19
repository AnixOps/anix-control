# V2 Package Bridge Addendum

**Status:** approved implementation addendum to `2026-07-19-v2-full-package-cutover.md`.

## Why This Exists

The package-host protocol deliberately starts child processes without a
database DSN, JWT signing secret, or full Control configuration. That
is the correct isolation boundary, but it means a real identity package
cannot implement login merely by receiving an HTTP dispatch RPC. This
addendum supplies the missing, narrow kernel-to-host bridge before the
identity-platform cutover proceeds.

## Security Invariants

- A package host receives only a private inherited socket endpoint. It never
  receives a database DSN, signing key, full configuration, or arbitrary SQL
  capability.
- The parent endpoint is bound to one package id, version, lifecycle
  generation, and host lease. A replacement generation receives a new bridge.
- Every package call carries a cryptographically-random, short-lived,
  single-request capability. The kernel mints it after v2 route resolution;
  the host cannot choose the principal, route, package, or deadline it grants.
- The bridge exposes explicit named operations only. It rejects unknown
  operations, a wrong route, a stale generation, a reused capability, and a
  deadline-expired request. It has no generic query, filesystem, configuration,
  process, or token-signing operation.
- The bridge returns application data only. HTTP status, response headers, and
  transport framing remain controlled by the package-host RPC and compatibility
  gateway.

## Delivery Sequence

### 1. Bridge transport and request capability

Add a socketpair-backed, inherited package bridge endpoint (FD 4; host runtime
directory remains FD 3). `pluginhost.Manager` owns the parent endpoint for the
life of the host process. `Dispatch` mints a one-shot capability, includes it
in the already authenticated host dispatch, and revokes it after the response.
The host SDK exposes the capability to package code and a small bridge client.

Tests prove that a child receives no sensitive environment value, a capability
cannot be replayed, another host cannot use it, and generation replacement
invalidates old capabilities.

### 2. Explicit bridge protocol and identity operations

Add a versioned local bridge protocol with a typed request and response.
The first allowlist contains only `identity.auth.login` and
`identity.auth.register`; both are bound to their exact package route ids and
receive the kernel-derived principal/request metadata. The parent adapter uses
the existing authentication service during the migration period, keeping
password verification, JWT signing, MFA policy, audit logging, client IP, and
user-agent semantics inside Control.

This is intentionally an adapter, not a hidden raw database escape hatch.
Package-owned projections and checkpointed import migrations can replace each
operation independently after their corresponding data contract is available.

### 3. Build and lifecycle integration

Teach the package builder to compile a package `control/` module reproducibly
when present and archive validated source `compat/v2-routes.json`, migration
index, and listed migration files. Retain generated stub hosts only for
packages that have not yet supplied a Control host.

Wire the production lifecycle dispatcher to materialize a verified package
artifact into a private artifact root before starting its host. A missing or
unverifiable release remains fail-closed.

### 4. Exact legacy response compatibility

Introduce a `panel` unary envelope alongside `data` and `raw`. A `panel`
response must be a valid full legacy `{code,msg,ts,data}` response; the
gateway forwards it as that JSON object rather than nesting it under `data`.
Identity declarations and catalog records use `panel` for endpoints whose
published response follows that contract.

### 5. Identity-platform cutover waves

First cut over login and registration through the signed, executable identity
host and bridge. Then move the remaining identity family in behaviorally
coherent waves: profile/session, MFA/invitations, administration, and
platform configuration/audit/backup. Each wave keeps its existing Gin
middleware, has an enabled-host assertion, a disabled-host 503 assertion, and
a rollback assertion before the next wave starts.

## Non-Goals

- No direct database credentials or arbitrary SQL interface are introduced.
- No legacy handler is used as a router fallback after a route is cut over.
- No response-envelope guessing is allowed; every signed route declaration
  specifies its transport and envelope.

## Initial Acceptance Commands

```bash
GOWORK=off go test ./internal/packagebridge ./pkg/packagebridgesdk ./internal/pluginhost ./pkg/pluginhostsdk -count=1
GOWORK=off go test ./packages/identity-platform/control ./internal/compat/v2 ./internal/router -run 'TestV2(Login|Register)|Test.*Bridge' -count=1
python3 -m unittest packages/shared/tests/test_manifest_schema.py
```
