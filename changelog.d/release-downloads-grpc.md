### Added

- `releases.v1`: `AgentArtifacts.DownloadRelease` streams the Agent and
  anx-node files of an `agent.upgrade` operation from
  `agent_install.artifact_dir` over the session's mTLS gRPC connection, in
  1 MiB chunks from any offset, so a node resumes a cut download and makes
  no separate https request to Control. The operation keeps its https URL
  for Agents without the capability.
- The Agent installer takes `--install-name NAME` (also for `uninstall`):
  the account, units, directories, polkit rule, sysctl drop-in and binaries
  are all called NAME (the kernel `core` and `core-server`), the unit runs
  the binary without arguments, and the files it writes carry no comments
  or links naming AnixOps. The Agent derives the same layout from its path.

### Changed

- `install.sh --offline` makes no https request to Control: it no longer
  reads `/install/agent.env` or `/install/ip` nor checks Control's https
  address; the node only opens the gRPC connection.
