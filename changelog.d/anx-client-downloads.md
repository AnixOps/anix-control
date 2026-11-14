### Added

- Control hosts the ANX edition of the AnixOps Net client for subscribers
  (docs/reference/client-downloads.md). Super administrators, or the anx
  release workflow with their admin API token, upload a signed release with
  `POST /api/v4/kernel/client-releases` (SHA256SUMS signed by the anx release
  key, as for anx-node); `GET` lists the stored releases and `DELETE
  /api/v4/kernel/client-releases/{version}` removes one. Releases are kept in
  `agent_install.artifact_dir/anixops-net-anx/`.
- Subscribers download the newest release with their subscription token:
  `GET /{subscribe_path}/{token}/client` and
  `GET /{subscribe_path}/{token}/client/{version}/{file}` (banned or expired
  subscribers get 403). The subscription page shows an "AnixOps Net (ANX
  edition)" section with the Windows, macOS, Linux and Android files and the
  iOS TestFlight invitation when Control holds a release; without one the page
  is unchanged.
- To use it, set `agent_install.artifact_dir` and let the reverse proxy in
  front of Control accept uploads of a few hundred MB on
  `/api/v4/kernel/client-releases` (nginx `client_max_body_size`).
