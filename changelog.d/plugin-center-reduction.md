### Added

- 系统设置 → 内置包 (`/admin/system/packages`) shows the built-in packages:
  each one's name, version, state and last error, a confirmed host restart,
  the settings of a package that declares them, and the WebUI extension load
  errors. It replaces 插件中心: `/admin/plugins` and
  `/admin/plugins/route-modes` redirect to it, and the sidebar's 扩展 group
  now holds only the packages' own WebUI menus.

### Changed

- Route modes come only from the rehearsed list
  (`config/package-route-defaults.json`): a listed route runs natively once
  the installed package is at least the listed release, identity group A
  follows the identity authority, and every other route runs its legacy
  handler. The first start runs the `0004_drop_route_modes` migration: it
  drops `v4_kernel_route_mode_revision` and
  `v4_kernel_shadow_mismatch_sample`, removes the stored route-mode map (the
  `routes` key) from every Control package configuration in a new revision,
  and writes each removed map to the audit log first (action
  `route_modes_removed`). A route pinned to `legacy` that the list names runs
  natively again; a route switched to `shadow` or `native` that the list does
  not name runs its legacy handler. A configuration `PUT` that sets `routes`
  is refused. The identity cutover and rollback move only the authority.
- Agent plugin dispatch and topology execution are off until M4 with no
  setting (`service.AgentPluginDispatchEnabled`,
  `service.TopologyExecutionEnabled`); the code paths stay. Node assignments
  and Agent configuration changes record the desired state without an Agent
  operation, and deployment apply and rollback answer
  `409 topology_execution_disabled`. The built-in packages' hosts always
  run.
- `plugins.control_execution_enabled`, `plugins.dispatch_enabled`,
  `plugins.dispatch_poll_interval`, `plugins.topology_execution_enabled`,
  `plugins.topology_poll_interval` and `package_routes.default_mode` (and
  their `ANIX_CONTROL_*` variables) are no longer read. A configuration that
  still sets one loads and logs `Ignoring the removed configuration key ...`
  once per key. `scripts/install.sh` no longer writes
  `control_execution_enabled` and drops the five `plugins.*` lines from an
  existing `config.yaml`.
- `POST /api/v3/plugin-installations/:id/actions` accepts only
  `"action":"restart"`; any other action answers
  `400 unsupported_plugin_action`.
- The live Control WebUI gate builds the signed built-in packages it needs
  into a temporary Control (`go build -overlay`) instead of importing them,
  and the frontend job no longer checks out the Agent. The cross-repository
  Agent suite writes its packages into the database the way the bootstrap
  does, and its Agent plugin scenarios skip while dispatch is off.

### Removed

- The plugin center's import, install, update, rollback, enable and disable
  (`POST /api/v3/plugin-releases`, `POST /api/v3/plugin-releases/:id/artifact`,
  `PUT /api/v3/plugin-installations`) and the route-mode administration
  (`GET` and `POST /api/v4/kernel/route-modes`, `/rollback`, `/revisions`,
  `/mismatches`): the routes answer `404`. Control installs, upgrades and
  enables its built-in packages at every start.
- `anix-control routes` (`list`, `set`, `rollback`, `history`): running it
  exits with status 2 and says why.
- The shadow mismatch sample collector (`internal/shadowsamples`), the
  route-mode page, the release import, installation and detail dialogs, and
  the staging rehearsal (`scripts/staging`, its guide and the Staging
  Rehearsal Smoke job).

### Fixed

- `.gitignore` no longer hides new source files under
  `packages/<id>/control/`: only the binary a `go build` leaves there is
  ignored.
