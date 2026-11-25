### Added

- Mesh: node and device configurations carry the region's resolver address
  as `dns.resolver_address` (spec section 15.9.2, item 8): nodes no longer
  derive it. While a device still holds the address from before it was
  reserved, it is not delivered and the region is marked
  `resolver_address_held`; it is delivered once the device takes another
  address. Agents leave the field out for kernels that do not take it.
