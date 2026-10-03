# Package reports

Status: Control side implemented (systemd services panel, PRs 1 and 2 of 7).
The Agent collector, the `machine-telemetry` 4.1 route and the NodeDetail
"服务" section follow in later PRs.

A package report is the latest observation of one kind that an Agent plugin
package makes on a node: the per-node systemd services table of
`machine-telemetry` first, and the forward line's reports later. It is a
generic channel. A package does not own a protobuf message or a kernel
table; it owns a kind, which the kernel reviews and accepts.

## Contract

- **Wire.** `PackageReport { plugin_id, kind, version, payload_json,
  observed_at_unix_ms }`, `AgentToControl.package_report = 18`, sent only
  with `package-reports.v1` negotiated. The details are in
  `sdk/api/agent/v1/PROTOCOL.md`, "Package reports"; the generic limits
  (kind: lowercase dot-separated words, at most 64 bytes; payload: at most
  256 KiB) are in `sdk/agentcontrol`.
- **Latest value wins.** There is no acknowledgement, spool or resend. The
  Agent sends each new observation; Control keeps the newest one per node,
  plugin and kind.
- **Kinds.** The kernel accepts only the kinds it knows
  (`internal/service/package_report.go`, `packageReportKinds`), each with
  the manifest capability a reporting release must declare and a sanitizer.

  | Kind | Capability | Schema and sanitizer | Stale after |
  |---|---|---|---|
  | `systemd.services` | `telemetry.systemd.read` | `sdk/telemetry/systemdreport` | 25 minutes |

  A new kind, such as a forward report, adds a row here, an SDK schema
  package with its sanitizer, and an entry in `packageReportKinds`.
- **Authorization.** The node must have an enabled assignment of
  `plugin_id` at `version`, and that release must be a signed official
  Agent release whose manifest still verifies and declares the kind's
  capability. `machine-telemetry` will declare `telemetry.systemd.read`
  in its 4.1 release (PR 5 of the panel); until then the kernel refuses its
  systemd reports.

## Kernel storage

`v4_kernel_package_report_state`, created at startup, one row per node,
plugin and kind:

| Column | Type | Note |
|---|---|---|
| `node_kind` | varchar(16), key | `proxy` (forward nodes are not offered the capability yet) |
| `node_id` | integer, key | |
| `plugin_id` | varchar(120), key | |
| `kind` | varchar(64), key | |
| `version` | varchar(64) | the release that reported |
| `payload_json` | text | the sanitizer's re-encoding, never the bytes the Agent sent |
| `observed_at` | timestamp, indexed | from the Agent, or the receive time |
| `received_at` | timestamp, indexed | |
| `updated_at` | timestamp | |

A report replaces the row unless the stored one was observed later. The
table is a protected kernel table: no package can adopt it.

`NodeService.LatestPackageReport` reads a row with a computed `stale`
(older than the kind's limit, 25 minutes for `systemd.services`, at read
time). `sdk/telemetry/systemdreport.IsStale` is the same rule for packages.

Packages read through the kernel API view `kapi_package_report_v1`
(`kernel.view:kapi_package_report_v1`): `node_kind`, `node_id`,
`plugin_id`, `kind`, `version`, `payload_json`, `observed_at`,
`received_at`. On PostgreSQL it is a security-barrier view that shows a
package role only the rows whose `plugin_id` is its own package
(`anix_pkg_<id>`). SQLite has no roles: there the view shows every row,
and package storage on SQLite isolates nothing anyway.

## Refusals and metrics

A refused report is logged and dropped; the stream stays open.

- `anixops_agent_package_reports_total{result}`: `accepted`, `superseded`
  (a newer report was stored), `refused`, `unrecorded` (the database
  failed).
- `anixops_agent_package_reports_refused_total{reason}`:
  - `invalid`: `plugin_id`, `kind` or `version` is malformed;
  - `oversize`: the payload exceeds 256 KiB;
  - `future`: observed more than a minute ahead of Control's clock;
  - `unknown_kind`: a kind the kernel does not accept;
  - `not_assigned`: the plugin is not enabled on the node;
  - `version_mismatch`: the node is assigned another version;
  - `unsigned`: not an official release, no stored release, or its
    signature no longer verifies;
  - `missing_capability`: not an Agent release declaring the kind's
    capability;
  - `bad_payload`: the kind's sanitizer refused the payload.

## Privacy: the systemd services report

Owner decision H24 (2026-10-03). The services table is read-only and
opt-in: the collector is off on every node until an administrator enables
it for that node, with optional include and exclude globs per node.

- **Collected, per unit.** The unit name, `ActiveState`, `SubState`, the
  CPU use averaged over the last 10 minutes and its peak, the current
  memory and its peak. Per report: whether the node supports collection
  (systemd and cgroup v2) and, if not, a short reason.
- **Which units.** Only `.service` units. `user@*` user managers and
  `run-*` transient units are never reported, and scope units such as
  `session-*.scope` are not services. At most 512 units, names at most
  256 bytes.
- **Never sent.** `Description`, `ExecStart` or any other unit property,
  command line, environment, PID, path or log line. Control refuses a
  payload that has a `Description` or `ExecStart` field and drops every
  field it does not know, so only the fields above reach the database,
  whatever an Agent sends.
- **Retention.** Only the latest report per node is kept; there is no
  history. A report older than 25 minutes is shown as stale.
- **Who reads it.** Control and the reporting package (`machine-telemetry`)
  through `kapi_package_report_v1`; on PostgreSQL no other package's role
  can read it.
