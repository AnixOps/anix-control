### Fixed

- Upgrading to 5.0.0-rc.11 from a release that had the gost-mesh package
  enabled no longer fails every built-in package. Before, the leftover
  gost-mesh installation named the removed nodex settings, every package
  failed with `enabled plugin gost-mesh is invalid`, the identity gateway did
  not start and `install.sh update` rolled back to a release that cannot read
  the migrated database. Control now disables the installation of a retired
  package at start, audited as `retired`. On an rc.11 Control that is stuck
  this way, disable it by hand and restart:
  `UPDATE v3_kernel_plugin_installation SET enabled = false, state = 'disabled' WHERE plugin_id = 'gost-mesh';`
