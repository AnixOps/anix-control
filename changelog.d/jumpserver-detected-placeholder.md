### Added

- The JumpServer workflow's `control` action fills
  `@@DETECTED:<node>:<interface>:<field>@@` in `api_body` with an address or
  gateway the node's Agent detected, masked, so a node's network settings can
  be set from the public workflow without typing addresses.
