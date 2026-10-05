# Node Credential Rotation (Admin API)

`POST /api/v4/kernel/agents/rotate-credentials` rotates a node's Agent
credentials: it revokes what the node's Agent holds, optionally replaces a
proxy node's API key, and issues a fresh one-time enrollment credential so
the Agent can enroll again. The operator procedure is in
[Rotating A Node's Credentials](../guide/agent-onboarding.md#rotating-a-nodes-credentials);
the design of the Agent PKI is in
[node-ops-service.md](../architecture/node-ops-service.md#53-identity-and-enrollment).

## Admission

- Under `/api/v4`, so the admin admission of v3 and v4 applies: the
  administrator rate limiter, a valid access token, the `is_admin` claim.
- **Super administrators only** (`super_admin_required`): an administrator
  who is not staff and not banned, read from the database on every call, so a
  demoted or banned administrator loses the right at once. Issuing install
  tokens needs the same, and a rotation also issues an enrollment credential.
- `/api/v4/kernel` is outside the `AuditLog` middleware, so the route writes
  its own entries to the operation log (below).

## Request

```json
{
  "node": "proxy-12",
  "rotate_api_key": false,
  "ttl_seconds": 3600,
  "reason": "disk of the host was stolen"
}
```

| Field | Meaning |
|---|---|
| `node` | required: `proxy-<id>` or `forward-<id>` |
| `rotate_api_key` | proxy nodes only, default `false`: also replace the node's API key (`400` for a forward node, whose token belongs to the frozen legacy forward runtime) |
| `ttl_seconds` | lifetime of the new enrollment credential: 60 to 604800, default 3600 |
| `reason` | optional, at most 200 bytes (UTF-8), recorded in the audit entry |

## Answer

`201`, `Cache-Control: no-store`:

```json
{
  "data": {
    "node": "proxy-12",
    "revoked": {"certificates": 1, "enrollments": 2, "link_certificates": 0},
    "api_key_rotated": false,
    "enrollment": {"id": "...", "node_kind": "proxy", "node_id": 12, "method": "enrollment_credential", "expires_at": "..."},
    "expires_at": "2026-10-05T12:00:00Z",
    "credential": "anixagt_..."
  }
}
```

- `credential` is shown **once**; Control keeps only its SHA-256, like every
  enrollment credential (`POST /api/v4/kernel/agents/enrollment-tokens`).
- `revoked` counts what the rotation revoked: Agent certificates,
  enrollments (the Agent's own and unused enrollment credentials) and
  forward link certificates that were not revoked yet.
- The new API key is never in the answer. Read it with the existing audited
  `GET /api/v2/admin/nodes/{id}/credentials` (action `reveal`).

| Status | Code | Cause |
|---|---|---|
| 400 | `invalid_request` | bad body, unknown node form, `ttl_seconds` out of range, `reason` too long, `rotate_api_key` on a forward node |
| 403 | `super_admin_required` | not a super administrator |
| 404 | `node_not_found` | no such node |
| 409 | `node_disabled` | the node is disabled: enable it first |
| 409 | `agent_pki_disabled` | the built-in agent CA is off (`module_runtime.ca_kek`) |

## What happens

One database transaction, with the node's row locked (PostgreSQL row lock;
SQLite has one writer), so concurrent rotations of one node run one after
the other:

1. `rotate_api_key`: a new API key is generated and stored with the code the
   `KernelNodeOps` `IssueCredential` executor and the legacy node routes
   use (`service.IssueProxyNodeCredentialsTx`): the legacy columns, its hash
   and the credential split's tables, in the split's current phase.
2. `agentpki.RevokeNode` with the reason `credentials_rotated` revokes every
   certificate, enrollment (unused credentials included) and forward link
   certificate of the node. The agent listener refuses revoked serials and
   open streams end at their next heartbeat (`agent_cert_revoked`); the
   rotation also drops this process's revocation cache after the commit.
3. `CreateEnrollmentTokenTx`, the code path of the enrollment token
   administration, issues the new credential. It comes after the revocation
   because the revocation also voids unused enrollment credentials.

Nothing dials the node, so rotating an offline node works and is the same
call. A repeated call revokes the previous credential and issues another: of
any number of calls, the last credential is the only valid one. No new
cryptography is involved: the credential is 32 random bytes from
`crypto/rand` with the `anixagt_` prefix, as before.

If any step fails, nothing changes.

## Audit

Two entries in `v2_operation_log` (module `agent_pki`), in the
administrator's name and with the client address, without any credential:

- `agent_credentials_rotate`: `node`, `enrollment_id`, `api_key_rotated`,
  `revoked_certificates`, `revoked_enrollments`, `revoked_link_certificates`
  and the `reason`;
- `agent_enrollment_token_issue`: as for any enrollment token.

The revoked certificates and enrollments carry `revoke_reason`
`credentials_rotated`.

## Notes for the UI

The node page's Credentials section (and a forwarding node's Agent card) call
this route from "Rotate credentials…": a danger confirmation with the reason
(counted in UTF-8 bytes, because the route's limit of 200 is `len(reason)` in
bytes), the lifetime and, for a proxy node, `rotate_api_key` (with the
consequence spelled out: without it the old key keeps working), then a dialog
that shows `credential` once, masked, with a copy button, the expiry with a
countdown, the `revoked` counts and whether the API key was replaced. It keeps
the credential only in that dialog's state and clears it on close. It shows no
`--reset` install command: that form is not in an Agent release yet, so the
dialog links to the guide instead. The console does not know the administrator
is a super administrator; it offers the action and shows the `403
super_admin_required` inside the confirmation.
