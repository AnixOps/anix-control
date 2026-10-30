### Changed

- Install commands no longer carry `--forward`, and new route hops run only
  on the network model (ANX): the route editor has no engine picker for them.
  A route still on nftables keeps showing its engine. The script still
  accepts `--forward` from older commands.
