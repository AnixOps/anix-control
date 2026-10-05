# Release Root Rotation Runbook

This runbook replaces the official Ed25519 package signing root: the key that
signs every package manifest and whose public half is
`plugins.official_public_key`. It was extracted from the retired 2026-07-20
root-rotation plan and design (`git show 57c62541:docs/superpowers/plans/2026-07-20-v4-release-root-rotation.md`).

## How Control Treats The Root

- Control has exactly **one active root**. At startup `cmd/server/main.go`
  calls `service.EnsurePluginTrustRoot`
  (`internal/service/kernel_service.go`), which records the configured key in
  `v3_kernel_plugin_trust_root` and retires every other root.
- Each admitted release is bound to the fingerprint of the root that signed
  it. A release bound to a retired root no longer verifies
  (`ErrPluginTrustRootRequired`), so its `/api/v2` routes fail closed with
  `503 package_unavailable` and it cannot be installed or updated.
- Releases are immutable per `(plugin_id, version)`. Re-signed packages must
  be registered under a **new package version**; the same version cannot be
  registered twice.
- Rotation is therefore a planned outage for package-served routes on every
  running Control, lasting from the restart with the new root until the
  re-signed package set is imported and enabled.

## When To Rotate

- The private key or the `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY` secret was, or
  may have been, exposed.
- The private key is lost (nothing new can be signed).
- The signing owner changes, or the operator's rotation policy requires it.

Do not rotate while a release tag or a release-tag workflow run is in flight.

## Secrets And Files

| Item | Content | Where |
|------|---------|-------|
| Private key | PEM Ed25519 key | Outside Git in a `0700` directory, file mode `0600`; GitHub secret `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY` |
| Public root | One-line Base64 of the raw 32-byte public key, **no trailing newline** | GitHub secret `ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY`; `plugins.official_public_key` in config |

On a release tag, `.github/workflows/ci.yml` job `plugin-package-publish`
requires `ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY` to equal `official_public_key` in
`config/config.prod.yaml` exactly before signing packages. A mismatch fails the
tag run closed.

The signing secrets can live at the organization level (shared with
`anix-control` and `anix-agent`, which both sign releases with this one key).
A repository-level secret with the same name overrides the organization one,
so delete any stale repository-level copy. `anix-agent` is a public
repository: an organization secret whose visibility is "Private
repositories" does not reach it; use "Selected repositories" or "All
repositories". GitHub never shows a secret again, so **keep your own backup
of the PEM** (a password manager and one offline copy): a lost key forces a
rotation like this one.

The root is also pinned in the Agent installer and in `anix-agent`; replace it
there in the same change:

- `anix-control`: `internal/agentinstall/install.sh` (the raw form and the DER
  form `MCowBQYDK2VwAyEA` followed by the raw form), `internal/config/defaults.yaml`,
  `internal/config/env_test.go`, `docs/guide/agent-onboarding.md`,
  `docs/reference/environment-variables.md`.
- `anix-agent`: `.github/workflows/release.yml` (`ANIXOPS_OFFICIAL_PUBLIC_KEY`),
  `upgrade/upgrade.go` (`OfficialPublicKey`), `scripts/check_release_assets.py`,
  `docs/INSTALL.md`.

Repository surfaces that carry the public root (check with
`git grep -l "<old root>"`):

- `config/config.prod.yaml`, `config/config.yaml.example`,
  `config/config.dev.yaml.example`
- `internal/config/config_test.go` (`TestOfficialPluginAlphaProfiles`)
- `web/src/__tests__/Nodes.test.js`
- `docs/reference/startup-config.md`

## Prerequisites

- A trusted signing workstation with OpenSSL (Ed25519 support), Python 3,
  and the pinned Go toolchain.
- `gh` authenticated with permission to set repository secrets
  (`gh auth status`).
- No release tag on the target commit and no release-tag run in progress:
  `gh run list --workflow ci.yml --limit 20`, `git tag --points-at HEAD`.
- A maintenance window for each running Control (see above).

## Procedure

### 1. Generate the new key pair outside Git

```bash
set -Eeuo pipefail
KEY_DIR="$HOME/.anixops-release/root-$(date +%Y%m%d)"
install -d -m 0700 "$KEY_DIR"
(
  umask 077
  openssl genpkey -algorithm ED25519 -out "$KEY_DIR/official-ed25519.pem"
  chmod 0600 "$KEY_DIR/official-ed25519.pem"
  openssl pkey -in "$KEY_DIR/official-ed25519.pem" -pubout -outform DER \
    | tail -c 32 | base64 | tr -d '\n' > "$KEY_DIR/official-public-key.raw"
)
```

On macOS the system `openssl` is LibreSSL and cannot generate Ed25519 keys.
Install OpenSSL 3 (`brew install openssl@3`), run the commands with
`OSSL="$(brew --prefix openssl@3)/bin/openssl"` instead of `openssl`, use
`"$OSSL" base64 -A` and `"$OSSL" base64 -d -A` for `base64`, and
`stat -f '%Lp'` for `stat -c '%a'`. Back up `official-ed25519.pem` before
uploading it anywhere.

### 2. Verify permissions and pairing without printing the PEM

```bash
test "$(stat -c '%a' "$KEY_DIR")" = 700
test "$(stat -c '%a' "$KEY_DIR/official-ed25519.pem")" = 600
test "$(base64 -d < "$KEY_DIR/official-public-key.raw" | wc -c | tr -d ' ')" = 32
test "$(openssl pkey -in "$KEY_DIR/official-ed25519.pem" -pubout -outform DER | tail -c 32 | base64 | tr -d '\n')" \
  = "$(cat "$KEY_DIR/official-public-key.raw")"
# Fingerprint as Control records it (SHA-256 of the raw key); key ID = first 16 hex chars.
base64 -d < "$KEY_DIR/official-public-key.raw" | sha256sum
```

