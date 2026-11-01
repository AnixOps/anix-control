### Added

- Hotfixes: `.github/workflows/hotfix.yml` ships a fix merged to `main` as
  `<release>.hf.<n>` (`v5.0.0-rc.25.hf.1`) without a whole release. It picks
  the named commits onto `hotfix/<release>`, tests only what they touch,
  builds and signs Control (and the Agent when Agent commits are named), and
  with `deploy` installs it on the running Control after a database dump,
  restoring the previous release when the health check fails; a cold standby
  gets the files only. Release tags with `.hf.<n>` are accepted wherever dev
  tags are (`agentinstall.IsHotfix`, `install.sh`, the Agent upgrade
  payload), and a hotfix Agent is served from the Control release of the
  same tag (docs/RELEASING.md, "Hotfixes").

### Fixed

- The dev build deploy no longer installs on a cold standby Control (a
  stopped service): only the running Control takes a dev build.
