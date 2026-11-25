### Added

- 组网 devices without an Agent, behind the gated feature `mesh`. An
  administrator issues a one-time enrollment credential (node or personal
  device, a region or the lowest latency, 1 to 168 hours; the token is
  shown once and only its SHA-256 is stored). The device enrolls with it at
  `POST /api/v5/mesh/device/enroll` and from then on, with its device
  token, reads its networks (`GET /api/v5/mesh/device/config`), sends a
  read-only status (`POST /api/v5/mesh/device/report`, answered 204) and
  asks to move to another region (`POST /api/v5/mesh/device/region-change`).
  Node devices get a fixed address from the server segment, personal
  devices one from the device pool (the upper half of the range), kept by
  the device key and released after 30 days unused. 节点 → 组网 lists the
  devices with their status, the pending region changes (approve, reject)
  and the credentials; a device can be moved or revoked. Tables
  `v5_mesh_enrollment`, `v5_mesh_device`, `v5_mesh_region_change`
  (migration 0046, back up the database before upgrading); audited (module
  `mesh`). Contract: `docs/design/mesh-regions/agent-contract.md`, section 4.
