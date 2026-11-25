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
