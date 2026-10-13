### Added

- The first administrator claims the panel with a one-time link.
  `anix-control admin claim-link [-ttl 24h]` prints
  `https://<domain>/claim#anixclaim_…` on its terminal only (never a log),
  plus `https://<the machine's public address>/claim#…` while the panel's
  certificate is not publicly trusted yet. The link works once, for 24
  hours; running the command again replaces it. Only the token's SHA-256 and
  expiry are stored (`v5_control_meta`, key `admin_claim`). Once the panel
  has an administrator the command exits 1. Containers run it with
  `docker compose ... exec control /app/anix-control admin claim-link` or
  `kubectl exec ... -c control -- /app/anix-control admin claim-link`;
  `make claim-link` prints the development instance's link.
- The claim page `/claim` (no sign-in) reads the token from the URL
  fragment, which the browser never sends, removes it from the address bar,
  and posts it with the chosen email and password to the public,
  rate-limited `POST /api/v5/claim`: 201 creates the administrator, consumes
  the token in the same transaction and signs in; 404 once an administrator
  exists, 403 `claim_invalid`, 410 `claim_expired`, 422 `weak_password`
  (the account rule, 6 characters), 409 `email_taken`. Once identity holds
  the credentials the account is created through identity. The body is never
  logged; the audit row holds `admin_claimed`, the email and the source
  address. `GET /api/v5/claim/status` answers `{claimable}`, and the sign-in
  page says how to claim a panel that has no administrator yet.

### Changed

- No start creates an administrator any more, and an empty database no
  longer refuses to start without a password: a panel without an
  administrator logs one line, ``No administrator yet. Run `sudo
  anix-control admin claim-link` to get the claim link.``. An existing
  administrator is not affected.
- The systemd installer writes no administrator and no
  `.bootstrap-admin-password` file; after a successful install or update it
  prints the claim link while the panel has no administrator. Its
  `--admin-email` and `--admin-password` options are refused with the
  reason.
- Commands that print a result on stdout (`admin`, `module`, `agent`,
  `agents`, `forward`, `tls`, `doctor`) send the text SQL log to stderr.
- The Compose files, `control.env.example` and the Helm chart (0.5.0) no
  longer carry `ANIX_CONTROL_ADMIN_*`; their notes name the claim command.

### Fixed

- An empty `database.log_level` logs SQL errors only, like the built-in
  default `error`. A configuration file without a `database` section (the
  installer's) left it empty, and Control then logged every statement at
  info level (values were never logged).

### Removed

- `admin.email` and `admin.password` (and `ANIX_CONTROL_ADMIN_EMAIL`,
  `ANIX_CONTROL_ADMIN_PASSWORD`) are ignored with one warning each; remove
  them from the configuration (`docs/UPGRADE.md`, "The First Administrator
  Claims The Panel").
