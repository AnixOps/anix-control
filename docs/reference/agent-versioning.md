# Agent Versions And Protocol Revisions

Since v5.0.0 Control and the Agent are released separately. This page is the
rule set; the code is `sdk/agentcontrol/compat.go` (the Agent's side is
`node/agent_control.go` in `anix-agent`).

## Version Numbers

- **One version line.** Control and the Agent share major and minor
  (`5.0`). A Control release pins an Agent release of its own line in
  `config/AGENT_RELEASE`; `config/scripts/check_release_version.py` (the tag
  gate) refuses another line from 5.0.0 on.
- **Independent patches.** Each repository numbers its patches on its own:
  Control `v5.0.3` may pin Agent `v5.0.1`, and Agent `v5.0.4` may ship
  without a Control release. A Control patch over an unchanged Agent keeps
  `config/AGENT_RELEASE` as it is (`prepare_release.py` without
  `--agent-tag`).
- **A new minor is cut together.** `5.1.0` of Control pins a `5.1.x` Agent;
  the Agent repository tags its `5.1.0` first.
- Hotfix tags (`.hf.N`) stay as they were for the pre-releases of 5.0.0; after
  a stable release a fix is the next patch.

## Protocol Revision

The protocol (`anix.agent.v1`, Hello `protocol`) stays the same string; inside
it a **protocol revision** counts the changes a side relies on. Capabilities
still negotiate each feature; the revision is the coarse number the panel
judges.

- The Agent names its revision in the Hello label `protocol.revision` (a
  decimal number). An Agent without the label (every Agent up to v5.0.0)
  speaks revision 1.
- Control serves the revisions from `MinAgentProtocolRevision` to
  `AgentProtocolRevision` (`internal/grpc/agent_control_server.go`, today
  1 to 3). Control stores each node's last revision
  (`v4_kernel_agent_transport.protocol_revision`) to decide what a
  subscription may offer at that node.
- Raise `agentcontrol.ProtocolRevisionCurrent` (and Control's maximum) in the
  pull request that adds a contract change an Agent relies on, and add a row
  below. The Agent raises its own revision when it implements the change.
  Raise Control's minimum only when Control drops support for an older
  revision, never in the same release that introduces the newer one, so
  nodes can still be upgraded over the stream.

| Revision | Introduced by | Contents |
| --- | --- | --- |
| 1 | v5.0.0 | The 5.0.0 contract (every Agent before the label). |
| 2 | v5.0.2 | users.v1 extra `ss2022_key` (`agentcontrol.UserExtraSS2022Key`): the user's stored Shadowsocks 2022 key, std base64 of 32 bytes (the 128-bit ciphers use the first 16). The Agent serves it as a second Shadowsocks 2022 user next to the legacy key; subscriptions offer it only at nodes whose Agent speaks revision 2 or later over the mTLS stream. |
| 3 | next release | Node type `snell` in the configuration answers (`agentcontrol.ProtocolRevisionSnell`): `psk`, `version` and, when set, `obfs` and `mode`; a user's key is their UUID. An older Agent answers `config_invalid` for a Snell protocol and keeps running its other protocols, so upgrade the Agent before adding one. The kernel's snell inbound came after anx-node v0.2.0 and is in no release yet; `KernelMinVersion` stays as it is until one exists. |

## Kernel

The Agent names the oldest anx-node release it works with in the Hello label
`kernel.anx-node.min`, next to the installed one (`kernel.anx-node`). anx-node
keeps its own version numbers (`0.1.0-alpha.15`); Control only compares them.

## What Control Does With It

`agentcontrol.CheckCompatibility` judges every live session:

| Problem | Severity | Fix |
| --- | --- | --- |
| `agent_protocol_too_old`: revision below Control's minimum | incompatible | Upgrade the Agent. |
| `agent_protocol_too_new`: revision above Control's maximum | warning | Upgrade Control; the newer features stay off. |
| `version_line_mismatch`: another major.minor | warning | Upgrade the Agent to a release of Control's line. |
| `kernel_too_old`: anx-node below the Agent's minimum | incompatible | Upgrade the kernel (kernel-only campaign). |

- An incompatible Agent still connects, so an upgrade campaign can reach it;
  Control logs `agent version compatibility` once per session.
- `GET /api/v4/kernel/agents/transports` returns `control` (version, version
  line, Agent release, protocol revisions), and per node `protocol_revision`
  and `compatibility`; `summary.incompatible` and
  `summary.compatibility_warnings` count them. The node list shows a badge
  next to the Agent version, with the reasons as its hint.
- `GET /api/v4/kernel/agents/upgrade-plan` returns the same `control`, per node
  `compatibility`, and `target_version_line_mismatch`.
- `POST /api/v4/kernel/agents/upgrades` refuses a target on another
  major.minor (`agent_version_line_mismatch`) unless
  `allow_version_line_mismatch` is set (a deliberate rollback), and always
  refuses a target on a newer line than Control. A newer patch than
  Control's is fine.

## Releasing

- Control release with a new Agent: tag the Agent first, then
  `prepare_release.py <control> --agent-tag <agent>` and
  `bump_agent_ref.py <agent-sha> <agent-tag>` (`docs/RELEASING.md`).
- Control-only patch: `prepare_release.py <control>`; the Agent pin stays.
- Agent-only patch: release the Agent; nodes get it once a Control pins it
  (`config/AGENT_RELEASE`) or an operator sets `agent_install.agent_version`
  and copies the release into `agent_install.artifact_dir`. The upgrade
  campaign accepts any release of Control's line.
