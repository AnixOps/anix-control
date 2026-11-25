### Changed

- Mesh: the last usable address of each region's range (for example
  `.255.254` of a /16) is the region's fixed resolver address (spec
  section 15.9.2, item 8) and is given to no node, so the device pool is
  one address smaller. The region list returns it as `resolver_address`.
  A device that held it before is marked `reserved_address`, is never
  reclaimed, and can take a pool address with "Assign new address".
- Mesh: a region range, the host group supernet, a host group segment or a
  full node's advertised subnet overlapping 100.64.0.0/10 is marked
  `shared_address_space` (spec section 8.3.1, coexistence with other
  overlay software). The region and settings forms show the warning as
  the range is typed. Such ranges are still accepted.
