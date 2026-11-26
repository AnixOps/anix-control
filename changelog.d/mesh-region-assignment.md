### Added

- Mesh: region directory, preferred region and automatic region choice for
  kernel 0.4 (appendix D of the mesh protocol). A credential can name a
  `preferred_region`, and the mesh settings hold a default one and the
  responder port (migrations 0055, 0056): when the device's latencies
  choose the region, the preferred region's latency counts 0.8 times, ties
  go by sort then id. Devices read the directory with a credential (not
  consumed) or a device token (`GET /api/v5/mesh/device/directory`). The
  configurations carry the `region` object (current region, basis, directory,
  the probe responder of a border node) and the per-network `region_request`
  state, for the kernel capabilities `mesh_region_probe` and
  `mesh_region_request_state` (the basis is stored, migration 0057). A
  pending region request can be withdrawn (`DELETE
  /api/v5/mesh/device/region-change`, the Agent payload
  `{"withdraw": true}`). An administrator can add an Agent node with an
  automatic region (`/api/v4/kernel/mesh/auto-members`, migration 0058):
  its reported latencies place it. A node's region suggestion is kept and
  answered as `region_suggestion` in the member and device lists.
