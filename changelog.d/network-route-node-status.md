### Fixed

- A route's detail page no longer shows "未上报" for a hop node that runs the
  route on the network model but never joined the forward inventory (for
  example an exit device of an ANX route). `GET /api/v4/forward/nodes` now
  also lists the proxy nodes that have a network model device, with the
  reports of their network status.
