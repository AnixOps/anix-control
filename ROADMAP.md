# Roadmap

This file is a pointer. The roadmap content lives in the maintained documents
below so that status is recorded in exactly one place.

| Question | Source of truth |
|----------|-----------------|
| What is the current development direction (moving business domains into packages, milestones M0–M4)? | [`docs/architecture/package-extraction.md`](docs/architecture/package-extraction.md) |
| What is the current release work, and how is a release cut? | The `## Unreleased` section of [`CHANGELOG.md`](CHANGELOG.md) and [`docs/RELEASING.md`](docs/RELEASING.md); the 4.0.x RC roadmap [`docs/ROADMAP-4.0.x-RC.md`](docs/ROADMAP-4.0.x-RC.md) and its evidence snapshot [`docs/RC-EVIDENCE-4.0.x.md`](docs/RC-EVIDENCE-4.0.x.md) are historical |
| Which product stages shipped, and what is explicitly not complete? | [`docs/architecture/release-line-status.md`](docs/architecture/release-line-status.md) |
| Is a feature implemented, partial, planned, or deferred? | [`docs/features.md`](docs/features.md) |
| What concrete work is still open? | [`TODO.md`](TODO.md) |
| What changed and when? | [`CHANGELOG.md`](CHANGELOG.md) |

Rules that apply to every roadmap item:

- An item moves to done only after code, tests, and CI evidence exist.
- A passing unit or browser test is not a production canary. Agent dispatch
  and topology execution (`plugins.dispatch_enabled`,
  `plugins.topology_execution_enabled`) are off by default; turning them on in
  production is the operator's decision, and staging or canary evidence is
  worth collecting first, but no release gate requires it
  ([`docs/RELEASING.md`](docs/RELEASING.md)).
- Product version tags follow the CHANGELOG and
  [`docs/RELEASING.md`](docs/RELEASING.md), not the Go module import suffix
  (`/v4`).
