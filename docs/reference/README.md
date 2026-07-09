# V2Board Reference

This section is the operator/developer reference index for `v2board_AnixOps`.

## Core References

- [quickstart.md](quickstart.md): shortest Docker/local startup path
- [../UPGRADE.md](../UPGRADE.md): GitHub Actions artifact upgrade, verification, and rollback runbook
- [startup-config.md](startup-config.md): exact backend startup flow and runtime bootstrap order
- [configuration.md](configuration.md): config source-of-truth, key ownership, and mode-specific examples
- [forward-runtime-migration.md](forward-runtime-migration.md): old `FORWARD_RUNTIME_*` inputs to the new YAML-only runtime config
- [sqlite-to-postgres-migration.md](sqlite-to-postgres-migration.md): SQLite to PostgreSQL dry-run, import, verification, and rollback runbook
- [traffic-stats-operations.md](traffic-stats-operations.md): traffic log indexes, query bounds, and retention runbook
- [repository-layout.md](repository-layout.md): root and directory ownership, plus root hygiene rules
- [runtime.md](runtime.md): NodeX mode vs local Ansible mode, recommended `nftables_ansible` defaults, and where to operate each path

## Runtime And Boundary References

- [../guide/forward-relay-onboarding.md](../guide/forward-relay-onboarding.md): exact meaning of "relay really joined" in NodeX mode vs local Ansible mode
- [../guide/nodex-internal-extension.md](../guide/nodex-internal-extension.md): NodeX boundary and dual-mode semantics
- [../guide/forward-tunnel-runtime-ops.md](../guide/forward-tunnel-runtime-ops.md): runtime operation details
- [../guide/forward-tunnel-smoke-test.md](../guide/forward-tunnel-smoke-test.md): manual smoke path

## Implementation Guides

- [../guide/README.md](../guide/README.md): Flux clone workstream and feature guides
