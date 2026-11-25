### Changed

- Mesh: a device without an Agent gets its region network only when its
  kernel lists `mesh_native_keys` (region networks admit only the keys
  they list, which an older kernel ignores), and `dns.resolver_address`
  only with `mesh_resolver_address`. The capabilities come from the
  `features` of the enroll request or of `GET /api/v5/mesh/device/config`
  (comma separated), or else from the top-level `features` of the
  device's latest report. The device list marks an enrolled device that
  has not named `mesh_native_keys` (`native_keys_unsupported`).
