### Fixed

- A freshly installed Agent now gets anx-node. The Agent installer only
  installed the Agent, and anx-node came only with an upgrade campaign, so a
  new node had no core for its protocols: every configuration failed with
  "the node type is not support" and the node showed as never reported.
  `/install/agent.env` now names the anx-node release Control holds in
  `agent_install.artifact_dir` (`kernel anx-node <version>`), and the
  installer downloads it from Control, checks the release's SSH signature
  (`ssh-keygen -Y verify`, the anx release key) and the SHA-256s, and
  installs `anx-node`, `anx-server` and `anx-node.json` exactly as the
  Agent's updater does. A host that already has a kernel keeps it; any
  failure leaves the Agent installed and says how to get the kernel later.
