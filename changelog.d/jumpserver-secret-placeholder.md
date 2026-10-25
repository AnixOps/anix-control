### Added

- The JumpServer workflow's `run` command can name a secret as
  `@@CLOUDFLARE_API_TOKEN@@`: `jms.py` fills it in from the `production`
  environment's secret of that name just before sending the job and masks it
  in the log, so a host can get the token for certbot's DNS-01 renewal
  without it passing through the workflow inputs.

### Fixed

- `jms.py run --show-output` no longer prints the NUL padding that ends
  JumpServer's job log.
