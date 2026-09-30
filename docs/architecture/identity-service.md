# Identity Service

Status: DESIGN (2026-09-30). This supersedes the earlier decision in
[`package-extraction.md`](package-extraction.md) that identity stays owned by
the kernel. Login, registration, credentials, MFA and invite codes move into
the identity module (`identity-platform`), the first network module on the
[module runtime](module-runtime.md).

## Today

- **Routing.** `identity-platform` is a pass-through host. Its 45 routes go
  through `internal/identitybridge` back into the kernel handlers
  `handler/auth.go`, `user.go`, `admin.go`, `mfa.go`, `invite.go` and
  `system*.go`.
- **Tokens.** HS256 with a shared secret (`internal/utils/jwt.go`), checked in
  one place, `internal/authn` (step 8):
  - the algorithm is pinned to HS256, and `iss=v2board`, `exp` and `iat` are
    required;
  - new tokens carry a session id (`sid`);
  - revocations are enforced (see below).
- **Data.** `v2_user` mixes identity columns (email, password, is_admin,
  is_staff, banned) with subscriber and entitlement columns (plan, traffic,
  expiry, limits, balance). About 20 other kernel files read those identity
  columns.

## Target

| Concern | Owner | Contract |
|---------|-------|----------|
| Accounts, password hashes, MFA, login attempts, invite codes, sessions, signing keys | identity module, own schema | `IdentityService` |
| Access tokens | issued by identity (Ed25519 JWT), verified by the kernel gateway | JWT/JWKS, `GetTokenKeys` |
| Subscriber rows (id, uuid, subscription token) and entitlements | kernel (`v2_user`) until a subscriber module exists | `KernelIdentity.CreateSubscriber` / `UpdateSubscriber` |
| Identity columns of `v2_user` (email, is_admin, is_staff, banned) | read projection written only by `ApplyAccountProjection` | versioned, monotonic |
| Plugin permissions in login and profile responses | kernel access groups | `KernelIdentity.ResolveActorAccess` |
| Revocation | identity decides; kernel enforces | `KernelIdentity.PublishRevocation` |

`KernelIdentity` is served only to a package whose signed manifest declares
the capability `kernel.identity.v1`, and only if that package is signed by the
official trust root.

## Tokens

- **Algorithm.** EdDSA (Ed25519) with header `kid`. The claims are fixed by
  `contracts/identity/v1/access-token-golden.json`:
  - `iss=anixops-identity` and `aud=anix-control`;
  - `sub`, `user_id`, `email` and `is_admin`, so the ~70 handler sites that read
    them are unchanged;
  - `sid` (session), `tv` (token version) and `jti`.
- **Lifetime.** The configured `jwt.expire`, 24 h by default. The v2 login
  response keeps its shape, so the web UI and v2board clients need no change.
- **Keys.** Signing keys live only in the identity module, encrypted under
  `ANIX_IDENTITY_KEK` and shared by its replicas.
  - A `next` key is published at least one kernel refresh interval before it
    signs.
  - Retired keys verify until the last token they signed expires.
  - A compromised key is revoked, which forces those users to log in again.
- **Kernel verification.**
  - The kernel pulls keys with `GetTokenKeys` and persists them in
    `v4_kernel_identity_token_key`, so it keeps verifying while identity is
    down. The public keys are also served at `/api/v4/identity/jwks.json`.
  - The verifier pins the algorithm, requires `kid`, `iss` and `aud`, and
    allows 60 s of leeway.
  - The negative cases in `contracts/identity/v1/access-token-negative.json`
    must all fail.
- **Legacy tokens.** HS256 tokens stay valid only until 24 h after the
  cutover. Until then the kernel pins HS256 for them, which closes today's
  algorithm-confusion gap.

## Real-time revocation

The kernel checks every token against `v4_kernel_identity_revocation`
(`user_id`, `token_version`, `not_before`) and a session-id denylist. Both are
cached in memory. The identity module publishes revocations through an outbox
on:
- ban;
- password change;
- email change;
- delete;
- admin demotion;
- logout (`POST /api/v4/identity/logout`).

Restrictive changes are projected to the kernel before identity commits them.
A banned user is therefore rejected within seconds, including by node user
lists that read the `v2_user` projection.

