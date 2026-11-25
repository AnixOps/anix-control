### Added

- Intranet mesh addresses and names (组网, gated feature `mesh`). 节点 → 组网
  gains the sections 设备, 宿主组, 内网域名, 地区变更 and 设置:
  personal devices take an address from their region's device pool (the
  upper half of the range) that stays theirs until it has gone unused for 30
  days, and only then goes to the next device when the pool is full; host
  groups behind a gateway node take a segment of the host group range
  (10.127.0.0/16 by default, outside the regions) with an optional standby
  gateway of the same region; gateway nodes publish names for the hosts of
  their host groups and their local networks; the intranet name list shows
  every node, device and published name as `name.region-code.suffix` and
  marks duplicates; region change requests of nodes and devices are
  approved (the member moves and gets a new address) or rejected, and a
  direct move settles a pending request; the settings hold the domain
  suffix, the host group range and the reserved ranges no region may
  overlap. A gateway node moves or leaves only after its host groups and
  names change. Routes under `/api/v4/kernel/mesh/` (`summary`,
  `settings`, `devices`, `host-groups`, `device-names`, `names`,
  `changes`); tables `v5_mesh_settings`, `v5_mesh_device`,
  `v5_mesh_host_group`, `v5_mesh_device_name`, `v5_mesh_region_change`
  (migration 0046, back up the database before upgrading); audited in the
  operation log (module `mesh`).
