### Added

- `releases.v1`: `AgentArtifacts.DownloadRelease` streams the Agent and
  anx-node files of an `agent.upgrade` operation from
  `agent_install.artifact_dir` over the session's mTLS gRPC connection, in
  1 MiB chunks from any offset, so a node resumes a cut download and makes
  no separate https request to Control. The operation keeps its https URL
  for Agents without the capability.
