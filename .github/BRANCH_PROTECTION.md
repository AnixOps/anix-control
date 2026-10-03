# Branch Protection: `go_dev` And Release Branch Rulesets

`go_dev` is the integration branch of `AnixOps/anix-control` and the default
branch. The only other long-lived branches are the maintenance branches
`release/vX.Y` ([`docs/RELEASING.md`](../docs/RELEASING.md#release-branches)).
Both are protected by repository rulesets (Settings -> Rules -> Rulesets), not
by classic branch protection rules. This file documents the intended
configuration so that it can be reviewed and restored; the rulesets in GitHub
are the enforcing copy.

## Target

- Enforcement: active
- Target branches: `go_dev` (default branch)
- Release branches have their own ruleset
  ([Release Branches](#release-branches-release)). Do not recreate
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
- `Forward Netns E2E`: the forward SDK's multi-namespace end-to-end suite
  (forward-sdk.md section 13), run on pull requests in the `forward` change
  class, nightly and on manual runs. Owner decision H14: it becomes
  required only after two weeks of green runs (added 2026-10-03, so not
  before 2026-10-17); pull requests outside the class skip it, which counts
  as passed.
- The remaining `ci.yml` jobs (race detector, integration-style gates, package
  release contracts, Docker smoke, and so on) run on every PR and should be
  green before merge, but are not merge-blocking.

If a required job is renamed in `ci.yml`, update the ruleset and this file in
the same PR.

## Bypass

- Repository administrators are on the bypass list with mode "pull requests
  only": they may merge a PR whose checks are pending or failing, but they
  cannot push directly to `go_dev`.
- Nobody pushes directly to `go_dev` or a release branch, including release
  preparation. Release tags (`v*`, and `control-center-v*` for the Control
  Center) are created on commits that already landed through a PR, on
  `go_dev` or on a release branch.

## Release Branches (`release/**`)

A second ruleset, "release branches", protects every `release/vX.Y` branch
with the same rules as `go_dev`:

- Enforcement: active
- Target branches: include `refs/heads/release/**`
- Rules: require a pull request (0 approvals, squash), require the same
  status checks as `go_dev` (the list above), block force pushes, restrict
  deletions
- Bypass: repository administrators, mode "pull requests only"

`ci.yml` runs on pull requests to and pushes on `release/**`, so the required
checks report there as on `go_dev`. A release branch is cut only with the
owner's approval. Create the branch before the ruleset, or set "Do not enforce
on create" on its status check rule, so that the first push is not refused for
missing checks.

## Contributor Workflow

1. Branch from the latest `go_dev` (or from `release/vX.Y` for a change to a
   maintenance line).
2. Open a PR against that branch; include the documentation update required
   by `config/scripts/check_docs_updated.sh` in the same PR.
3. Wait for the required checks, then squash-merge.
