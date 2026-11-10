### Added

- ANX nodes have two handshake generation settings (admin node editor, ANX
  protocol): `wire_gen` and `min_wire_gen`, each 1 or 2, empty = 1. With
  `wire_gen` 2 the subscription publishes the node's server public key with
  the suffix `@2`, so clients log in with the second handshake generation
  (they never fall back to the first). With `min_wire_gen` 2 the node config
  carries `min_wire_gen: 2` and anx-node refuses first-generation logins;
  `min_wire_gen` 2 needs `wire_gen` 2. Both stay out of the node config and
  the key text until set, so nodes and clients older than the second
  generation are unaffected. Switch a node only after its anx-node and all
  of its clients know the second generation.
- Control logs a warning when two devices report the same ANX server public
  key (`network model: devices share a server key`): every server is meant to
  have its own key, so that a login recorded at one cannot be replayed at
  another.
