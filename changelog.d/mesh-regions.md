### Added

- Intranet mesh regions and addresses (网络 → 组网, `/admin/mesh`). A new
  mesh starts with four regions: 中国大陆 (`cn`), 亚太 (`ap`), 欧洲 (`eu`)
  and 北美 (`na`); administrators add, change and delete regions. Each region
  is an independent network with forward secrecy, a generated network name
  and a private IPv4 subnet that no other region, the host group supernet or
  a reserved range overlaps, split into a server range and a device pool.
  Border nodes, gateways and servers get fixed addresses; a device gets an
  address from the device pool that it keeps until the address has gone
  unused for 30 days. Host groups take their segments from a separate
  supernet (`10.127.0.0/16`, `/24` by default) and are represented by one
  gateway and an optional standby gateway; virtual machines and devices in
  a gateway's local network are named by hand. The intranet names are
  `<name>.<region code>.mesh.internal` (suffix configurable), unique per
  region. A member's region change is a request that an administrator
  approves or rejects, or an administrator's own move; either gives the
  member a new address. Routes under `/api/v5/mesh` (administrators, written
  to the operation log); tables `v5_mesh_*` (migration 0045). The join
  credentials, the devices' own requests and the reports of machines without
  an Agent follow in a later release.
