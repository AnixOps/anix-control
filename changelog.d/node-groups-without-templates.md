### Changed

- Node groups no longer have a kind, a protocol template or subscription groups. A machine that joins a group is a clean node without protocols (migration `0020_node_groups_without_templates`); the group keeps its token and the defaults its nodes copy. The join dialog's quick-create form asks only for a name, and the old forward-node compatibility in groups is gone.
