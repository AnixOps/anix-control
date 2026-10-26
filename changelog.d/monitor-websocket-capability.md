### Fixed

- The admin live monitor (Traffic & Monitoring → Live nodes) connects again
  instead of closing right after the handshake and reconnecting forever. A
  package WebSocket route's bridge capability was revoked as soon as the relay
  opened, before the package host could redeem it; it now stays live for the
  relay and is revoked when the relay ends.