**In place since step 8** (`internal/authn`):
- **Kernel-side triggers.**
  - The kernel's own user changes revoke in the same transaction: ban, and a
    changed password, email, admin flag or ban flag through `UserService.Update`.
  - Deleting a user also revokes.
  - A form that resends unchanged values revokes nothing.
- **Caching.** The HTTP middleware, the monitor WebSocket and the gRPC
  interceptor check the in-memory cache.
  - Revocations made in the process apply at once.
  - Others arrive within 5 s through a reload, which also prunes rows that can
    no longer match an unexpired token.
- **Second precision.** `iat` has one-second precision, so a time-based
  revocation also revokes a token issued in its own second.
- **By token version for identity tokens.**
  - A session-ending change identity projects carries the account's new token
    version (`ApplyAccountProjectionRequest.token_version`).
  - The kernel then ends identity tokens whose `tv` claim is lower, so a
    token issued right after the change works, even within the same second.
  - The kernel's own HS256 tokens carry no `tv` and are still ended by time.
  - A version that did not rise above the last one projected
    (`v4_kernel_identity_account.token_version`) falls back to time, so a
    revocation cannot be missed.
  - Revocations without a token version (delete, or legacy changes after a
    rollback) bound identity tokens by time
    (`v4_kernel_identity_revocation.identity_not_before`).

## Data split

- **Identity schema.** Tables `account`, `mfa`, `mfa_attempt`, `invite_code`,
  `signing_key`, `outbox`, `throttle` and `session`.
  - `account.id` equals `v2_user.id`.
  - TOTP secrets are encrypted at rest.
  - The login throttle is a table, so identity replicas share it.
- **Registration.** Identity calls `CreateSubscriber`, which is idempotent on
  the account UUID. The kernel allocates the `v2_user` id, uuid and
  subscription token, so logging in right after registering works.
- **Import.** The kernel reads the legacy identity columns, MFA and invite
  codes and pushes them with `ImportAccounts`. The import is checkpointed and
  can repeat as `updated_at` deltas. Identity never reads `v2_user.password`.
- **Authority state machine.** `kernel → importing → identity → finalized`.
  - **Before finalize,** identity mirrors credentials back into the legacy
    tables (`LegacyCredentialMirror`), so switching the routes back to
    `legacy` is an instant rollback.
  - **Finalize** stops the mirror, sets `v2_user.password` to a sentinel,
    deletes `v2_user_mfa` rows and disables HS256.
- **Settings.** The `auth.*` settings (registration policy, login limits, MFA
  configuration) move into the identity installation configuration. They are
  seeded once from `GetIdentitySettings`.

**Kernel side, in place since step 9** (`internal/kernelidentity`). It is
served on the local package bridge and on the module listener to the bound
instance's generation.
- **Authorization.** Every call must come from the current generation of an
  official AnixOps package whose signed release, verified again on every call,
  declares `kernel.identity.v1`.
- **Links.** `v4_kernel_identity_account` links an account UUID to its
  `v2_user` id and records the last projection version applied.
  - `CreateSubscriber` creates the row with a fresh node UUID and
    subscription token, and a password (`!identity`) that never matches.
  - A repeated call for the same account returns the same user.
  - An email that belongs to another subscriber is `AlreadyExists`.
- **Entitlements.** `UpdateSubscriber` takes the entitlement keys of the v2
  admin user update (`balance`, `plan_id`, `group_id`, `expired_at`,
  `transfer_enable`, `speed_limit`, `device_limit`, `flowResetTime`,
  `remark_content`). A new plan fills group, traffic and limits exactly as the
  admin API does. Only linked subscribers can be changed.
- **Projection.** `ApplyAccountProjection` ignores versions at or below the
  stored one. It writes email, admin, staff and ban flags through the same
  path as the admin API, so a ban or demotion revokes at once.
- **Legacy mirror.** Password hash, algorithm and salt, and TOTP, are copied
  back until the state is `finalized`: through `ApplyAccountProjection` for
  existing subscribers, and through `CreateSubscriber` for new ones. Backup codes are not: legacy compares
  them in plain text and identity keeps only hashes, so after a rollback users
  use TOTP or regenerate codes.
- **Authority state.** `v4_kernel_identity_authority` holds the state (no row
  means `kernel`) and the import's checkpoint.
- **Account import.** `ImportAccounts` is started by the kernel against the
  identity module's `IdentityService`, so it arrives with that service in
  step 10.

## Routes

