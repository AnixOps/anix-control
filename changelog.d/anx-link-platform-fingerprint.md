### Changed

- `anx://` links in the AnixOps Net subscription no longer carry `fp=chrome`.
  The client then uses the TLS ClientHello shape of its own platform: Safari's
  on iOS and macOS, Chrome's elsewhere (a Chrome ClientHello from an Apple
  device is unusual on the wire). Clients released before that choice existed
  read a missing `fp` as `chrome`, so nothing changes for them.
