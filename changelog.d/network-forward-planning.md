### Added

- The network model planner takes forward routes (`networkmodel.Forward`:
  entry node, listen port, targets, policy, transports, egress) and plans
  each as a port forward on the entry device, with its transports planned
  once and shared with paths and other forwards that continue the same way.
  A forward that cannot be planned yet (broken chain, missing key, a
  transport that continues elsewhere, a taken port, a bad policy) is listed
  under `pending` with `forward` instead of failing the document.
  `sdk/network.CheckPortForward` checks a port forward on its own. Nothing
  feeds forwards in yet.
