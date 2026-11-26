### Added

- Mesh: custom paths. An administrator can define, for one node or full
  node device of a region, the path to a peer or a private subnet through
  1 to 6 waypoints (`/api/v4/kernel/mesh/paths`, section 自定义路径 of the
  组网 page). Paths are stored by member and delivered in each network's
  `routing.paths` with the members' current addresses; a path whose member
  no longer resolves is left out and flagged, and a device without an Agent
  gets paths only when its kernel lists `mesh_paths`. The list shows the
  state the source reports for each path.
