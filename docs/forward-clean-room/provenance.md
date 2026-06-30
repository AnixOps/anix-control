# Clean-Room Provenance

## Rule

This repository must not copy Forwardx source code, vendored code, generated
files, comments, tests, or documentation text.

Forwardx can be studied only as product behavior inspiration. Any implementation
must be written from independently designed contracts and local project needs.

## Current Implementation

The current panel-side implementation was created from this repository's
existing forwarding architecture:

- `ForwardRuntimeJob` job queue
- `Forward`, `ForwardTunnel`, and `ForwardNode` models
- Existing runtime status fields on `Forward`
- Existing admin/user forward APIs

New code added for the clean runtime:

- `internal/model/forward_clean_agent.go`
- `internal/service/forward_clean_agent_service.go`
- `internal/handler/forward_clean_agent.go`
- `clean_agent` backend branches in existing runtime code

No files were copied from Forwardx.

## Review Checklist

Before adding future agent-side code:

- Confirm all source files are authored in this repository.
- Do not paste Forwardx code, tests, configs, or docs.
- Do not reuse Forwardx names for internal symbols unless they are generic terms.
- Keep external behavior documented in `docs/forward-clean-room/spec.md`.
- Preserve license notices for any new third-party dependency.

## Dependency Policy

Prefer Go standard library and existing project dependencies. If a new dependency
is required:

- Verify the license is compatible with this repository.
- Add it through normal module tooling.
- Document why it is required.
