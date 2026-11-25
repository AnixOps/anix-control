### Added

- Mesh: external network bridges (R36). A node of the mesh can advertise
  chosen addresses and subnets of another overlay network in its home
  region (`/api/v4/kernel/mesh/bridges`): its external interface, the
  routes (single addresses also inside 100.64.0.0/10 and
  fd7a:115c:a1e0::/48, private subnets outside them and outside the
  mesh's own ranges) and the members, devices and host groups allowed
  through it. The node gets the section `external_bridge` with the allow
  list expanded to current addresses. An address or segment that leaves
  an allow list is not given out again until the bridge node reports a
  list without it applied (migration 0051).

### Fixed

- Mesh: a bridge node's report without `externalBridge` no longer releases
  the addresses held for it. Only an explicit `null` observed after the
  bridge was removed shows the node no longer bridges; a missing value is
  unknown, as the kernel may still bridge with the old allow list. A
  report with the alert `external_bridge_unsupported` and no
  `externalBridge` counts as a node running no bridge.
  Two consecutive reports received after the removal must both show the
  release, so a late or out-of-order report cannot release alone.
  A node's report observed before the stored one is dropped, and reading
  the previous report, storing the new one and releasing holds happen
  under one lock.
