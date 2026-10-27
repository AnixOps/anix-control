### Changed

- CI runs the Frontend E2E shards in the official Playwright image
  (`mcr.microsoft.com/playwright`, the visual job's image) instead of
  installing Chromium and its system packages on every run. A step fails the
  job when `@playwright/test` in `web/package-lock.json` no longer matches the
  image tag, so bump both together.
