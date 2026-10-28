### Added

- The device network model on Control (`internal/networkmodel`,
  migration 0016): an administrator declares lines on nodes, user and hop
  ingresses, internet and hop egresses, ANX-Line transports between
  devices and paths as one document (`GET`/`PUT /api/v4/kernel/network`,
  with an optional revision check). Every device's `NetworkPlan` must pass
  the planner (sdk/network checks plus the cross-device rules) before the
  document is stored. Each ANX-Line transport gets a random link key
  sealed under `module_runtime.ca_kek`
  (`POST /api/v4/kernel/network/transports/{id}/rotate-key` rotates it);
  devices get a new plan generation when their plan changes, and
  `GET /api/v4/kernel/network/devices/{id}/plan` shows a plan with the
  keys redacted. Paths wait (listed as pending) until the accepting device
  reports its ANX-Line server key. Plans do not reach Agents yet.
