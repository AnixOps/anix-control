### Security

- CI: the `ci`, `control-center` and `control-center-workers` workflows run
  with a read-only `GITHUB_TOKEN` by default; the jobs that publish images or
  releases keep their own explicit permissions.
