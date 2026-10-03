# Branch Protection: `go_dev` Ruleset

`go_dev` is the only long-lived branch of `AnixOps/anix-control` and the
default branch. It is protected by a repository ruleset (Settings -> Rules ->
Rulesets), not by a classic branch protection rule. This file documents the
intended configuration so that it can be reviewed and restored; the ruleset in
GitHub is the enforcing copy.

## Target

- Enforcement: active
- Target branches: `go_dev` (default branch)
- There are no other protected long-lived branches. Do not recreate
  `production` or `master`.

## Rules

| Rule | Setting |
|------|---------|
| Require a pull request before merging | on |
| Required approvals | 0 |
| Dismiss stale approvals / require Code Owner review | off (`CODEOWNERS` only requests review) |
| Allowed merge method | squash |
| Require status checks to pass | on |
| Block force pushes | on |
| Restrict deletions | on |

## Required Status Checks

These are job names from `.github/workflows/ci.yml` and must match exactly:

- `Go Quality Gates`
- `Go Lint Gate`
- `Backend Tests`
- `Frontend Build`
- `Documentation Sync Check`
- `Release Workflow Policy Check`
- `Frontend Visual Regression` (required since 2026-10-03, after 26 green runs; owner decision H9)

Deliberately not required:

- `Go Security Scans`: `govulncheck` reads a vulnerability database that
  changes daily, so a new advisory could block unrelated PRs. It still runs on
  every push and PR and must be triaged when it fails.
- Control Center checks (`.github/workflows/control-center.yml`,
  `control-center-workers.yml`): they are path-filtered to `control-center/**`
  and do not run on every PR, so requiring them would leave unrelated PRs
  pending forever.
- The remaining `ci.yml` jobs (race detector, integration-style gates, package
  release contracts, Docker smoke, and so on) run on every PR and should be
  green before merge, but are not merge-blocking.

If a required job is renamed in `ci.yml`, update the ruleset and this file in
the same PR.

## Bypass

- Repository administrators are on the bypass list with mode "pull requests
  only": they may merge a PR whose checks are pending or failing, but they
  cannot push directly to `go_dev`.
- Nobody pushes directly to `go_dev`, including release preparation. Release
  tags (`v*`, and `control-center-v*` for the Control Center) are created on
  commits that already landed through a PR.

## Contributor Workflow

1. Branch from the latest `go_dev`.
2. Open a PR against `go_dev`; include the documentation update required by
   `config/scripts/check_docs_updated.sh` in the same PR.
3. Wait for the required checks, then squash-merge.
