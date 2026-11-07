### Added

- Manual kernel install. 升级节点 → 只升级内核 can upload the files of an
  anx-node release (`SHA256SUMS`, `SHA256SUMS.sig` and the packages), one
  package per request: `POST /api/v4/kernel/agents/kernel-releases`
  (multipart field `files`, super administrators). Control checks the
  signature against the kernel release key built into Control (the one the
  install script and the Agent embed) and no other, checks each package
  against `SHA256SUMS` and stores the release in
  `agent_install.artifact_dir/anx-node/<version>/`. Push it with a kernel-only
  upgrade, which installs the newest release stored there. Errors:
  `kernel_release_invalid` (signature or digest), `kernel_release_conflict`
  (another `SHA256SUMS` for that version), `agent_install_unconfigured` (no
  `agent_install.artifact_dir`).
