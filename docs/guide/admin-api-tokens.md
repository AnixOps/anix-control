# Admin API Tokens For Automation

A script, a CI job or a monitoring probe can call the administrator APIs with
a personal access token instead of an administrator's password and session.
The rules, endpoints and threat model are in the
[reference](../reference/admin-api-tokens.md).

## Create A Token

Sign in as the administrator who will own the token (tokens are created
from a session, never with another token) and call, from the admin UI or with
the session's bearer token:

```sh
curl -sS -X POST https://panel.example.com/api/v4/kernel/api-tokens \
  -H "Authorization: Bearer $SESSION_JWT" -H 'Content-Type: application/json' \
  -d '{"name":"nightly export","scope":"read","expires_in_days":90,"password":"..."}'
```

- **Scope.** `read` for dashboards, exports and probes: `GET` and `HEAD`
  only, and it cannot read node credentials or the Telegram bot token. `admin` for automation that must
  change things: what you may do, except managing tokens.
- **Expiry.** Always set one (90 days is a good start) and put the renewal in
  your calendar. A token without an expiry works until it is revoked.
- **Re-authentication.** Your current password, or, when your account has a
  second factor, `"code":"123456","method":"totp"` (or a recovery code with
  `"method":"backup"`) instead of `password`.

The answer's `data.token` is the token (`anixadm_...`). **It is shown once.**
Put it in the secret store of the automation straight away; Control keeps only
its hash and cannot show it again. Lost: revoke it and create another.

## Use A Token

```sh
curl -sS https://panel.example.com/api/v3/plugins \
  -H "Authorization: Bearer $ANIXOPS_TOKEN"
```

- Header only. A token in a URL (`?token=...`) is refused and audited, and
  the URL is already in the proxy and access logs: revoke that token.
- Only administrator APIs: `/api/v2/admin`, `/api/v3` and `/api/v4`. It does
  not work on user routes.
- `401` means the token is unknown, revoked, expired or its owner can no
  longer use it (the answer is the same for all, on purpose); `403` means the
  scope does not allow the request; `429` means this address was refused too
  often, wait for `Retry-After`.

## List And Revoke

```sh
curl -sS https://panel.example.com/api/v4/kernel/api-tokens \
  -H "Authorization: Bearer $SESSION_JWT"            # yours: id, name, scope, hint, last use
curl -sS -X DELETE https://panel.example.com/api/v4/kernel/api-tokens/<id> \
  -H "Authorization: Bearer $SESSION_JWT"            # ends it at once
```

The list shows the last four characters (`hint`), the expiry and when and
from which address the token was last used. A token you did not expect to be
used lately, or used from an unknown address, should be revoked.

## When An Administrator Leaves, Or A Token Leaks

- **Departed or demoted administrator.** Ban or demote the account: its
  tokens stop working on the next request, with no cache, and are revoked when
  they are next tried. To revoke them explicitly, a super administrator (an
  administrator who is not staff and not banned) lists them with
  `GET /api/v4/kernel/api-tokens?user_id=<id>` and revokes each with `DELETE`.
- **Leaked token.** Revoke it (`DELETE`, as its owner or as a super
  administrator), create a replacement, and look at the operation log
  (`v2_operation_log`, `module = 'admin_api_token'`) for the token's `id`: the
  entries `admin_api_token_use` list the writes it made, with their status,
  and `admin_api_token_use_denied` the refused attempts. The administrator
  audit (`v2_audit_log`, for `/api/v2/admin`, `/api/v3` and `/api/v4/forward`)
  has those requests themselves, and the access log lines carry
  `api_token_id`.
- **Rotating.** Create the new token, deploy it, check that the old one's
  `last_used_at` stops moving, then revoke the old one.

## What To Know

- Up to 25 active tokens per administrator.
- Every use reads the owner's account: a banned, demoted or deleted owner
  ends the token. A token revoked for that reason stays revoked if the owner
  is restored.
- Failed attempts are limited per client address: 10 refused tokens in 5
  minutes lock the address out of token use for 10 minutes. JWT sessions are
  not affected.
- Behind a reverse proxy, set `server.trusted_proxies` so the client address
  in the audit entries and the limit is the caller's, not the proxy's.
