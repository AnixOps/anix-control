### Security

- Shadowsocks 2022 keys are no longer only derived from guessable values. A
  user gets a random key at the first subscription pull
  (`v5_user_ss2022_key`, migration `0030_ss2022_user_keys`); users.v1 sends it
  to the nodes as extra `ss2022_key`, and Agents of protocol revision 2
  (5.0.2 and later) serve it as a second user next to the legacy
  UUID-derived key. Subscriptions offer the new key only at nodes whose Agent
  reported revision 2 over the mTLS stream (Control now stores each node's
  revision, `v4_kernel_agent_transport.protocol_revision`); other nodes keep
  the legacy key. Existing users are not forced to change anything: their
  configured clients keep working, and the next subscription pull switches
  them. A new root node's Shadowsocks 2022 protocol without a `server_key`
  gets a random one; existing nodes and child nodes keep the key derived from
  the node's creation time. A user who uses both keys on one node is counted
  twice by the kernel's per-user speed and device limits.
