### Added

- 节点 → 拓扑 (`/admin/topology`): every device and the lines between them,
  as a world map (全球地图) and as columns from entry to exit (抽象拓扑).
  Selecting a device or a line lights its path and shows its details. It is
  built from the node list, the forward routes and the egress routes, with no
  new API. A device's place is read from its name and tags (a region code or
  a city, such as `JP` or `tokyo`); a device nothing matches is listed under
  the map.
