# Releasing

A release is a `vX.Y.Z` tag on a commit that landed through a pull request,
on `go_dev` or on a maintenance branch `release/vX.Y`
([Release Branches](#release-branches)). GitHub Actions builds, signs and
publishes everything. Nothing is built locally.

## Cut A Release

1. On a branch from `go_dev` (or from the release branch), set the version
   and date the changelog:

   ```bash
   python3 config/scripts/prepare_release.py 4.1.0
   ```

   The script turns the `## Unreleased` entries into `## 4.1.0 - <today>`,
   leaving an empty `## Unreleased` on top. It sets the version wherever the
   tree declares it:
   - branding;
   - Makefile;
   - Swagger;
   - frontend package;
   - example configurations;
   - README.

   Then it checks the result with `config/scripts/check_release_version.py`.
2. Open a pull request (`release: v4.1.0`) and merge it once CI passes.
3. Tag the merged commit and push the tag:

   ```bash
   git tag v4.1.0 <merge-commit>
   git push origin v4.1.0
   ```

A suffix (`v4.1.0-rc.1`, `-beta.2`, `-alpha.3`) publishes a prerelease. Only
unsuffixed tags become the latest release.

## Release Branches

When `go_dev` already holds work for the next minor version, release from a
maintenance branch `release/vX.Y` instead. v4.1.0 is the first release cut
this way: `release/v4.1` is cut from the v4.1.0-rc.6 commit, because `go_dev`
already holds v4.2 work.

1. **Cut the branch** from the previous release's tag or commit, with the
   owner's approval:

   ```bash
   git push origin <tag-or-commit>:refs/heads/release/v4.1
   ```

   The branch is protected like `go_dev` (`.github/BRANCH_PROTECTION.md`).
2. **Bump on the branch.** Branch from `release/vX.Y`, run
   `prepare_release.py`, and open the pull request (`release: v4.1.0`)
   against `release/vX.Y`, not `go_dev`. CI runs on pull requests to
   `release/**` with the same required checks.
3. **Tag the release branch commit** that the pull request merged, as above.
   The tag pipeline does not depend on the branch that holds the commit.
4. **Merge back.** Open a follow-up pull request to `go_dev` with the
   release's CHANGELOG section and, when they apply, its version surfaces:
   - the `## X.Y.Z - <date>` section goes directly under `go_dev`'s
     `## Unreleased`, or below a newer release section `go_dev` already has
     (`## 4.2.0-rc.1`, say). The first dated section must stay the version
     `go_dev` declares, since `check_release_version.py` reads the first one;
   - entries that shipped in the release leave `go_dev`'s `## Unreleased`;
   - the version surfaces come back only while `go_dev` declares an older
     version (`4.1.0-rc.6` → `4.1.0`). Never lower `go_dev`'s version.

**Patch releases** (`vX.Y.Z`, Z > 0) of that line use the same branch. Land
the fix on `go_dev` first, then bring it to `release/vX.Y` in a pull request
(cherry-pick). A fix for the old line only goes to the release branch alone,
and its pull request says so. Then bump, tag and merge back as above.

**`latest` on a superseded line.** An unsuffixed tag becomes the latest GitHub
Release and moves the image's `latest` tag (`make_latest` and `latest=auto` in
`ci.yml`), and the frozen `scripts/install.sh` installs `releases/latest` when
no version is pinned. A patch on a line that a newer release has superseded
(`v4.1.1` after `v4.2.0`) would move both back to the older line. The pipeline
does not handle that yet: before tagging such a release, change it to mark
only the highest version `latest`.

## What The Tag Pipeline Does

`ci.yml` runs the full test suite, then:

- **Tag gate.** The tag's version must equal the tree's declared version.
- **Official packages.** Every package under `packages/` is built at the tag's
  version and signed with the protected key `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY`.
  - The signing root must equal `plugins.official_public_key` in
    `config/config.prod.yaml`, since Control only accepts packages signed by
    that root.
  - Agent packages embed Agent binaries from the pinned `anix-agent` commit
    and a checksum-pinned GOST runtime.
  - The pin is `AGENT_REF` in the top-level `env` of `ci.yml`; every
    anix-agent checkout (packages, cross-repository E2E, live WebUI gate)
    takes its ref from it. Move it to the Agent's release commit with every
    Agent tag, in the pull request that prepares the Control release.
  - `plugin-package-release-test` runs `govulncheck` over the four Agent
    commands the plugin binaries are built from and fails for a
    vulnerability their call graph reaches, so a vulnerable Agent
    dependency cannot reach the signed packages.
- **Binaries.** Linux, Windows and macOS builds for amd64 and arm64, plus the
  frontend archive (`anix-control-frontend.tar.gz`).
- **Image.** `ghcr.io/anixops/anix-control` for linux/amd64 and linux/arm64,
  with the signed identity bootstrap package inside.
  - It is signed with cosign (keyless).
  - It carries SBOM and provenance attestations.
- **GitHub Release.** About 20 assets: the six binaries, the frontend,
  `official-public-key.raw`, the source SBOM, `RELEASE_MANIFEST.json`,
  `RELEASE_NOTES.md`, `SHA256SUMS.txt`, `docker-image.txt`, the Agent
  installer `agent-install.sh` with its signature `agent-install.sh.sig`
  (the same file Control serves at `/install.sh`; verify it against the
  official root as in `docs/guide/agent-onboarding.md`), and the packages:
  - `anix-control-packages-<version>.tar.gz` holds every package's `.anxp`,
    `.manifest.json`, `.manifest.sig` and `.sbom.spdx.json`, plus one
    `official-public-key.pem`. `build_package.py --release-archive` writes
    it from the `--all` build, so `PACKAGE_SPECS` in
    `packages/shared/build_package.py` is the one list of what ships.
  - `anix-control-packages-<version>.tar.gz.sig` is the Base64 Ed25519
    signature of the archive bytes, made with the package signing key by the
    same `openssl pkeyutl -sign -rawin` step that signs each manifest.
  - `identity-platform-<version>.anxp`, `.manifest.json` and `.manifest.sig`
    stay separate assets: the frozen `scripts/install.sh` downloads them by
    name.
  - `RELEASE_MANIFEST.json` lists each package under `packages` (id,
    version, `.anxp` size and SHA-256, manifest SHA-256).

  The release body is the tag's CHANGELOG section.

`config/scripts/check_release_workflow.sh` keeps these essentials in the
workflow. Change it in the same pull request as any release step.

## Nightly Security Scan

The `Nightly Security` workflow (`.github/workflows/nightly-security.yml`)
runs every night at 03:17 UTC against `go_dev`, and on demand with
`gh workflow run nightly-security.yml -R AnixOps/anix-control`. It exists
because advisories are published against code nobody touched: they would
otherwise fail an unrelated pull request, or be found on release day. A
failed job fails the run, and the failing job names the finding:

- **govulncheck** over every Go module (root, `sdk`, `identity`,
  `control-center`), at the version `ci.yml` pins.
- **gosec**: the production gate exactly as `ci.yml` runs it, plus the `sdk`
  and `identity` gates.
- **npm audit** of `web/` through `npm run audit:check`, so the waivers in
  `web/audit-allowlist.json` and their expiry dates apply.
- **relay fuzz**: every fuzz target of `sdk/forward/relay` and
  `sdk/forward/relay/link` for 60 seconds each. The corpus the engine learns
  is cached from night to night, and a failing input is uploaded as the
  `relay-fuzz-failures-*` artifact: copy it to `testdata/fuzz/<Target>/` of
  its package to reproduce it with a plain `go test`.

The pull-request pipeline keeps its own `govulncheck` and `gosec` jobs; this
workflow does not replace them.

## Verifying A Release

- **Files.** Run `sha256sum -c SHA256SUMS.txt`, and see
  [`UPGRADE.md`](UPGRADE.md#artifact-verification).
- **Image.** Verify the image with:

  ```bash
  cosign verify ghcr.io/anixops/anix-control@<digest> \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/AnixOps/anix-control/'
  ```

- **Packages.** Check the archive signature and extract a package as in
  [`UPGRADE.md`](UPGRADE.md#getting-a-package-from-the-release). Control
  verifies every package manifest against its configured official root on
  import.

## History

Up to `v4.0.0`, releases also passed a product-stage contract, a public
rehearsal and a signed evidence bundle with canary and support approvals.
Those gates were retired after `v4.0.0`; that release keeps its evidence
bundle, and its guides still describe it
([`guide/v4-plugin-only-upgrade.md`](guide/v4-plugin-only-upgrade.md)).
