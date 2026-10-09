### Security

- **The sprintf-js npm audit waiver is dropped.** GHSA-hp3w-g68c-fv3c has no
  fixed version, but the package was reached only through Histoire's
  `gray-matter`, `js-yaml` 3 and `argparse` 1, and `js-yaml` 3 uses `argparse`
  only in its command-line tool. `web/package.json` now overrides the
  `argparse` that `js-yaml` asks for to `^2.0.1`, `sprintf-js` is no longer in
  `web/package-lock.json`, and its entry is removed from
  `web/audit-allowlist.json`. The braces waiver (GHSA-vfj7-8cjw-p6xm) is
  unchanged. Nothing in the shipped frontend changes.
