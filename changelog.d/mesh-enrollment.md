### Added

- 组网 devices without an Agent, behind the gated feature `mesh`. 节点 →
  组网 → 设备 issues one-time enrollment credentials (hostname, a region or
  the lowest latency, 1 to 168 hours; the token is shown once and only its
  SHA-256 is stored). A device enrolls with one at
  `POST /api/v5/mesh/device/enroll` and joins as a personal device with a
  pool address; with its device token it then reads its network
  (`GET /api/v5/mesh/device/config`), sends a read-only status
  (`POST /api/v5/mesh/device/report`, answered 204) and asks to change
  region (`POST /api/v5/mesh/device/region-change`). Every read or report
  uses the address. The device list shows the reported status; a token can
  be revoked. Tables `v5_mesh_enrollment`, `v5_mesh_device_token`
  (migration 0047, back up the database before upgrading); audited (module
  `mesh`). Contract: `docs/design/mesh-regions/agent-contract.md`, section 4.
