# Forwarding Security

Forwarding is high-risk because it can expose ports, execute relay-side changes,
and consume traffic quota. Treat the forwarding module as a control plane with a
separate execution plane.

## Trust Boundaries

The main boundaries are:

- Browser to panel API.
- Panel API to database.
- Panel API to NodeX.
- NodeX to relay gost API.
- Panel local executor to Ansible and SSH inventory.
- Clean agent to panel API.
- Runtime collector to internal traffic ingestion.

Each boundary needs its own authentication and least-privilege rules. A green UI
node status must not be treated as authorization or attachment proof.

## Authentication Controls

Current route-level controls:

- User forward routes require JWT authentication.
- Admin routes require JWT plus admin authorization.
- Internal traffic routes require application token authentication.
- Legacy `/flow/upload` requires application token authentication.
- Clean-agent register/heartbeat/report require an agent token.
- Clean-agent heartbeat/report can carry agent ID through body or `X-Agent-ID`.
- Clean-agent token management is admin-only.

Implementation requirements:

- Keep ownership checks in service methods, not only in router groups.
- Admin mirrors must not bypass user ownership rules accidentally when they call
  shared code.
- Do not accept agent-reported job results unless the job is claimed by that
  agent.
- Do not let revoked clean agents claim or report new work.

## Secret Handling

Sensitive values include:

- JWT signing secret.
- application token.
- NodeX runtime token.
- relay gost API token.
- clean-agent token.
- Ansible SSH credentials stored in inventory or environment.

Rules:

- Never log full tokens.
- Do not expose `ForwardNode.api_token` to user-facing routes.
- Admin responses should avoid returning secrets except at creation time when a
  one-time token is required.
- Clean-agent token creation may return the token once; operators must store it
  outside the panel response.
- Keep Ansible credentials out of `ForwardNode`. Use inventory and deployment
  secrets instead.

## Runtime Command Execution

Local Ansible mode runs external commands. The current implementation restricts
configured runtime commands to `ansible-playbook` or reviewed absolute
`ansible-playbook` paths.

Maintain these constraints:

- Do not run arbitrary command names from config.
- Do not concatenate shell strings.
- Pass arguments as structured `exec.CommandContext` arguments.
- Require context timeouts for runtime jobs.
- Keep environment allowlists small and documented.
- Treat inventory and playbook paths as privileged deployment configuration.

## SSRF And Network Reachability

Forwarding configuration can point at hosts and ports. Risks include SSRF,
metadata-service access, internal-network scanning, and relay abuse.

Current and required controls:

- Validate ports and protocols before creating runtime state.
- Keep port conflicts guarded by `ForwardPortBinding` and service-level wildcard
  overlap checks.
- Use explicit runtime backend configuration for NodeX base URL.
- Do not allow users to configure NodeX or relay management API endpoints.
- Consider denylisting metadata-service and loopback targets for user-controlled
  `remoteAddr` if product requirements do not require them.
- Keep gost API tests and runtime diagnostics admin-only.

## Authorization And Quota

User-controlled forwards must be constrained by grants:

- User must have an active `ForwardUserTunnel` for the selected tunnel.
- Grant must not be expired or disabled.
- Grant forward count limit must be enforced.
- User and tunnel traffic quotas must be checked when creating or resuming
  forwards.
- Traffic ingestion must pause affected forwards when quota is exhausted.

Security-sensitive behavior:

- Manual reset must not silently bypass quota enforcement expectations.
- Expired user-tunnel grants should pause affected forwards before being
  disabled.
- Force delete should remain visible as a risk-bearing admin or owner operation,
  not the default path.

## Clean Agent Security

Clean agent mode is a pull-based execution path.

Controls that must remain true:

- Agent tokens are generated from cryptographic randomness.
- Register, heartbeat, and report reject missing or invalid tokens.
- Revoked agents cannot heartbeat.
- Heartbeat only claims pending `clean_agent` jobs for the agent node.
- Report only updates a job claimed by the same agent.
- Negative traffic values are rejected.
- Agent install script must not embed a real token by default.

Recommended follow-up hardening:

- Store only a token hash for clean agents.
- Add token rotation.
- Add last-used metadata and admin audit entries.
- Add rate limiting around register and heartbeat.

## Data Leakage

Avoid exposing:

- user emails or UUIDs to other users
- relay API tokens
- clean-agent tokens
- NodeX tokens
- Ansible inventory paths where they reveal host structure
- raw runtime errors that contain secrets or command arguments

Runtime job `payload`, `result`, and `error` are operationally useful but can
also store sensitive details. Keep them admin-only and avoid copying them into
user-facing responses.

## Audit And Observability

Forwarding mutations should be auditable:

- create/update/delete forward
- pause/resume/force-delete forward
- tunnel grant assign/update/remove
- node create/update/delete/toggle/check
- runtime sync backend
- clean-agent token create/revoke

Current coverage is incomplete. Until full admin audit logging exists, runtime
job rows and database timestamps are supporting evidence, not a complete audit
trail.

## Availability Risks

Forwarding can create load on the panel, runtime backend, and relay host.

Controls and requirements:

- Clamp pagination on list endpoints.
- Keep runtime job polling bounded.
- Keep clean-agent heartbeat task limit bounded.
- Use context deadlines for network probes and runtime calls.
- Keep snapshot traffic updates serialized per forward to avoid double counting.
- Avoid unbounded goroutine creation in workers.
- Ensure background workers shut down on application context cancellation.

## CI And Test Expectations

Security-relevant forwarding changes need tests at the right layer:

- service tests for ownership, quota, grant expiry, port conflict, and runtime
  job idempotency
- handler tests for route auth and response shape
- clean-agent tests for invalid token, revoked token, mismatched claimed job, and
  negative traffic
- race tests for background workers and traffic counters
- `gosec` verification for command execution, token generation, and file/path
  handling
