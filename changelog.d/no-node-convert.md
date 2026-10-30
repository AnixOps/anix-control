### Removed

- `POST /api/v4/kernel/nodes/forward-<id>/convert` is gone, with the rest of
  the proxy/forward split: there is nothing left to convert to. Node groups
  no longer take or show a `kind`; every group makes regular nodes, and one
  without protocols makes nodes that only run the network model (ANX).
