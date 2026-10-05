# Kernel Alerts: Certificate Expiry And Stuck Phases

Control watches the things that fail slowly and silently: a certificate whose
holder stopped renewing it, a CA that is about to end, and a rollout that was
started and never finished. The **alert monitor** (`internal/kernelalerts`)
scans them every 15 minutes, keeps one row per alert in `v4_kernel_alert`,
sends the administrators one digest of what needs them, and lists the rows at
`GET /api/v4/kernel/alerts`. It only reads the certificate stores and the
phase tables: it never renews, revokes or advances anything.

## Alert Kinds

| Kind | Subject | Raised when | Clear it by |
|------|---------|-------------|-------------|
| `agent_certificate_expiring` | `proxy-<id>` / `forward-<id>` | the node's latest Agent client certificate is inside its [window](#thresholds) and the Agent did not renew it, or it ended and the node is still enabled | bringing the Agent back (it renews itself while its certificate is valid). After the certificate ended the Agent cannot connect: [enroll it again](../guide/agent-onboarding.md#when-an-agent-certificate-was-not-renewed) with a new install command (`--reset`) |
| `link_certificate_expiring` | `forward-<id>` | the same for the forward link certificate (H28), which renews with the Agent certificate | the same: a connected Agent renews both |
| `module_certificate_expiring` | `<package>#<enrollment>` | a module's latest certificate (per enrollment) is inside its window or ended | starting the module again or enrolling it again; revoking its enrollment ends the alert |
| `ca_expiring` | `service_ca:<key id>` / `forward_link_ca:<key id>` | the **current** module (Agent and kernel) CA or forward link CA ends within `alerts.ca_expiry_days` | rotating the CA (`anix-control module ca rotate` or `POST /api/v4/kernel/modules/ca/rotate`; `anix-control agent link-ca rotate` for the link CA): the new CA is trusted at once and takes over signing by itself after one certificate lifetime |
| `node_secrets_split_stalled` | the legacy table (`v2_node`, ...) | a split table is in `dual_write` or `dual_read` and was not touched for `alerts.phase_stuck_after` | `anix-control node-secrets verify`, `phase <table> dual_read`, `finalize -confirm <table>`; or accept it and set `alerts.phase_stuck_after: "0"` |
| `node_secrets_finalize_interrupted` | the legacy table | a finalize started (phase `finalized`, no `finalized_at`) over an hour ago and did not complete | run `anix-control node-secrets finalize -confirm <table>` again: it resumes |
| `identity_import_stalled` | `authority` | the identity authority sits in `importing` with no progress for `alerts.phase_stuck_after` | resume the import or start the cutover (`POST /api/v4/kernel/identity/import`, `/cutover`) |
| `identity_cutover_not_finalized` | `authority` | identity owns logins (state `identity`) since the latest cutover event, and the legacy credentials were not finalized for `alerts.phase_stuck_after` | `POST /api/v4/kernel/identity/finalize` (allowed a day after the cutover), or roll back |

An alert resolves by itself on the next scan after its cause is gone (a new
certificate exists, the CA was rotated, the phase moved on, the node was
disabled or deleted). Resolved alerts stay listed for 30 days.

### Thresholds

A certificate is only reported when its holder **missed** a renewal, because
healthy holders renew at two thirds of the lifetime (`RenewAfter`). A fixed
"14 days before the end" would fire for every fresh 7-day Agent certificate.
So the window of a leaf certificate (Agent, link, module) is

```text
window = min(alerts.leaf_expiry_days, one sixth of the certificate's own lifetime)
```

For the default lifetimes that is 28 hours for the 7-day Agent and link
certificates and 4 hours for the 24-hour module certificates;
`leaf_expiry_days` (default 14) only matters when you lower it below that.
The lifetime is `not_after - created_at` of the certificate record. Only a
node's **latest not-revoked** certificate counts: a node that renewed is
healthy whatever its old certificates say. Revoked certificates, disabled
nodes and deleted nodes are skipped (their credentials are revoked or
revocable; an expiry there is expected). A certificate whose record was
pruned (Agent and link records are kept for a day after they ended) no longer
alerts.

CAs alert when the **current** CA is within `alerts.ca_expiry_days` (default
60) of its end; next and retired CAs do not. The detail says whether a next CA
is already staged.

Severity: `critical` when the certificate or CA already ended or is inside the
last quarter of its window (7 hours for an Agent certificate, 15 days for the
default CA window), `warning` before. The phase alerts are always `warning`.

### What "stuck" means for the phases

Only processes with a reliable timestamp in their own rows are checked:

- **Node credential split** (`v4_kernel_node_secret_split`): the row has no
  "entered the phase at" time. `updated_at` moves with every backfill,
  verification and phase change, so the alert is "**not touched** for
  `phase_stuck_after`". It never fires early; it can fire late for a table
  somebody still verifies every hour. A started finalize writes the row once
  (its `updated_at` is its start), so an interrupted finalize is exact.
  `dual_write` is the state every install starts in, so a fresh install with
  nothing to move gets the reminder too: finalize the tables or set
  `phase_stuck_after: "0"`.
- **Identity authority** (`v4_kernel_identity_authority`): while `importing`,
  every batch saves the row, so `updated_at` is the last progress. For
  `identity`, the cutover time is the latest `cutover` event in
  `v4_kernel_identity_cutover`; the row's `updated_at` is also written by a
  rollback and an abort. Without a cutover event no alert is raised.
