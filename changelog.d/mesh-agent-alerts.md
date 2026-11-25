### Added

- 组网: node status names two more reasons the Agent holds a network back: the node kernel does not take the admission list, and the network's kind is neither managed nor legacy.

### Fixed

- 组网: a static key a node reports is not taken when a device or another node has it, so revoking that device always takes its key off the admission list.