Record the fingerprint out of band; operators pin it in their trust store
(`ANIXOPS_TRUSTED_OFFICIAL_PUBLIC_KEY` in
[`release-installation.md`](release-installation.md)).

### 3. Replace the public root in the repository (PR to `go_dev`)

1. Branch from `go_dev`. In `TestOfficialPluginAlphaProfiles`
   (`internal/config/config_test.go`) set `officialPublicKey` to the new
   value first, and confirm that
   `GOWORK=off go test ./internal/config -run TestOfficialPluginAlphaProfiles -count=1`
   fails against the old configuration.
2. Replace the old value in every surface listed above.
3. Verify:

   ```bash
   new_root="$(cat "$KEY_DIR/official-public-key.raw")"
   old_root="$(git show origin/go_dev:config/config.prod.yaml \
     | sed -n 's/^[[:space:]]*official_public_key:[[:space:]]*"\([^"]*\)".*/\1/p')"
   for f in config/config.prod.yaml config/config.yaml.example config/config.dev.yaml.example; do
     test "$(sed -n 's/^[[:space:]]*official_public_key:[[:space:]]*"\([^"]*\)".*/\1/p' "$f")" = "$new_root"
   done
   if git grep -q --fixed-strings "$old_root"; then
     echo 'old root still present' >&2; exit 1
   fi
   GOWORK=off go test ./internal/config -count=1
   (cd web && npm test -- src/__tests__/Nodes.test.js)
   ```

4. Commit only these public files, add a `CHANGELOG.md` entry, and open the
   PR. Do not merge it while a tag run is active.

### 4. Prove formal signing with the new root (local)

A formal build requires both Linux platforms; Agent packages also need the
pinned Agent binaries and GOST runtime (see
[`v4-plugin-only-upgrade.md`](v4-plugin-only-upgrade.md)). A Control-only
package is enough to prove the key pair. `--version` must equal the version in
the package's `migrations/index.json`.

```bash
WORK="$(mktemp -d)"
GOWORK=off python3 packages/shared/build_package.py --package knowledge --version 4.0.0 \
  --platform linux/amd64 --platform linux/arm64 --out "$WORK" --formal-release \
  --signing-key "$KEY_DIR/official-ed25519.pem" \
  --official-public-key "$KEY_DIR/official-public-key.raw"
GOWORK=off python3 packages/shared/build_package.py --package knowledge --version 4.0.0 \
  --out "$WORK" --verify-release --official-public-key "$KEY_DIR/official-public-key.raw"
python3 packages/shared/tests/test_manifest_schema.py -v
bash config/scripts/check_release_workflow.sh --self-test
bash config/scripts/check_release_workflow.sh
```

### 5. Merge, then set the key secrets

After the PR is merged:

```bash
gh secret set ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY < "$KEY_DIR/official-ed25519.pem"
gh secret set ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY --body "$(cat "$KEY_DIR/official-public-key.raw")"
gh secret list | grep -E '^(ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY|ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY)[[:space:]]'
```

Never read back or print secret values. For organization-level secrets use
`gh secret set NAME --org <org> --visibility selected --repos anix-control,anix-agent`
(same stdin and `--body` forms).

Then prove the pair in both repositories without releasing anything. The
manual workflow `Signing Key Check` signs a random message with the private
key, verifies it with the public root, compares the root with
`config/config.prod.yaml` (`anix-control`) or `release.yml` (`anix-agent`) and
prints only the public key id:

```bash
gh workflow run signing-key-check.yml -R <org>/anix-control --ref go_dev
gh workflow run signing-key-check.yml -R <org>/anix-agent --ref dev_new
gh run list -R <org>/anix-control --workflow signing-key-check.yml --limit 1
gh run list -R <org>/anix-agent --workflow signing-key-check.yml --limit 1
```

Both runs must succeed and report the same key id (the first 16 hex
characters of the SHA-256 of the raw public key) before a release tag is
pushed.

### 6. Produce and import re-signed packages

1. Cut the next release through the normal process (`docs/RELEASING.md`).
   The `plugin-package-publish` job signs every package with the new root at
   the tag version; the builder stamps that version into each manifest and
   host binary.
2. On each running Control, in the maintenance window: set
   `plugins.official_public_key` in the deployed configuration to the new
   root, deploy the release, then import every package with the new version (**Control > Plugins > Import release**, see
   [`release-installation.md`](release-installation.md)) and update each
   installation to it.
3. If `plugins.identity_bootstrap_package_dir` is set, replace the bootstrap
   `identity-platform` package there with the new-root build before
   restarting.

## Verification

- Key directory `0700`, PEM `0600`; raw public key decodes to 32 bytes; the
  derived public key equals `official-public-key.raw`.
- Every `plugins.official_public_key` equals the new root; the old root no
  longer appears anywhere in the tree.
- `--formal-release` and `--verify-release` succeed with the new pair.
- `gh secret list` shows both key secret names.
- After import: every installation is enabled at the new version and
  `/api/v2` business routes answer (no `503` `package_unavailable` responses).

## Rollback

- **Before any new-root release is deployed:** revert the configuration PR
  and restore the previous secret values (or remove the new ones if the old
  key must not be used again). Nothing else changes.
- **A Control restarted with the new root, old key still trustworthy:**
  redeploy the previous release/configuration. Startup re-activates the old
  root's existing record and retires the new one, so old-root releases verify
  again; new-root releases stop verifying.
- **After a compromise or after a new-root release:** there is no dual-root
  mode. Recovery is re-signing the selected package set with the active root
  and importing it as a new version; never re-enable a compromised root.
