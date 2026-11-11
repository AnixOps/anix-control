### Added

- Node protocol type `snell`: Snell served by the Agent's kernel, on TCP with
  the protocol's own encryption (no TLS, no transport). A protocol has one
  PSK (`settings.psk`, generated in the admin form; 12 bytes or more for
  version 6) plus `version` (1 to 6, default 4), `obfs` (`http`, versions 1
  to 4) and `mode` (version 6). Each user logs in with their UUID as the user
  key, so traffic stays per user and an expired user is locked out. The
  add-protocol picker has a Snell template.
- The Surge subscription lists Snell nodes (`psk`, `version`, `userkey`); the
  other formats leave them out. `userkey` is not in Surge's manual and has
  not been checked with a real Surge client yet.
- Agent protocol revision 3: the `snell` node type. Upgrade the Agent of a
  node before adding a Snell protocol to it; an older Agent answers
  `config_invalid` for that protocol and keeps running the others
  (`docs/reference/agent-versioning.md`). The node also needs an anx-node
  with the snell inbound, which no anx-node release has yet.
