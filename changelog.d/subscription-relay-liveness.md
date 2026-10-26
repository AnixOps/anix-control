### Fixed

- Subscriptions offer a relay node (a node with a parent) again while the
  landing node it forwards to is online. A relay runs no Agent and never
  heartbeats, so every relay was dropped as offline.
