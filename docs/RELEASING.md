# Releasing

A release is a `vX.Y.Z` tag on a commit that is already on `go_dev`. GitHub
Actions builds, signs and publishes everything. Nothing is built locally.

## Cut A Release

1. On a branch from `go_dev`, set the version and date the changelog:

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
- **Binaries.** Linux, Windows and macOS builds for amd64 and arm64, plus the
  frontend archives.
- **Image.** `ghcr.io/anixops/anix-control` for linux/amd64 and linux/arm64,
  with the signed identity bootstrap package inside.
  - It is signed with cosign (keyless).
  - It carries SBOM and provenance attestations.
- **GitHub Release.** It publishes the binaries, frontend, signed packages,
  `official-public-key.raw`, the source SBOM, `RELEASE_MANIFEST.json`,
  `SHA256SUMS.txt` and `docker-image.txt`. The release body is the tag's
  CHANGELOG section.

`config/scripts/check_release_workflow.sh` keeps these essentials in the
workflow. Change it in the same pull request as any release step.

## Verifying A Release

- **Files.** Run `sha256sum -c SHA256SUMS.txt`, and see
  [`UPGRADE.md`](UPGRADE.md#artifact-verification).
- **Image.** Verify the image with:

  ```bash
  cosign verify ghcr.io/anixops/anix-control@<digest> \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/AnixOps/anix-control/'
  ```

- **Packages.** Control verifies every package manifest against its
  configured official root on import.

## History

Up to `v4.0.0`, releases also passed a product-stage contract, a public
rehearsal and a signed evidence bundle with canary and support approvals.
Those gates were retired after `v4.0.0`; that release keeps its evidence
bundle, and its guides still describe it
([`guide/v4-plugin-only-upgrade.md`](guide/v4-plugin-only-upgrade.md)).
