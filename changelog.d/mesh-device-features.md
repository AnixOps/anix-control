### Changed

- Mesh: a device without an Agent gets `kind`/`native_keys` and
  `dns.resolver_address` in its configuration only when its latest report
  lists `mesh_native_keys` and `mesh_resolver_address` in a top-level
  `features` array (its kernel's capabilities); before its first report
  they are left out, since the device refuses fields it does not know.
