### Security

- The nightly frontend audit (Nightly Security, `npm audit (web)`) passes again.
  Development tools move to fixed releases: vitest, `@vitest/ui` and
  `@vitest/coverage-v8` 4.1.12, vite 8.3.8 (and with it launch-editor 2.14.2),
  happy-dom 21.0.4, markdown-it 14.3.3 and fast-uri 3.1.9.
  `web/audit-allowlist.json` drops the braces entry that no longer applies and
  waives, until 2026-12-13, the two advisories that have no fix the current
  eslint and js-beautify accept (minimatch GHSA-g3hv-75wg-m249, glob
  GHSA-3g3m-2fjr-6pw4). Nothing in the shipped frontend changes.
