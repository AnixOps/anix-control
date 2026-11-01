### Added

- The panel shows each node's anx-node kernel and can upgrade only the
  kernel. An Agent names the anx-node release it has installed in its
  Hello (label `kernel.anx-node`, Agents from the next release on); the
  transport inventory (`GET /api/v4/kernel/agents/transports`) answers it
  as `kernel_version` per node and per live session, and the upgrade plan
  answers the newest release Control holds (`kernel_version`) and each
  node's. The node list shows it under the Agent version, the node page in
  Agent connection with 升级内核, and the node menu has 升级内核 too.
- 升级节点 has a third mode, 只升级内核 (`POST /api/v4/kernel/agents/upgrades`
  with `nodes` and `kernel_only`): the target is the Agent release the
  ticked nodes run now, so each Agent keeps its binary and installs the
  newest anx-node release in `agent_install.artifact_dir`, then restarts
  once. The nodes must run one Agent release, and that release must still
  be in the artifact directory.

### Fixed

- A batch that rolls back no longer tells a node that already ran the
  target before the campaign (a kernel-only upgrade) to reinstate a
  previous Agent release.
