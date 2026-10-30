### Added

- Manual Agent upgrade: **升级节点** has a 手动选择 mode that upgrades only
  the ticked nodes, at once in a single batch without the 30 minute
  minimum (a failed node is still rolled back). The upgrade plan lists the
  eligible nodes with their name and current Agent version, and
  `POST /api/v4/kernel/agents/upgrades` takes `"nodes"` for it.
