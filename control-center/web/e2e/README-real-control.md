# Real Control Integration Gate

Run from `web/`:

```sh
npm run test:e2e:real-control
```

The gate builds a temporary signed `identity-platform` package, starts a real
Anix Control process with a private SQLite database and loopback listener, then
drives the Control Center browser through `/api/v2/login`, `/api/v3/plugins`,
and `/api/v3/plugin-installations`. Temporary files and processes are removed
after the run. Set `ANIX_CONTROL_ROOT` when the Control checkout is not at
`../anix-control`; set `ANIXOPS_REAL_CONTROL_API_PORT` or
`ANIXOPS_REAL_CONTROL_WEB_PORT` when the default ports are busy.

The default run is read-only after bootstrap. To add the stronger local
Control-target lifecycle check, provide the pinned Agent checkout and opt in:

```sh
ANIXOPS_AGENT_ROOT=/path/to/anix-agent \
ANIXOPS_REAL_CONTROL_LIFECYCLE=1 \
npm run test:e2e:real-control -- --reporter=line
```

That mode builds a signed `machine-telemetry` package, installs it into the
temporary Control process, then uses the Center page to disable and re-enable
the Control target. It does not claim Agent staging evidence; the Agent
binary is only a formal package input for the isolated local process.

A synthetic Workers session is deliberately pre-seeded in `localStorage` so the
test can enter the authenticated Center route. It is not a Workers login test
and does not claim Workers service integration. The Control login is real and
uses the separately issued Control JWT stored in `sessionStorage`.

If the system `go` command is a version selector that cannot build the checked
out Control module, set `ANIXOPS_GO_BIN` to a compatible Go binary. The default
is the `go` command found on `PATH`.
