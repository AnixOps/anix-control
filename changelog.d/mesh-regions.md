### Added

- Intranet mesh regions (组网), behind the gated feature `mesh` (dev builds
  only until the node kernel runs the mesh). 节点 → 组网 lists the regions
  (four defaults: 中国大陆 `cn` 10.150.0.0/16, 亚太 `apac` 10.151.0.0/16,
  欧洲 `eu` 10.152.0.0/16, 北美 `na` 10.153.0.0/16, all disabled) and the
  nodes in them. A node gets the lowest free fixed address of its region's
  server segment (the lower half of the range); a boundary node gets an
  address in every region. The desired configuration of a node in an enabled
  region carries the member `mesh` (its networks, peers and advertised
  ranges; forward secrecy always on), and every change pushes it at once.
  The node's Agent reports `mesh.status` on `mesh.v1`, shown with its
  alerts and a lost control link after three minutes. Routes under
  `/api/v4/kernel/mesh/`; tables `v5_mesh_region`, `v5_mesh_member`,
  `v5_mesh_status` (migration 0045, back up the database before upgrading);
  audited in the operation log (module `mesh`). Contract with the Agent:
  `docs/design/mesh-regions/agent-contract.md`.
