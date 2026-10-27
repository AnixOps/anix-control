### Changed

- The systemd Control installer (`scripts/install.sh`) keeps the Agent
  release Control installs nodes with: an empty `agent_install.artifact_dir`
  is set to `<data_dir>/agent-releases`, and after a healthy install or
  update the release `/install/agent.env` names is downloaded into
  `<artifact_dir>/<tag>/` (both zips with their `.dgst` and `.sig`,
  `SHA256SUMS` and `SHA256SUMS.sig`, checked against the official key).
  Nodes then download the Agent and its checksum from Control, so hosts
  that cannot reach GitHub (mainland China) install with the copied command
  alone. A failed download is a warning; nodes then use GitHub as before.
- The Agent install script writes `net.ipv6.conf.<if>.accept_ra = 2` for
  SLAAC interfaces by default when it turns forwarding on, so a forward
  node keeps its IPv6 default route without `--accept-ra`. `--no-accept-ra`
  turns it off; `--accept-ra` is still accepted.
