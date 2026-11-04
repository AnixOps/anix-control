### Added

- Agent protocol revisions and version compatibility
  (`docs/reference/agent-versioning.md`). Agents name their protocol revision
  (`protocol.revision`) and the oldest anx-node they work with
  (`kernel.anx-node.min`) in the Hello; Control judges every session
  (compatible, warning, incompatible) in the transport inventory and the
  upgrade plan, logs problems once per session, and the node list shows a
  badge with the reasons.
- The release tag gate requires `config/AGENT_RELEASE` to share Control's
  major.minor from 5.0.0 on.

### Changed

- Control and the Agent number their patches independently since 5.0.0. An
  upgrade campaign accepts an Agent patch newer than Control's and refuses a
  target on another major.minor (`agent_version_line_mismatch`) unless
  `allow_version_line_mismatch` is set; a target on a newer line than Control
  is always refused.
