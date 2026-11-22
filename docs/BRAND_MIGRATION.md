# AnixOps Brand Migration

The product names and repositories are:

| Role | Primary name | Repository |
|---|---|---|
| Control plane | AnixOps Control | `https://github.com/AnixOps/anix-control` |
| Node runtime | AnixOps Agent | `https://github.com/AnixOps/anix-agent` |

Control uses `anix-control` for the binary, systemd service, Docker image,
container and release assets. The node runtime uses `anix-agent`.

## Origin

Control started in December 2025 as a fork of the community-maintained
`wyx2685/v2board` (V2Board, PHP, MIT). The PHP code was replaced by a Go
backend and a Vue 3 frontend within the first weeks; no PHP code remains. The
original MIT notice is kept in [`NOTICE`](../NOTICE).

## Names That Are Gone

The names inherited from V2Board are no longer used:

| Former name | Now |
|---|---|
| UniProxy, the `v2board` gRPC services and the node WebSocket | removed in 5.0; nodes run `anix-agent` on the `anix.agent.v1` stream |
| default SQLite file `config/data/v2board.db` | `config/data/control.db` (an install whose configuration names no file keeps `v2board.db`: `install.sh update` writes it into the configuration) |
| Prometheus metrics `v2board_*` | `anixops_control_*` |
| browser storage key `v2board-theme` | `app.theme` (the theme choice is asked again once) |
| `bin/v2board` link and the `/opt/v2board` layout of the installer | removed; `install.sh update` deletes the link |
| `/home/v2board/.ssh` in the container | `/home/anixops/.ssh` |

Still unchanged:

- database tables prefixed with `v2_` (renamed by a later migration);
- the HTTP API versions `/api/v2` to `/api/v5`, which are Control's own;
- PostgreSQL database and role names an operator chose, for example
  `anix_control_v2board`: renaming a database is an operator decision.

## Release Naming

GitHub Releases publish only `anix-control-*` artifacts and the
`anixops/anix-control` Docker image.

The Go module is `github.com/AnixOps/anix-control/v5` (the 4.x tags were built
with `github.com/AnixOps/anix-control/v4`). Stable and prerelease tags take
the forms `vX.Y.Z` and `vX.Y.Z-alpha.N`, `-beta.N`, `-rc.N`.

## Native Install Paths

```text
/opt/anixops/control/bin/anix-control
/opt/anixops/control/config/config.yaml
/opt/anixops/control/web/public
anix-control.service
```
