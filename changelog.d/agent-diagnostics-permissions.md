### Fixed

- 诊断 on a proxy node could neither read nor restart the Agent: the Agent
  runs as `anixops-agent`, which may not read the journal ("No journal files
  were opened due to insufficient permissions") and which polkit let manage
  only `anixops-gost.service`. The installer now adds `systemd-journal` to the
  Agent unit's `SupplementaryGroups` (where the host has that group) and lets
  `anixops-agent` restart `anix-agent.service`. Re-run the node's install
  command to apply it to an installed node.
