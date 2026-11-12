### Changed

- **Breaking, on upgrade:** Control starts only once identity is finalized.
  A database with users whose identity authority is not `finalized` is
  refused before any migration runs; on 5.0.3, import, cut over and finalize
  first ([UPGRADE.md](docs/UPGRADE.md)). A fresh database, or one without
  users, is recorded as finalized at start.
- identity-platform's 23 account routes (login, registration, MFA, the
  administrator's user management and MFA configuration, the profile, the
  dashboard and the resets) run natively only. Their answers are recorded
  in `internal/tests/identitycompat/testdata`, taken while CI proved them
  equal to the kernel's handlers.
- identity-platform creates its first signing key when it first signs, so
  the administrator of a fresh install can log in right after the claim
  instead of after the next key rotation pass (up to a minute, during which
  the failed logins counted toward the login limit). Control fetches
  identity's keys while it answers the first request that carries a token
  of an unknown key, so that request is no longer refused as expired, and
  retries a failed key refresh every 10 seconds instead of every 5 minutes,
  so that tokens keep verifying when the identity host stops soon after
  Control started.
- The first administrator's claim waits up to 15 seconds for the identity
  host after a start instead of answering `503 identity_unavailable` at
  once. A build without identity-platform (a source image) cannot claim.

### Removed

- The kernel's legacy login, registration, MFA and user administration
  handlers and the services only they used.
- The identity cutover API (`GET /api/v4/kernel/identity`, `POST
  /api/v4/kernel/identity/import`, `/cutover`, `/rollback`, `/finalize`),
  the gateway's route freeze (`503 package_route_frozen`) and the alerts
  `identity_import_stalled` and `identity_cutover_not_finalized`.
