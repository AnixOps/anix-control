# Admin API Tokens (Reference)

An admin API token is an administrator's personal access token for
automation: a script, a CI job or a monitoring probe that calls the
administrator APIs without signing in. This page is the reference; the
operator steps are in [Admin API tokens](../guide/admin-api-tokens.md).

## What a token is

| Property | Value |
|---|---|
| Format | `anixadm_` followed by 43 URL-safe base64 characters (32 random bytes, 256 bits); 51 characters in all |
| Stored | only its SHA-256 (`v4_kernel_admin_api_token.token_hash`) and the last four characters (`hint`); the token is shown once, in the answer that creates it |
| Owner | one administrator (`v2_user.is_admin = 1`); the owner's rights are read from `v2_user` on every use |
| Scope | `read` or `admin` |
| Expiry | optional (`expires_in_days`, 1 to 730); no expiry is allowed, and not advised |
| Limit | 25 active (unrevoked, unexpired) tokens per administrator |
| Last use | `last_used_at` and `last_used_ip`, written at most every 5 minutes per token |

The table is new and kernel-owned (`v4_kernel_` prefix: no package can read
it); no existing table is changed.

## Scopes

| Scope | Allows | Never allows |
|---|---|---|
| `read` | `GET` and `HEAD` on the administrator APIs | any other method; the reads that still answer a secret in clear (today `GET /api/v2/admin/nodes/:id/credentials` and `GET /api/v2/admin/telegram/bot`; every other secret is masked in administrator answers); managing tokens |
| `admin` | what the owner may do on the administrator APIs, including the super-administrator actions when the owner is one (the handlers read the owner's rights from the database) | managing tokens |

Scopes are a column, so areas (for example `read:forward`) can be added
without a schema change. They are not offered yet: the router has no
per-area metadata, and a path-prefix scheme would drift from it.

## Where a token works

`Authorization: Bearer <token>`, on the administrator APIs only:

- `/api/v2/admin/*` and `/api/v2/admin/agent/*`,
- `/api/v3/*` (the kernel API),
- `/api/v4/*` (the kernel, module, Agent and forwarding administration).

It is not accepted on user routes (`/api/v2/user/*`, the identity routes),
on the flux-compatibility routes that share the user authentication
(`POST /api/v2/user/reset`, `/speed-limit/*`, `/tunnel/user/*`), or on any
node, Agent or public route: those refuse it like any string that is not a
JWT.

**Never in a URL.** A request that has something starting with `anixadm_`
in its query string is refused with `401 api_token_in_url`, whatever else it
carries, and is audited. The URL has reached the access log by then:
revoke that token. Only the `Bearer` scheme is read; a bare token without a
scheme is not a token. JWTs are checked exactly as before (algorithm pinning
and revocation are unchanged).

## Authentication answers

| Status | `code` | Meaning |
|---|---|---|
| 401 | `api_token_invalid` | malformed, unknown, revoked, expired, or its owner is banned, demoted or deleted: one answer for all, so a caller learns nothing about which tokens exist |
| 401 | `api_token_in_url` | a token in the query string |
| 403 | `api_token_forbidden` | the token's scope does not allow this request, or the request manages tokens |
| 429 | `api_token_rate_limited` | the client address was refused too often; `Retry-After` says when to retry |
| 503 | `api_token_unavailable` | the database could not be read |

Refusals count against the client address: the 10th within 5 minutes locks
the address out of token authentication for 10 minutes (valid tokens
included). JWT sessions are unaffected.

## Endpoints

All three need a signed-in session (a JWT). An API token on them is
`403 api_token_forbidden` from the middleware, and `403 session_required`
from the handler.

### `POST /api/v4/kernel/api-tokens`

Creates a token. The router's `adminLimiter` applies as for every
administrator route.

```json
{
  "name": "nightly export",
  "scope": "read",
  "expires_in_days": 90,
  "password": "<the administrator's current password>"
}
```

- `name`: 1 to 100 printable characters (counted as characters, not bytes); `scope`: `read` or `admin`.
- Re-authentication, as the user's own subscription reset requires
  (`VerifyStepUp`): with a second factor enabled, `code` and `method`
  (`totp` or `backup`; a recovery code works once) instead of `password`.
  Failed attempts are limited per administrator: the 5th failure in 15
  minutes locks creation out for 15 minutes (`429 step_up_rate_limited`).
- When identity holds the credentials (the account's legacy password is the
  unusable marker, after the identity cutover is finalized) the kernel cannot
  check one. It then requires a sign-in at most 10 minutes old, which
  identity's login (with the MFA policy) has just checked; otherwise
  `403 step_up_sign_in_stale` (up to 4.2.0-rc.2 this was `step_up_required`
  with the message `sign in again and retry within 10 minutes`).

```json
{
  "data": {
    "token": "anixadm_...shown once...",
    "api_token": {
      "id": "6b0e…", "user_id": 7, "name": "nightly export", "scope": "read",
      "hint": "k3Zq", "expires_at": "2027-01-03T12:00:00Z", "last_used_at": null,
      "created_at": "2026-10-05T12:00:00Z", "revoked_at": null
    }
  }
}
```

`Cache-Control: no-store`. Errors: `400 invalid_request`,
`403 step_up_required` / `step_up_sign_in_stale` / `step_up_failed` /
`not_an_administrator`,
`409 too_many_tokens`, `429 step_up_rate_limited`.

### `GET /api/v4/kernel/api-tokens`

Lists the caller's active tokens (never a secret): `include_inactive=true`
adds revoked and expired ones. A super administrator (an administrator who is
not staff and not banned) may add `user_id=<id>` or `all=true`; anyone else
gets `403 super_admin_required`. Each row carries `owner_email`, the owner's
current email (empty if the owner no longer exists), so a list of everyone's
tokens says whose each is.

### `DELETE /api/v4/kernel/api-tokens/:id`

Ends a token at once. An administrator revokes their own; a super
administrator revokes anyone's. A token that is not yours, or does not exist,
is `404`. Revoking a revoked token is `200` with `"changed": false`.

## Owner state, at every use

On every request the kernel reads the owner's row. A token stops working
when its owner is banned, is no longer an administrator or is deleted, with
no cache in between. The token is then also **revoked for good**
(`revoke_reason: owner_not_admin`): unbanning or promoting the owner again
does not bring back a credential that was out there while its owner was not
trusted. An unused token whose owner is restored before it is ever tried
again keeps working, so give tokens an expiry.

## Audit

Entries go to the operation log (`v2_operation_log`, module
`admin_api_token`). The administrator audit (`v2_audit_log`, which covers
`/api/v2/admin`, `/api/v3` and `/api/v4/forward`) records the request itself
as for any administrator request, and the structured log line of such a
request adds `auth_method=api_token` and `api_token_id`; the `admin_api_token_use`
entry below covers every write, `/api/v4/kernel` included. The administrator
audit endpoint (`GET /api/v2/admin/system/audit-logs`) lists module `system`
only, so these entries are read from the database today. Entries carry the token's id, name, scope, hint, expiry and owner, never the token or
its hash.

| Action | When |
|---|---|
| `admin_api_token_create` | a token was created |
| `admin_api_token_create_denied` | a creation was refused for its re-authentication (no password or code is kept) |
| `admin_api_token_revoke` | a token was revoked, by its owner or by a super administrator |
| `admin_api_token_use` | a write (not `GET` or `HEAD`) made with a token, with the status code |
| `admin_api_token_use_denied` | an unusable token (`malformed`, `unknown`, `revoked`, `expired`, `owner banned`, `owner not an administrator`, `owner deleted`), a request outside the scope (`scope_read_only`, `scope_secret_read`), a request that needs a session (`interactive_only`), or a token in a URL (`token_in_url`). The same subject and reason is written at most once a minute |

## Threat model

| Threat | Mitigation |
|---|---|
| Token theft (repository, log, laptop) | stored only as a hash, so a database or backup leak yields no usable token; 256-bit random value, so it cannot be guessed; expiry and revocation; `read` scope for what does not need to write; last use time and address to spot a stolen token; rate-limited failures; never accepted in a URL |
| Brute force | 256 bits of entropy make guessing infeasible; the lookup is by hash (an attacker learns nothing from timing without the preimage), compared again in constant time; refusals are all one answer and lock the address out |
| Scope escalation | the scope is checked by the middleware on every request, before any handler; `read` allows safe methods only and never a secret-revealing read; token management needs a session, so a stolen token cannot mint a longer-lived one or hide itself by revoking others |
| Weaker path than the session | creation needs the re-authentication the repo already uses (password, or MFA code when MFA is on), and a session |
| Deleted, banned or demoted administrator | rights are read at use; the token is revoked on first refusal; a super administrator can list and revoke any administrator's tokens |
| Token in logs | only the `Authorization` header carries it, and neither the access log nor the audit log records headers (a test checks the log output); a token in a URL is refused, audited by path only, and the answer says to revoke it |
| Using a token on a user route | the token middleware exists on the administrator groups only; elsewhere the token is not a JWT and fails |

Residual risks: a token is a bearer credential, so whoever holds it acts as
its owner within its scope until it is revoked or expires. `admin` scope is
as powerful as the owner; prefer `read`, short expiries and one token per
automation.
