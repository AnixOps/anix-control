### Fixed

- Mesh: a device without an Agent gets the `dns` section only when its
  kernel lists `mesh_dns`. The `features` of a configuration read are
  limited to 4 KiB and 64 names, those of an enroll request to 64 names
  (400 otherwise).
