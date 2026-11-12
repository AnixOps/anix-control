### Changed

- Forward routes are stored as lines. Migration
  `0036_forward_routes_as_lines` moves every route of
  `v4_kernel_forward_route` into `v5_line` (the route stays as protojson in
  its `route` column) and drops the old table; the forward API reads and
  writes `v5_line`. After every accepted plan Control writes each route's
  line columns (name, entries, chain, exit, protocol) and the transports
  from the routes, so the line table always matches them. Rolling back to
  an earlier Control needs the database backup taken before the upgrade.
