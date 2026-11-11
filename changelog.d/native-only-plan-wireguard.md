### Removed

- The kernel's legacy handlers for the plan and wireguard routes (8 v2
  routes), the plan service methods only they used, and their
  native-vs-legacy compatibility tests (`plancompat`, `wireguardcompat`).
  The plan and wireguard package hosts now run every route natively from
  the start (`pluginhostsdk.RouterConfig.NativeOnly`), as knowledge and
  ticket already do.
- The wireguard keypair route keeps a test of its own in the package
  (`packages/wireguard/native`), which the removed comparison test carried.
