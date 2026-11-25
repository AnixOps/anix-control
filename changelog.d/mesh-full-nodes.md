### Added

- 组网: an enrollment credential of the role 完整节点 enrolls a router or server without an Agent as a full mesh node. It gets a fixed address of its region's server segment (given on the credential and kept until it is used, or the lowest free one), which is never reclaimed, listens on TCP and UDP 11010, and advertises only the local subnets an administrator confirms in the device list (`PUT /api/v4/kernel/mesh/devices/:device_id/subnets`, audited as `mesh_device_subnets`; migration 0048).

### Changed

- 组网: an intranet name that more than one entry has goes to the nodes with each of its addresses (R28); the kernel answers them all and marks the name. Name normalization replaces combining marks too and encodes a node's name whole before cutting it to 63 bytes. A reserved range (an old network during a migration) has the regions' prefix length. A region change request to a disabled region is refused, and approving one whose region was disabled since is refused.

### Fixed

- 组网: two region change requests for one member at once no longer both stay pending on PostgreSQL, and a direct move settles only changes still pending.
