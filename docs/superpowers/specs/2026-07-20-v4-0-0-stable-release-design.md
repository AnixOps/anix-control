# V4.0.0 Stable Release Design

## Goal

Publish the first immutable `v4.0.0` plugin-only stable release only after its
release version contract, CI gates, formal package evidence, and two operator
approval declarations are valid under the V4 official Ed25519 root.

## Decision

Use a release-preparation branch and repair the existing release blockers
before creating a tag. Do not create a draft, prerelease, or release tag as a
way to bypass a failing gate. The release tag is exactly `v4.0.0` because the
formal evidence tooling accepts that tag and no other V4 stable tag.

## Release Boundaries

- The current V4 root is the only trust root. The signing PEM remains outside
  Git and is never included in a diff, artifact source tree, or documentation.
- Every version surface checked by `check_release_version.py` changes from
  `4.0.0-alpha.7` to `4.0.0`, including the frontend lockfile and release
  documentation.
- Existing CI failures are fixed at their source rather than disabled or
  suppressed: stale `go.sum` entries, Control-backed Playwright startup,
  golangci-lint findings, gosec findings, and the cross-repository plugin-host
  lifecycle.
- Canary and support approval documents are generated only after a successful
  formal package rehearsal. The user's release authorization is the operator
  approval for those records; each record is bound to the resulting package
  subject digest and the V4 official root.
- GitHub Secrets hold the two approval documents and the already-rotated root
  signing material. Secret values are never read back or printed.

## Verification Contract

Before tagging, the release branch must pass local version validation, Go and
frontend suites, browser E2E, cross-repository Agent E2E, lint, production
gosec, formal sixteen-package verification, and V4 evidence verification.
After the branch is merged and pushed, its `go_dev` CI run must be green. Only
then create and push annotated tag `v4.0.0`, wait for the release workflow,
and verify the GitHub Release assets and V4 evidence bundle.

## Non-Goals

- Do not change release policy to tolerate a failing quality, security, or
  integration gate.
- Do not recreate historical trust roots or accept old-root packages.
- Do not alter the user's untracked `.superpowers/sdd/task-6-report.md`.
