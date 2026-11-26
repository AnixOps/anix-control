### Fixed

- Mesh custom paths: a host group can no longer be created over a path's
  subnet target, which made the kernel refuse the source node's whole
  configuration. An explicit segment holding a target is refused, and an
  automatic segment skips it. Saving a path and creating a group take the
  same lock. A conflict that already exists is checked again at delivery:
  the path is left out and marked `path_target_in_host_group`.
- Deleting a device or removing a node from the mesh deletes every path
  that names it, so a node rejoining under the same identity does not
  inherit old paths. The audit records of both actions carry
  `paths_deleted`.

### Added

- Mesh custom paths: a waypoint without a static key in the admission
  list is marked `path_waypoint_not_admitted`. The node alert
  `paths_unsupported` is labelled. Subnet targets are IPv4 only, and the
  form now says so.
