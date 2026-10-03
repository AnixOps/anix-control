# Package reports

Status: Control side implemented (systemd services panel, PRs 1, 2, 5 and 6
of 7): the channel, the kernel store, the `machine-telemetry` route and
settings, and the node page's "服务" section. The Agent collector (PRs 3 and
4, anix-agent) and the cross-repository E2E (PR 7) follow.

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
  capability. `machine-telemetry` declares `telemetry.systemd.read` from
  its 4.1 release; the kernel refuses the systemd reports of older
  releases.

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

## Consumer: the machine-telemetry services table

The first consumer is the per-node systemd services table of
`machine-telemetry` (PRs 5 and 6 of the panel).

### Settings: off until enabled per node

The collector is off on every node. An administrator enables it for one node
in the node page's "服务" section, which saves the package's **Agent
installation** configuration through the kernel's plugin configuration API
(`PUT /api/v3/plugin-installations/:id/config`, with `expected_revision`).
The settings are the key `systemd_services` of that document:

```json
{
  "interval_seconds": 30,
  "systemd_services": {
    "nodes": {
      "12": { "enabled": true, "include": ["nginx*.service"], "exclude": ["*-debug.service"] }
    }
  }
}
```

- `nodes` maps a proxy node id, in decimal without leading zeros (1 to
  4294967295), to that node's settings; at most 4096 nodes.
- `enabled` (required in an entry) turns collection on. A node without an
  entry, or with `enabled: false`, collects nothing.
- `include` and `exclude` are optional lists of at most 32 globs each, at
  most 256 bytes, in `path.Match` syntax over the unit name alphabet. With
  includes, a unit must match one; it must match no exclude.
  `systemdreport.Selected` applies them, after the fixed rules
  (`.service` only, no `user@*` or `run-*`).

The manifest's `config_schema` describes the shape. When the configuration
of a release declaring `telemetry.systemd.read` is saved, the kernel also
parses it with `systemdreport.ParseConfig`, so a malformed glob is refused
with 422 (`invalid_plugin_configuration`).

**How it reaches the node.** Saving the Agent installation's configuration
queues `plugin.configure` for every node the package is assigned to
(`SyncAgentInstallationAssignments`). The operation's `payload_json` is the
`anixops.operation/v1` envelope whose `config` is the whole document. Every
node gets every node's entry; the Agent collector reads only its own, by the
node id of its Control stream: `systemdreport.ParseConfig(config)` then
`.Node(nodeID)`, and filters units with `NodeConfig.Selected`. Changing one
node's switch therefore re-pushes the configuration to all of the package's
nodes.

An Agent plugin binary that rejects unknown configuration keys (the
`machine-telemetry` plugin before the collector, anix-agent `c459383`)
fails to configure once `systemd_services` is present. Ship the 4.1 package
only with an Agent plugin that accepts the key, and enable the table only on
nodes running it: the node page shows the section only when the node's
assigned release declares `telemetry.systemd.read`.

### Route

`GET /api/v3/plugins/machine-telemetry/nodes/:id/services`, declared as the
control route `/api/v3/plugins/machine-telemetry/nodes/*`. The kernel
gateway requires an administrator with `machine-telemetry.api`; the package
host answers the route itself (it is not a compatibility route, so route
modes do not apply), accepts only `GET` and a decimal node id without
leading zeros, and again requires an administrator. The manifest declares
the permission `machine-telemetry.services.view` for the WebUI.

The host reads its Agent installation's settings through
`kapi_plugin_configuration_v1` and the node's report through
`kapi_package_report_v1`, sanitizes the payload again, applies the node's
current globs and counts the units:

```json
{"data": {
  "node_id": 12, "enabled": true, "include": [], "exclude": ["*-debug.service"],
  "reported": true, "supported": true, "unsupported_reason": "", "stale": false,
  "observed_at": "2026-10-03T11:55:00Z", "version": "4.1.0", "window_seconds": 600,
  "summary": {"total": 3, "failed": 1, "active": 1, "inactive": 1},
  "units": [
    {"name": "nginx.service", "active_state": "active", "sub_state": "running",
     "cpu_avg_percent": 1.5, "cpu_peak_percent": 12.25,
     "memory_bytes": 52428800, "memory_peak_bytes": 73400320}
  ]
}}
```

- A node that is not enabled answers `enabled: false` and no units, even
  with a stored report.
- `reported: false` means no report yet; `supported: false` comes with the
  collector's `unsupported_reason` (no systemd, cgroup v1).
- `summary` counts by `active_state`; units activating, deactivating or
  reloading count only in `total`.
- Errors use the kernel shape `{"error": {"code", "message"}}`: 404
  `not_found`, 405, 403 `forbidden`, 503 `storage_unavailable`.

### Node page

The node page (`/admin/nodes/:id?section=services`) shows "服务" when the
node's enabled `machine-telemetry` assignment is at a release whose manifest
declares `telemetry.systemd.read`. The section loads on demand. It has the
table (filter by name and by state, sortable name, state, CPU average and
peak, memory and memory peak), the totals line "总计 N | 失败 N | 每 10
分钟更新一次", a banner when the report is stale, the unsupported reason,
the "not enabled on this node" state with a switch, and the include /
exclude editor. It never starts, stops or restarts a unit.

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
  can read it. In the panel, administrators only.