- **Group A — moves to identity, switched together at cutover:**
  - login and register;
  - user MFA (6) and admin MFA configuration (2);
  - admin user create, update, ban, unban and delete;
  - invite code generate and list.

  For admin create and update, identity handles the identity fields and
  passes the entitlement fields to `UpdateSubscriber`. Responses stay
  byte-compatible with v2 (panel envelope, Chinese error strings,
  `Retry-After`, the MFA response shapes and the permission fields). Parity is
  proven with `internal/tests/packagecompat`.
- **Stay bridged for now:** profile, dashboard, and admin user list, detail
  and stats. They read the projection.
- **Moved to other packages, still bridged (done, step 7):**
  - system configuration, audit and backup (12 routes) → new package
    `platform`;
  - `/user/reset` → `forward`;
  - commissions, withdrawals and invite statistics and configuration (8) →
    new package `affiliate`.

  The HTTP paths are unchanged; only the owner and route id changed
  (`platform.*`, `affiliate.*`, `forward.user.reset.post`).
  - **Transition.** Old identity-platform releases still declare these
    routes. The kernel keeps their old `identity.*` ids callable, and while
    both packages are active it resolves the route to the new owner
    (`internal/compat/v2/moved_routes.go`).
  - **Upgrade order.** Install `platform` and `affiliate`, and upgrade
    `forward`, before upgrading identity-platform: the routes then keep
    serving throughout.

## Failure behaviour

- **Identity down.** Login, registration and MFA return 503. Issued tokens keep
  working.
- **Rollback before finalize.** `POST /api/v4/kernel/identity/rollback`
  switches group A back to `legacy` with the authority. A later cutover
  imports what changed meanwhile. Backup codes generated natively are not
  mirrored, so after a rollback users use TOTP or regenerate codes.
- **Kernel restart.** Sessions and capabilities are lost; modules bind again.

## Delivery

7. Re-home the non-identity routes.
8. `internal/authn` verifier, HS256 pinning and the revocation store.
9. `KernelIdentity` in the kernel, `kernel.identity.v1`, links and the
   versioned projection.
10. Identity storage, keys, `GetTokenKeys`, the kernel EdDSA verifier, and
    the kernel-led account import, in three steps:
    - 10a: the token contract in the SDK and the identity core module;
    - 10b: identity storage and `IdentityService` (host side, in place:
      signing keys in the package's storage, sealed under `ANIX_IDENTITY_KEK`,
      which Control passes to a local host from `identity.kek`; rotation in
      the host; `GetTokenKeys`), then the kernel's key pull, JWKS and EdDSA
      verification (in place: `internal/identitykeys` pulls every 5 minutes
      and when a token names an unknown `kid`, persists the keys in
      `v4_kernel_identity_token_key`, and `internal/authn` accepts EdDSA
      tokens for `aud=anix-control` beside the HS256 ones, through the same
      revocation store; `GET /api/v4/identity/jwks.json` publishes them);
    - 10c: the kernel-led account import (in place):
      - **Trigger.** `POST /api/v4/kernel/identity/import` (`{"delta": true}`
        for a delta), followed with `GET /api/v4/kernel/identity`.
      - **Batches.** The kernel reads users by id and streams each batch to
        `ImportAccounts`, which commits it in one transaction.
        It checkpoints after each batch, so an interrupted import resumes.
      - **Links.** Legacy users get a deterministic account UUID and a link
        row.
      - **Credentials.** Password hashes travel with their algorithm and
        salt. TOTP seeds are sealed under the KEK. Backup codes go as SHA-256
        digests, and identity keeps only KEK-keyed hashes of those.
      - **Delta.** A delta sends what changed since the previous import
        started.
      - **Authority.** The state moves from `kernel` to `importing`. Imports
        are refused once identity is authoritative.
      - **Invite codes stay with Control.** They carry subscriber and
        commission data. Registration will validate and consume a code
        through `CreateSubscriber`, so `/user/invite` and
        `/user/invite/generate` leave group A.