- **Route modes are not checked.** `legacy`, `shadow` and `native` are
  operating modes, not phases of one process: there is no final one, `shadow`
  is a validation period with its own mismatch gate, and a route's current
  mode lives in the signed package configuration, not in a table the monitor
  can read. The revision history (`v4_kernel_route_mode_revision`) has the
  change times, but cannot say what the current mode is.

## Notifications

Each scan sends **one digest** to every administrator who is not banned, for
the alerts that are due: new, worse (warning to critical, told at once), or
due again.

| Channel | When |
|---------|------|
| in-app | always: an entry in the administrator's notifications (`v2_notification_log`, event `system.alert`) |
| e-mail | when the notification e-mail settings are configured |
| Telegram | for administrators who bound a Telegram account (the bot must be configured) |

An alert is announced once per subject. It is announced again after
`alerts.renotify_interval` (default 24 hours); a critical alert after a
quarter of that (6 hours), a phase alert after seven times it (a week). A
finding that disappears and comes back inside the interval is not announced
again. A digest lists at most 20 alerts and 3,500 characters (critical first)
and points to the list for the rest. If no channel could deliver, the alerts
stay due and the next scan tries again.

The Telegram copy of a digest has the characters `_ * ` [ ]` replaced
(`v2_node` reads `v2-node`): Telegram's legacy Markdown mode refuses a whole
message that has one of them unpaired. The in-app and e-mail copies keep the
text as it is.

Messages and rows hold no secret: the alert kind, the node's id and name,
dates, phase names and counts. Never serials, enrollment ids, keys or
checkpoints. A node name has the Markdown characters `_ * ` [ ]` replaced in
the message (the `detail` keeps the name as stored).

## API

`GET /api/v4/kernel/alerts`, administrators, read-only (`Cache-Control:
no-store`).

| Query | |
|-------|---|
| `status` | `active` (default), `resolved` or `all` |
| `kind` | one kind from the table |
| `severity` | `warning` or `critical` |
| `limit` | 1 to 500, default 100 |

```json
{
  "data": {
    "alerts": [
      {
        "id": 7,
        "key": "agent_certificate_expiring/proxy-12",
        "kind": "agent_certificate_expiring",
        "severity": "warning",
        "status": "active",
        "subject_kind": "node",
        "subject": "proxy-12",
        "message": "The Agent certificate of node proxy-12 (hk-1) ends at 2026-10-08T10:00:00Z and the Agent has not renewed it. ...",
        "detail": {"node": "proxy-12", "node_name": "hk-1", "not_after": "2026-10-08T10:00:00Z", "expired": false, "lifetime_hours": 168, "window_hours": 28},
        "expires_at": "2026-10-08T10:00:00Z",
        "first_seen_at": "2026-10-07T09:00:00Z",
        "last_seen_at": "2026-10-07T12:15:00Z",
        "last_notified_at": "2026-10-07T09:00:00Z",
        "notify_count": 1,
        "resolved_at": null
      }
    ],
    "summary": {"active": 3, "critical": 1, "warning": 2}
  }
}
```

Critical alerts come first, then the soonest to end (or first seen). The
summary counts every active alert whatever the filters. `message` is English
for notifications; a UI composes its own text from `kind` and `detail`.

**In the admin console.** The dashboard's **Needs attention** list
(`DashboardAlerts.vue`) merges `GET /api/v4/kernel/alerts?status=active` with
the items it builds in the browser: one item per alert (icon by kind, tone
`danger` for `critical` and `warning` otherwise, critical first, a link to
`/admin/nodes/<id>` for `proxy-<id>` subjects and to the forward node page for
`forward-<id>`, text composed from `kind` and `detail` in both languages, dates
in the viewer's locale), and a count badge from `summary`. **Resolved** shows
`status=resolved` (`limit=30`). The alerts load apart from the dashboard: a
failure is one item with **Try again**. Nothing else changes: there is no write
endpoint, alerts clear themselves.

## Configuration

| Key | Environment variable | Default |
|-----|----------------------|---------|
| `alerts.enabled` | `ANIX_CONTROL_ALERTS_ENABLED` | `true` |
| `alerts.check_interval` | `ANIX_CONTROL_ALERTS_CHECK_INTERVAL` | `15m` (at least `1m`) |
| `alerts.leaf_expiry_days` | `ANIX_CONTROL_ALERTS_LEAF_EXPIRY_DAYS` | `14` (the cap, see [thresholds](#thresholds)) |
| `alerts.ca_expiry_days` | `ANIX_CONTROL_ALERTS_CA_EXPIRY_DAYS` | `60` |
| `alerts.renotify_interval` | `ANIX_CONTROL_ALERTS_RENOTIFY_INTERVAL` | `24h` (at least `1h`) |
| `alerts.phase_stuck_after` | `ANIX_CONTROL_ALERTS_PHASE_STUCK_AFTER` | `72h` (at least `1h`; `0` turns the phase alerts off) |

The monitor runs in the one Control process that holds the singleton-worker
lease, so a second process does not send a second digest. The table
`v4_kernel_alert` is new; no existing table changes. It is created at start
like the other kernel tables.
