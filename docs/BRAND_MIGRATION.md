# AnixOps Brand Migration

The product names and repositories are now:

| Role | Primary name | Repository |
|---|---|---|
| Control plane | AnixOps Control | `https://github.com/AnixOps/anix-control` |
| Node runtime | AnixOps Agent | `https://github.com/AnixOps/anix-agent` |

New installations use `anix-control` for the binary, systemd service, Docker
image, container, and release assets. The node runtime uses `anix-agent`.
The development template enables the Agent gRPC control channel on loopback
port `50051`. Production keeps it disabled until TLS or a restricted
HTTP/2-capable proxy is configured. Existing configuration files are preserved
and are not force-enabled. In `v4.0.0-alpha.3`, `anix.agent.v1` and the
official signed package/WebUI path are an operational alpha preview: they are
available for canary use but do not yet replace every REST/UniProxy, legacy
gRPC, or WebSocket task path. Stable publication requires a passing 72-hour
canary and explicit operator authorization.

## Compatibility Window

The v3 migration deliberately keeps these compatibility identifiers unchanged;
the v4 release continues to preserve them:

- HTTP API versions and compatibility routes: `/api/v1`, `/api/v2`, and UniProxy
- protobuf package/service namespace and generated directory: `v2board`
- database tables prefixed with `v2_`
- default SQLite filename `config/data/v2board.db`
- existing PostgreSQL database/user names when already deployed
- JWT issuer `v2board` and browser storage key `v2board-theme`
- Prometheus metric names prefixed with `v2board_`
- Docker runtime user/home path `/home/v2board` used by mounted SSH material
- Ansible variables and legacy playbooks prefixed with `v2bx_`

Renaming those values in place would invalidate tokens, disconnect old nodes,
or make an existing database appear empty. They will only move behind explicit
migrations with dual-read or dual-registration support.

## Release Naming

GitHub Releases publish only `anix-control-*` artifacts and the
`anixops/anix-control` Docker image. Existing installations upgrade in place
through the AnixOps installer, which preserves their configuration and data.

The new Go module is `github.com/AnixOps/anix-control/v4`. Stable and
prerelease tags are accepted in these forms:

- `v4.0.0`
- `v4.0.0-alpha.1`
- `v4.0.0-alpha.3`
- `v4.0.0-beta.1`
- `v4.0.0-rc.1`

## Native Install Paths

Fresh installations default to:

```text
/opt/anixops/control/bin/anix-control
/opt/anixops/control/config/config.yaml
/opt/anixops/control/web/public
anix-control.service
```

The installer also creates `bin/v2board` as a compatibility symlink. For an
in-place upgrade of an existing `/opt/v2board` deployment, keep the old service
account and directory explicitly:

```bash
sudo SERVICE_NAME=v2board APP_USER=v2board INSTALL_DIR=/opt/v2board \
  bash /tmp/anix-control-install.sh update --version v4.0.0-alpha.3
```

The existing configuration and database are preserved. Back them up before the
first branded release exactly as for any other application upgrade.

## Test Expectations

Brand migration changes must keep these checks green:

```bash
go test ./internal/config ./internal/handler ./cmd/server
cd web && npm test && npm run build
bash config/scripts/check_release_workflow.sh --self-test
bash config/scripts/check_release_workflow.sh
python3 config/scripts/generate_release_manifest.py --self-test
python3 config/scripts/verify_release_artifacts.py --self-test
```
