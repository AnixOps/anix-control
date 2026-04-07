# Repository Layout

This page defines what belongs at the repository root and where future files should go.

## Root Principles

The root should stay comparable to NodeX:

- short README
- source directories
- deployment entrypoints
- contributor/agent instructions

It should not become a dumping ground for:

- generated logs
- temporary smoke output
- ad-hoc YAML examples
- scratch notes

## Root Ownership

| Path | Purpose |
|------|------|
| `cmd/` | backend and helper program entrypoints |
| `config/` | app config, deploy assets, examples, ansible material |
| `docs/` | intro, reference, and guide docs |
| `internal/` | backend implementation |
| `web/` | Vue frontend |
| `deploy/` | deployment-specific assets |
| `scripts/` | automation and helper scripts |
| `readme.md` | short root entrypoint only |
| `AGENTS.md` | contributor/agent rules |
| `docker-compose.yml` / `Dockerfile` | container startup |

## Example Inputs

Sample YAML and test inputs belong under:

- `config/examples/`

Current examples:
- `config/examples/local_test.yaml`
- `config/examples/test_nodes.yaml`

The `subtest` helper should generate sample output there instead of the repo root.

## Generated Artifacts

Generated artifacts should go to dedicated locations or stay ignored:

- logs: `logs/`
- test reports: `test-reports/`
- local temporary output: ignored `tmp_*`, `.codex_*`, and similar scratch files

If you create new tooling that emits artifacts, do not write them into the root by default.

## Related Docs

- [startup-config.md](startup-config.md)
- [../intro/README.md](../intro/README.md)
- [../guide/README.md](../guide/README.md)
