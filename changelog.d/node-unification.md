### Changed

- One machine is one node: a relay is a regular node without protocols
  (`docs/design/node-unification`). Node groups make regular nodes (a
  group without protocols makes relays; existing forward groups are
  migrated), new forward nodes are Ansible hosts only, and the install
  command of a relay carries `--forward`, which turns forwarding on for the
  regular node.

### Added

- `POST /api/v4/kernel/nodes/forward-<id>/convert` turns an Agent forward
  node into a regular node without protocols, moving its route hops, ports
  and DNS rows in one step (dry run first; super administrators only). The
  forward Nodes page offers it as "Make a regular node" and then shows the
  new node's install command.
