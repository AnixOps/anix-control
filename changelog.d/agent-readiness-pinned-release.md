### Fixed

- The install readiness check (节点 → Agent 版本) checks the Agent release
  this Control was built with. Before, a Control-only release such as
  5.0.0-rc.14 looked for an Agent of its own number in
  `agent_install.artifact_dir` and warned to set `agent_install.agent_version`,
  although install commands and upgrade campaigns already used the pinned
  Agent release.
