### Added

- Dev builds, an internal test channel (`docs/RELEASING.md`, "Dev builds"):
  after every green `main`, `.github/workflows/dev-release.yml` publishes a
  pre-release such as `v5.0.0-rc.15.dev.42` holding Control and the Agent at
  the same version, signed with the official key. A manual run with `deploy`
  installs it on the production Control after a database dump. Release tags,
  `install.sh`, `agent_install.agent_version`, the Agent installer and Agent
  upgrade requests accept these versions, and the node page's github mirror
  downloads a dev Agent from the Control pre-release.
- Feature flags for unfinished work (`internal/features`): a feature in the
  dev stage runs in dev builds only. `GET /api/v4/kernel/features` lists the
  build's channel and the gated features.
