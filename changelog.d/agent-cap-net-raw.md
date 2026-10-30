### Fixed

- The Agent unit the install script writes adds `CAP_NET_RAW` to its ambient
  and bounding capabilities: ANX-Line's fake-TCP carrier (c3), which anx-node
  and the Agent's anx-server run, needs raw sockets and failed without it.
  Re-run the install script on a node to rewrite its unit.
