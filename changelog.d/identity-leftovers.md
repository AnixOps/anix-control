### Fixed

- A password or second-factor change an administrator makes in identity
  ends the account's sessions again. Since finalize the kernel kept no
  credentials to compare, so only an email, administrator or ban change
  revoked tokens; a projection whose token version rose now revokes the
  older tokens itself.

### Changed

- Creating an admin API token needs a sign-in at most 10 minutes old
  (`403 step_up_sign_in_stale` otherwise). The kernel holds no password or
  second factor, so the `password`, `code` and `method` fields of the
  request are no longer read, and `step_up_required`, `step_up_failed` and
  `step_up_rate_limited` are no longer answered.
- The admin API token form no longer asks for a password, authenticator code
  or recovery code; a stale sign-in still offers *Sign in again*.

### Removed

- The kernel's own HS256 user tokens: Control signs no user token and
  verifies identity's (EdDSA) only.
- The full identity import (only the first administrator's claim still
  hands identity its account), the legacy credential mirror of
  `KernelIdentity.CreateSubscriber` and `ApplyAccountProjection` (identity
  no longer sends it), and the `authority_state` and `legacy_token_issuer`
  fields of `GetIdentitySettings`.
- The rehearsed route list (`config/package-route-defaults.json`) and the
  route modes `GetPackageConfig` answered: every built-in package serves its
  routes natively.
- The setting `alerts.phase_stuck_after` (already ignored).
- Migration `0034_drop_identity_leftovers` drops `v2_mfa_login_attempt` and
  `v4_kernel_identity_cutover`.
