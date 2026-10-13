### Added

- The official packages are built into Control. Every package host is
  linked into the binary (`packages/builtin`) and runs as a child process of
  the same binary, `anix-control package-host <id>`, with the usual host
  socket, package bridge and environment. The signed slim packages
  (`build_package.py --all --builtin`: no host or Agent binary, a manifest
  whose `control_entrypoint` is `{"builtin": "<id>"}`) are embedded
  (`internal/builtinpkg`). At every start Control verifies each one against
  `plugins.official_public_key`, installs or upgrades it to the binary's
  version (the package migrations run for the new lifecycle generation) and
  enables it, in dependency order; a restart with the same binary changes
  nothing. There is no package to import. `make build` and `make run` build
  a development-key set first (`make dev-package-key`).
- A built-in package that does not verify or does not run is named in the
  start log and raised as the critical kernel alerts
  `builtin_package_invalid` and `builtin_package_failed` (dashboard and
  digest); only its own routes are unavailable and Control starts anyway.
- The release signs `SHA256SUMS.txt` with the official package key
  (`SHA256SUMS.txt.sig`), and `scripts/install.sh` verifies it (OpenSSL 3)
  before it trusts a checksum. Control logs the path and SHA-256 of its own
  executable, the file every package host runs.

### Changed

- Releases build the packages before the binaries, publish linux/amd64 and
  linux/arm64 binaries only, and list the built-in packages in
  `RELEASE_MANIFEST.json`. The release image no longer carries an identity
  package. `plugins.identity_bootstrap_package_dir` is ignored by a binary
  with built-in packages (a development build without them still reads it).
  Agent dispatch and topology execution stay off: the release builds no
  Agent plugin.
- `GET /api/v4/public/config` answers `edition: commercial` and an empty
  `hidden_packages` on every installation, so the web app shows every page.

### Removed

- The community and commercial editions: `app.edition`
  (`ANIX_CONTROL_APP_EDITION`), `config/editions.json`, the router's edition
  filter, the forward v4 API's reserved commercial prefixes, the
  `Commercial Packages` workflow and `build_package.py --edition`. Orders,
  payments and affiliate are installed in every installation; a
  configuration that still sets `app.edition` loads and the key is ignored.
- The release assets `anix-control-packages-<version>.tar.gz` (+ `.sig`),
  the `identity-platform` package files and the Windows and macOS binaries,
  and `build_package.py --release-archive` / `--verify-release-archive`.
