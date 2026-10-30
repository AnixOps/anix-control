### Added

- The node page's 网络 settings offer the interfaces the node's Agent
  detected: pick the egress interface, and its source addresses and
  gateways fill in, instead of typing them (手动填写 still types them).
  `GET`/`PUT /api/v4/kernel/nodes/{id}/network` answer `detected`
  (`name`, `addresses_v4`, `addresses_v6`, `gateway_v4`, `gateway_v6`) and
  `detected_at`, from the interfaces in the Agent's network status (Agent
  5.0.0-rc.20 or later).
