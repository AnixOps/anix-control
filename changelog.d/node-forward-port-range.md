### Added

- The node page's 协议 tab has a 转发端口 section: the port range the forward
  planner allocates listen ports from on the node, and its reserved ports.
  It shows the range in force (the node's own, else the built-in
  30000–39999) and saving keeps it as the node's own and replans the routes
  through the node. Before, the range was only editable on the forwarding
  area's node page, which the node page no longer links to.