11. Group A native handlers with parity tests, in three steps:
    - 11a (in place): login and registration in identity.
      - Accounts, bcrypt passwords, the ban flag and MFA come from identity;
        expiry comes from `kapi_user_directory_v1`.
      - Attempt limits live in identity's `throttle` table, shared by
        replicas. The configuration is seeded once from
        `GetIdentitySettings`.
      - Login issues an EdDSA token for `aud=anix-control`. The permission
        fields come from `ResolveActorAccess`.
      - Registration creates the subscriber with `CreateSubscriber`, which
        also consumes the invite code.
      - `internal/tests/identitycompat` proves the responses equal the v2
        handlers on SQLite and PostgreSQL. Only the token differs, by
        design.
    - 11b (in place): the six user MFA routes and the admin MFA
      configuration.
      - TOTP secrets are sealed and backup codes keyed-hashed in identity.
      - The admin configuration lives in identity's settings document, with
        the v2 defaults, coercions and normalization.
      - Parity is proven except for the random secrets and codes, and
        `last_used`, which identity keeps to the second.
    - 11c (in place): admin user create, update, ban, unban and delete.
      - Identity changes the account and projects email, admin, staff and
        ban flags with `ApplyAccountProjection`, so Control revokes as v2
        does.
      - A password change also mirrors the hash and the MFA state to the
        legacy columns.
      - Entitlements go to `UpdateSubscriber`. Create answers with
        `GetSubscriber` (new RPC); `CreateSubscriber` takes the admin and
        staff flags.
      - Parity covers the responses and the resulting Control state:
        subscribers, revocations and legacy MFA rows.
12. Cutover, revocation push and finalize, in two steps:
    - 12a (in place): what a rollback before finalize needs, and logout.
      - **Mirror everywhere.** Every native change that the legacy routes
        read is mirrored until finalize:
        - registration and admin create pass the password hash in
          `CreateSubscriber.legacy_mirror` (new field), applied with the
          subscriber and without revoking anything;
        - TOTP setup, enable and disable mirror the MFA state;
        - password changes mirror as in 11c.
      - **Rollback test.** `internal/tests/identitycompat` registers and
        enables MFA natively, then logs in through the legacy handler with
        the same password and TOTP secret.
      - **Logout.** `POST /api/v4/identity/logout` ends the calling session
        (its `sid`) until the token would have expired, in every process,
        for HS256 and EdDSA tokens alike. Other sessions of the user go on.
    - 12b (in place): the cutover, rollback and finalize
      (`internal/identitycutover`), each recorded in
      `v4_kernel_identity_cutover`.
      - **Cutover.** `POST /api/v4/kernel/identity/cutover` runs in the
        background; `GET /api/v4/kernel/identity` follows it. It needs a
        completed full import and identity's token keys. Then:
        1. a catch-up delta import;
        2. group A is paused: the v2 gateway answers 503 with `Retry-After`,
           and requests already dispatched finish;
        3. a final delta import;
        4. in one transaction, the authority becomes `identity` and group A
           `native`;
        5. once the identity host reports the new configuration revision and
           serves group A natively, plus one poll interval for other
           instances, group A resumes.

        If the host does not confirm within 30 seconds, both switch back
        (`cutover_aborted`).
      - **Complete deltas.** A delta also carries users whose
        `v2_user_mfa` row changed. Disabling MFA in legacy touches the
        user. Every import ends by deleting the identity accounts of deleted
        subscribers, sent as `ImportAccountsRequest.deleted_user_id` (new).
      - **Consistency.** Configuration writes are refused unless group A's 15
        routes are native together, and exactly while identity is
        authoritative. Only the cutover and rollback change both.
      - **Rollback.** `POST /api/v4/kernel/identity/rollback` pauses group A,
        switches it to legacy and the authority to `importing`.
      - **Finalize.** `POST /api/v4/kernel/identity/finalize`
        (`{"force": true}` skips the day). One day after the latest
        cutover, when pre-cutover tokens have expired:
        - legacy passwords become unusable and `v2_user_mfa` is emptied;
        - the authority becomes `finalized`, so identity stops mirroring;
        - Control refuses HS256 tokens, from then on and after restarts.

        There is no rollback after finalize.
      - **Bootstrap.** `InitAdmin` creates no default administrator once
        identity is authoritative.
13. End-to-end acceptance on Compose and kind (in place).
    - `config/scripts/identity_cutover_acceptance.sh` drives the whole move
      against a running Control.
    - The kind smoke runs it against two identity replicas.
    - The Compose smoke runs it, then checks that issued tokens outlive the
      identity module.

Deferred:
- deleting the legacy identity handlers (after finalize and the rollback
  window);
- multiple kernel replicas;
- OIDC discovery and refresh tokens;
- a separate subscriber and entitlement module.
