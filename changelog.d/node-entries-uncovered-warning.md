### Fixed

- The node page warns when a node's entries leave a protocol out of every
  subscription. A node with enabled entries is offered only through them, so
  an enabled protocol whose port no enabled entry offers (a NAT map written
  with the public ports as machine ports, as on leikwanhost-sh) disappeared
  from subscriptions while it still showed online. 入口 now lists those
  protocols above the table, and `GET`/`PUT /api/v4/kernel/nodes/{id}/entries`
  answer them as `uncovered_protocols`.
