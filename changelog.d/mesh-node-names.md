### Added

- Intranet mesh (组网, gated feature `mesh`): each region network of a
  node's configuration now carries the intranet names (`dns`: the suffix and
  every name as `name.region-code` with its address; duplicate names are
  left out and stay marked in the panel) and the host groups whose gateway's
  home is the region (`host_groups`: the segment and the instance ids of the
  gateway and the standby). The Agent's `mesh.status` may carry the node's
  `instance_id`; a host group is sent once its gateway has reported one, and
  a gateway's new instance id pushes the configuration at once. Changes to
  devices, host groups, names and the mesh settings now push the members'
  configurations too.
- Intranet mesh: a node asks for another region itself with the Agent's
  package report `mesh.region_request` (recorded as a member's pending
  change for the network manager), and a device whose pool address went to
  another device gets a new one (设备 → 分配新地址,
  `POST /api/v4/kernel/mesh/devices/{device_id}/address`, audited as
  `mesh_device_address`). The region list warns about an enabled region
  with fewer than two boundary nodes (`few_boundary_nodes`).

### Fixed

- Intranet mesh: a device's pool address now counts as used whenever a
  mesh node reports it among its peers, so only an address really unused
  for 30 days goes to another device. Every region has the same prefix
  length, a region code ends with a letter or digit and stays while the
  region has members or devices, and the domain suffix lies under
  `internal` or `home.arpa` and keeps full names within 253 characters.
  Node names become intranet names the way the node kernel normalizes them
  (lowercase, `xn--` Punycode for other than ASCII, other characters to
  hyphens) instead of being left out. Concurrent address allocations retry
  and end in a conflict instead of a server error; two decisions on one
  region change no longer both apply; an administrator's direct move
  records the administrator. Status reports of nodes that are in no region
  are refused.
