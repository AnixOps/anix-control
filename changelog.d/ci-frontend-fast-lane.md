### Changed

- CI: a pull request that leaves `web/` alone skips Frontend Build's lint,
  unit tests and mocked Playwright gate (about 14 of its 16 minutes); the
  job, its live Control WebUI gate, the build and the bundle budget still
  run, and the full lane runs everything. Playwright uses 4 workers in CI.
