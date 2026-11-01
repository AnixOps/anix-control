### Security

- CI: the `ci`, `control-center` and `control-center-workers` workflows run
  with a read-only `GITHUB_TOKEN` by default; the jobs that publish images or
  releases keep their own explicit permissions.
- Control Center web: follow-redirects 1.16.1 → 1.16.2
  (GHSA-mrwp-f3q8-3g9p, high: credentials could follow redirects).
