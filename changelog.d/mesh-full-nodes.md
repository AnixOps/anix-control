### Added

- 组网: an enrollment credential of the role 完整节点 enrolls a router or server without an Agent as a full mesh node. It gets a fixed address of its region's server segment (given on the credential and kept until it is used, or the lowest free one), which is never reclaimed, listens on TCP and UDP 11010, and advertises only the local subnets an administrator confirms in the device list (`PUT /api/v4/kernel/mesh/devices/:device_id/subnets`, audited as `mesh_device_subnets`; migration 0048).
