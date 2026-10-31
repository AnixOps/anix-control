### Added

- Upgrade campaigns offer the arm64 anx-node package (`anx-node-<version>-aarch64-linux-musl.tar.gz`)
  when the release in `agent_install.artifact_dir/anx-node/<version>/` lists it in
  `SHA256SUMS`. A release without it still works: arm64 nodes just get no kernel from it.
  The private anx release workflow now builds and signs both x86_64 and aarch64.
