# Releasing

A release is a `vX.Y.Z` tag on a commit that landed through a pull request,
on `main` or on a maintenance branch `release/vX.Y`
([Release Branches](#release-branches)). GitHub Actions builds, signs and
publishes everything. Nothing is built locally.

## Cut A Release

1. On a branch from `main` (or from the release branch), set the version
   and date the changelog:

   ```bash
   python3 config/scripts/prepare_release.py 4.1.0
   python3 config/scripts/prepare_release.py 4.2.0 --agent-tag v4.2.0
   ```

   The script merges the changelog fragments of `changelog.d/` (one file per
   change, [format](../changelog.d/README.md)) into `## Unreleased`: their
   entries join the headings already there, in file-name order, and no
   heading appears twice. It turns the `## Unreleased` entries into
   `## 4.1.0 - <today>`, leaving an empty `## Unreleased` on top, and deletes
   the consumed fragments with `git rm` (`changelog.d/README.md` stays).
   Entries written directly under `## Unreleased` are released too. An
   invalid fragment stops it before any file changes;
   `python3 config/scripts/changelog_fragments.py check` shows why, and
   `... render` previews what the release will contain. `--agent-tag` names the
   anix-agent release this Control installs on nodes (`config/AGENT_RELEASE`,
   see [the Agent pin](#what-the-tag-pipeline-does)); without it the file keeps
   its value, so a patch release over an unchanged Agent names the Agent tag
   it already had. It sets the version wherever the
   tree declares it:
   - branding;
   - Makefile;
   - Swagger;
   - frontend package;
   - the Helm chart's `appVersion` (its default image tag; the chart's own
     `version` is a separate SemVer);
   - README;
   - `config/AGENT_RELEASE`, only with `--agent-tag`.

   Then it checks the result with `config/scripts/check_release_version.py`.

   When the release ships a new Agent, tag the Agent first, then move the
   Agent pins in the same branch with the commit that tag points to:

   ```bash
   python3 config/scripts/bump_agent_ref.py <agent-commit-sha> v5.0.0-rc.1
   ```

   It sets `AGENT_REF` in `ci.yml`, the same commit in the second literal
   that `config/scripts/check_release_workflow.sh` requires, the Agent module
   path and version label of the cross-repository E2E build (its `-ldflags`
   in `ci.yml` and in `internal/tests/agente2e/build_test.go`) and
   `config/AGENT_RELEASE`, then runs `check_release_workflow.sh`
   ([the Agent pin](#what-the-tag-pipeline-does)). It refuses a commit that
   is not 40 lowercase hex characters and a tag that is not `vX.Y.Z` or
   `vX.Y.Z-rc.N`, and changes no file unless it finds every pin.
2. Open a pull request (`release: v4.1.0`) and merge it once CI passes.
3. Before you tag, go through the pre-tag checklist:
   - Run the `Signing Key Check` workflow on `main`
     (`gh workflow run signing-key-check.yml --ref main -R AnixOps/anix-control`).
     It proves that the signing secrets are a working pair and match the
     official root in `internal/config/defaults.yaml`; the tag run signs the
     packages, the checksums and the Agent installer with them.
   - Confirm that the latest CI run on `main` (the merged commit) and the
     latest nightly runs (the `ci.yml` schedule, which runs the race
     detector, and `Nightly Security`) are green.
   - Expect live advisories. The tag run's Go Security Scans (`govulncheck`)
     and Frontend Build (`npm audit --omit=dev --audit-level=moderate`) read
     the advisory databases of the day, so an advisory published since the
     last green run fails the tag with nothing changed in the repository. A
     run of `Nightly Security` that day
     (`gh workflow run nightly-security.yml -R AnixOps/anix-control`) shows
     them before the tag does.
4. Tag the merged commit and push the tag:

   ```bash
   git tag v4.1.0 <merge-commit>
   git push origin v4.1.0
   ```

   Without a machine that may push tags, run the `Release Tag` workflow on
   `main` instead (`gh workflow run release-tag.yml --ref main -f tag=v4.1.0
   -f commit=<merge-commit> -R AnixOps/anix-control`). It refuses a commit
   that is not on `main`, an existing tag, or a tree that does not declare
   the version, pushes the tag with the workflow token and, since that push
   starts no workflow, dispatches the CI/CD Pipeline on the tag; the tag gate
   accepts a dispatch on a release tag as it does a push. Agent releases have
   the same workflow in anix-agent (`release.yml` on the tag).

5. Watch the tag run to its end: the release jobs wait for every gate. When a
   job fails on a flake (a runner, network or registry error, not a test that
   fails again), use "Re-run failed jobs" on the run page: the jobs that
   passed keep their results, and the release jobs run once the gates pass.

A suffix (`v4.1.0-rc.1`, `-beta.2`, `-alpha.3`) publishes a prerelease. Only
unsuffixed tags become the latest release.

## Release Branches

When `main` already holds work for the next minor version, release from a
maintenance branch `release/vX.Y` instead. v4.1.0 is the first release cut
this way: `release/v4.1` is cut from the v4.1.0-rc.6 commit, because `main`
already holds v4.2 work.

1. **Cut the branch** from the previous release's tag or commit, with the
   owner's approval:

   ```bash
   git push origin <tag-or-commit>:refs/heads/release/v4.1
   ```

   The branch is protected like `main` (`.github/BRANCH_PROTECTION.md`).
2. **Bump on the branch.** Branch from `release/vX.Y`, run
   `prepare_release.py`, and open the pull request (`release: v4.1.0`)
   against `release/vX.Y`, not `main`. CI runs on pull requests to
   `release/**` with the same required checks.
3. **Tag the release branch commit** that the pull request merged, as above.
   The tag pipeline does not depend on the branch that holds the commit.
4. **Merge back.** Open a follow-up pull request to `main` with the
   release's CHANGELOG section and, when they apply, its version surfaces:
   - the `## X.Y.Z - <date>` section goes directly under `main`'s
     `## Unreleased`, or below a newer release section `main` already has
     (`## 4.2.0-rc.1`, say). The first dated section must stay the version
     `main` declares, since `check_release_version.py` reads the first one;
   - entries that shipped in the release leave `main`'s `## Unreleased`,
     and so do their fragments: delete the `changelog.d/` files that the
     release branch consumed (a fix cherry-picked from `main` has the same
     file name there), or they would be released a second time;
   - the version surfaces come back only while `main` declares an older
     version (`4.1.0-rc.6` → `4.1.0`). Never lower `main`'s version.

**Patch releases** (`vX.Y.Z`, Z > 0) of that line use the same branch. Land
the fix on `main` first, then bring it to `release/vX.Y` in a pull request
(cherry-pick, which carries its changelog fragment). A fix for the old line
only goes to the release branch alone, and its pull request says so. Then
bump, tag and merge back as above.

**`latest` on a superseded line.** An unsuffixed tag becomes the latest GitHub
Release and moves the image's `latest` tag (`make_latest` and `latest=auto` in
`ci.yml`). `scripts/install.sh` does not read it: it installs the tag it is
given. A patch on a line that a newer release has superseded (`v4.1.1` after
`v4.2.0`) would move both back to the older line. The pipeline does not handle
that yet: before tagging such a release, change it to mark only the highest
version `latest`.

## What The Tag Pipeline Does

`ci.yml` runs the full test suite, then:

- **Tag gate.** The tag's version must equal the tree's declared version.
- **Test gates.** Packages, binaries and the image wait for the test and
  policy jobs. The `needs` lists of `plugin-package-publish` and
  `release-binaries` in `ci.yml` name them, and
  `config/scripts/check_release_workflow.sh` fails if one is dropped.
- **Official packages, first.** `plugin-package-publish` builds every package
  under `packages/` (`PACKAGE_SPECS` in `packages/shared/build_package.py` is
  the one list) in the built-in format at the tag's version
  (`build_package.py --all --builtin --formal-release`) and signs it with the
  protected key `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY`, then verifies the set
  (`--verify-release`) and hands it to the binary job as the
  `builtin-packages` artifact. A built-in package holds no binary: Control
  links every package host and embeds the set
  ([`architecture/plugin-kernel-contract.md`](architecture/plugin-kernel-contract.md#built-in-packages)).
  Orders, payments and affiliate are official packages like the others:
  there are no editions and no separate commercial build.
  - The signing root must equal `plugins.official_public_key` in
    `internal/config/defaults.yaml`, since Control only accepts packages signed by
    that root.
  - No Agent plugin or GOST runtime is built for the release: Agent
    dispatch stays off until the Agent release carries them (M4).
  - The pin is `AGENT_REF` in the top-level `env` of `ci.yml`; every
    anix-agent checkout (Plugin Package Release Contracts, Control to Agent
    Process E2E, Cross-Repo E2E) takes its ref from it.
    `config/scripts/check_release_workflow.sh` requires the same commit as a
    second literal (`require_text "AGENT_REF: '<sha>'"`), and the Cross-Repo
    E2E builds the Agent with its module path in `-ldflags`
    (`-X github.com/AnixOps/anix-agent/v<major>/cmd.version=<version>-e2e`, in
    `ci.yml` and in `internal/tests/agente2e/build_test.go`). Move all of them
    to the Agent's release commit with every Agent tag, in the pull request
    that prepares the Control release: `config/scripts/bump_agent_ref.py
    <sha> <agent-tag>` edits exactly these places and `config/AGENT_RELEASE`.
  - `config/AGENT_RELEASE` is the one line that names that release's tag
    (`v5.0.0-rc.1`). The release binaries embed it (`-X main.agentRelease`), and
    it is the Agent a node installs unless `agent_install.agent_version`
    says otherwise. The Agent version is not the Control's: a patch release
    can ship an unchanged Agent, and the Agent repository has no tag with
    the Control's number then. `check_release_version.py` (the tag gate)
    requires the file to hold `vX.Y.Z` or `vX.Y.Z-rc.N`; that the tag exists
    in `anix-agent`, and that `AGENT_REF` is its commit, cannot be checked
    offline: tag the Agent first (the Agent tag is asked for first,
    `docs/architecture/forward-sdk.md` H25), then set both.
  - `plugin-package-release-test` runs `govulncheck` over the four Agent
    commands the plugin binaries are built from and fails for a
    vulnerability their call graph reaches, so a vulnerable Agent
    dependency cannot reach the signed packages.
- **Binaries.** linux/amd64 and linux/arm64 only (the package hosts run as
  Linux processes): each job downloads the `builtin-packages` set into
  `internal/builtinpkg/dist/`, verifies it again against
  `internal/config/defaults.yaml`'s root, and builds with `-trimpath`. Plus the
  frontend archive (`anix-control-frontend.tar.gz`).
- **Image.** `ghcr.io/anixops/anix-control` for linux/amd64 and linux/arm64,
  from the release binaries (the packages are inside them).
  - It is signed with cosign (keyless).
  - It carries SBOM and provenance attestations.
- **GitHub Release.** 12 assets, the files of `release/` that the `release`
  job checks with `verify_release_artifacts.py --require` and uploads:
  - `anix-control-linux-amd64.tar.gz` and `anix-control-linux-arm64.tar.gz`
    (`release-binaries`);
  - `anix-control-frontend.tar.gz`;
  - `official-public-key.raw`, and the Agent installer `agent-install.sh`
    with its signature `agent-install.sh.sig` (`plugin-package-publish`;
    the same file Control serves at `/install.sh`; verify it against the
    official root as in `docs/guide/agent-onboarding.md`);
  - `docker-image.txt` (`docker`);
  - `anix-control-source.sbom.spdx.json`, `RELEASE_NOTES.md`,
    `RELEASE_MANIFEST.json`, `SHA256SUMS.txt` and `SHA256SUMS.txt.sig`.

  No package is a release asset.
  - `SHA256SUMS.txt.sig` is the Base64 raw Ed25519 signature of
    `SHA256SUMS.txt` made with the package signing key, by the same
    `openssl pkeyutl -sign -rawin` step that signs each manifest. It covers
    the Control binaries, and through them every package host and built-in
    package: `scripts/install.sh` verifies it before it trusts a checksum.
  - `RELEASE_MANIFEST.json` lists each built-in package under `packages`
    (id, version, slim artifact size and SHA-256, manifest SHA-256), from
    the verified set (`generate_release_manifest.py --builtin-packages`).

  The release body is the tag's CHANGELOG section.

`config/scripts/check_release_workflow.sh` keeps these essentials in the
workflow. Change it in the same pull request as any release step.

## Dev Builds

Dev builds are the internal test channel: a feature merged to `main` can be
tried on a test Control and test nodes without waiting for the next release.
`.github/workflows/dev-release.yml` cuts one after every green `main` push
CI of this repository, every three hours when `anix-agent`'s `main` moved,
and on a manual run.

- **Version.** The release the tree declares plus `.dev.<run number>`:
  `v5.0.0-rc.15.dev.42` after `v5.0.0-rc.15`. After a stable `vX.Y.Z` it is
  `vX.Y.(Z+1)-alpha.0.dev.<run>`. Both are valid SemVer and order between the
  release before and the next one, so a test node can go on from a dev build
  to the next release (the Agent refuses downgrades). Control and the Agent
  always carry the same dev version.
- **What is built.** The newest commit of each `main` whose push CI passed:
  Control (linux amd64 and arm64, the signed built-in packages embedded,
  `agentRelease` set to the same tag), the frontend, and the Agent from the
  public `anix-agent` repository, packaged and signed as its `release.yml`
  does. All of it is one pre-release of this repository, never marked
  latest; the notes name both commits. Nothing is built when neither
  repository moved (`force` builds anyway). The 15 newest dev builds are
  kept; older ones are deleted with their tags.
- **Installing.** `install.sh update --version <dev tag>` installs Control
  and, because the Agent of a dev tag is in this repository's pre-release,
  fills `agent_install.artifact_dir` from there. The node page's github
  mirror points dev tags here too (`agentinstall.GitHubAgentBase`). The
  manual run with `deploy` does this on the production Control through
  JumpServer, after a database dump (the five newest `pre-dev` dumps and
  configuration backups are kept); `install.sh` restores the previous
  release when the new one fails its health check. Move test nodes with an
  ordinary Agent upgrade campaign to the dev tag.
- **Older Agents.** An Agent before the release that taught it `.dev.N`
  versions (after `v5.0.0-rc.15`) cannot take a dev build: upgrade it to that
  release first.
- **Feature flags.** A feature still being built is registered in
  `internal/features` with the dev stage and checked with
  `features.Enabled(name)`: it runs in dev builds only, so releases cut from
  the same `main` keep it off. `GET /api/v4/kernel/features` lists the
  channel and every gated feature. When the feature is done, move it to the
  stable stage; a release later, remove the entry and the checks.

Dev builds are not releases: `ci.yml`'s tag gate does not take their tags,
no container image is published, and `CHANGELOG.md` does not list them.

## Runner Image

Every Linux job in `.github/workflows/` runs on the pinned label
`ubuntu-24.04`, not `ubuntu-latest`. GitHub moves `ubuntu-latest` to Ubuntu
26.04 in a rollout from 2026-10-19 to 2026-11-19, and while it lasts the label
can resolve to either image from one run to the next, so a release could
break with nothing changed in the repository. The macOS and Windows jobs
still use `macos-latest` and `windows-latest`.

Bump the pin in a pull request of its own, never inside a release pull request:

1. Try the new label on a branch (`sed -i 's/ubuntu-24.04/ubuntu-26.04/'` on
   the workflow files) and run the full lane there: a pull request that edits
   the `changes` or `tag-gate` job of `ci.yml` runs it, or use
   `gh workflow run ci.yml --ref <branch>`.
2. Fix what breaks first. Known today: `postgres-restore-rehearsal` installs
   `postgresql-client-16` with `apt-get`, and Ubuntu 26.04 has no such
   package. The rehearsal must keep using the client of the server's major
   version (16), so take it from the PostgreSQL apt repository rather than
   switching to the unversioned `postgresql-client`, which is 18 on 26.04.
   Also watch the jobs that depend on the kernel and on distribution packages
   (the network namespace, `nft` and `tc` jobs, and the `apt-get` line of the
   Control Center release build).
3. Merge the bump before cutting the tag that needs it. A tag run uses the
   workflow files of the tagged commit.

## Nightly Security Scan

The `Nightly Security` workflow (`.github/workflows/nightly-security.yml`)
runs every night at 03:17 UTC against `main`, and on demand with
`gh workflow run nightly-security.yml -R AnixOps/anix-control`. It exists
because advisories are published against code nobody touched: they would
otherwise fail an unrelated pull request, or be found on release day. A
failed job fails the run, and the failing job names the finding:

- **govulncheck** over every Go module (root, `sdk`, `identity`,
  `control-center`), at the version `ci.yml` pins.
- **gosec**: the production gate exactly as `ci.yml` runs it, plus the `sdk`
  and `identity` gates.
- **npm audit** of `web/` through `npm run audit:check`: shipped and
  development dependencies, so the waivers in `web/audit-allowlist.json` and
  their expiry dates apply here and only here. When a waiver expires, an
  advisory is new or an entry is no longer reported, this job fails the run;
  renew it with the owner's approval or fix the dependency.
- **relay fuzz**: every fuzz target of `sdk/forward/relay` and
  `sdk/forward/relay/link` for 60 seconds each. The corpus the engine learns
  is cached from night to night, and a failing input is uploaded as the
  `relay-fuzz-failures-*` artifact: copy it to `testdata/fuzz/<Target>/` of
  its package to reproduce it with a plain `go test`.

The pull-request pipeline keeps its own `govulncheck` and `gosec` jobs; this
workflow does not replace them. Its required "Frontend Build" check (the audit runs in "Frontend Checks") audits only
the shipped frontend dependencies, strictly (`npm audit --omit=dev
--audit-level=moderate`, no waivers): an advisory in a development tool with no
fix shows up in the nightly run and never blocks an unrelated pull request.

## Verifying A Release

- **Files.** Verify `SHA256SUMS.txt.sig` against `official-public-key.raw`
  (which must equal `plugins.official_public_key` of
  `internal/config/defaults.yaml` at the tag), then run `sha256sum -c
  SHA256SUMS.txt`:

  ```bash
  { printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; base64 -d official-public-key.raw; } > official.der
  base64 -d SHA256SUMS.txt.sig > SHA256SUMS.txt.sig.bin
  openssl pkeyutl -verify -pubin -keyform DER -inkey official.der -rawin \
    -in SHA256SUMS.txt -sigfile SHA256SUMS.txt.sig.bin
  sha256sum -c --ignore-missing SHA256SUMS.txt
  ```

  See also [`UPGRADE.md`](UPGRADE.md#artifact-verification).
- **Image.** Verify the image with:

  ```bash
  cosign verify ghcr.io/anixops/anix-control@<digest> \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/AnixOps/anix-control/'
  ```

- **Packages.** They are inside the binary. At every start Control verifies
  each built-in package against its configured official root and logs the
  key that signed them and its own SHA-256 (`Control executable ... (SHA-256
  ...)`), which equals `sha256sum` of the binary extracted from the verified
  archive.

## History

Up to `v4.0.0`, releases also passed a product-stage contract, a public
rehearsal and a signed evidence bundle with canary and support approvals.
Those gates were retired after `v4.0.0`; that release keeps its evidence
bundle, and its guides still describe it
([`guide/v4-plugin-only-upgrade.md`](guide/v4-plugin-only-upgrade.md)).
