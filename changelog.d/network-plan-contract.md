### Added

- Draft device network contract `anixops.network.v1`
  (`sdk/api/network/v1`, docs/design/network-model): the per-device
  `NetworkPlan` (lines, ingresses, egresses, transports with ANX-Line link
  settings, paths, relay forwarding), the `DeviceCapabilities` an Agent
  offers with the capability `network.v1`, and its `NetworkStatus`.
  `sdk/network` carries them over the Agent Control stream (Hello
  attribute, nodeconfig member `network`, PackageReport), checks plans
  against the planner invariants, hashes and stamps them. Golden plans for
  the Agent and the kernel live in `contracts/network/v1`. Control does not
  serve it yet.
