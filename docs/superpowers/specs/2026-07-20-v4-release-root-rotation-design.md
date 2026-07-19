# V4 Release Root Rotation Design

## Decision

V4 uses one active Ed25519 official package root. The old root is retired.
Packages signed by the old root must be re-signed and re-imported before they
can be installed under the rotated V4 configuration.

## Scope

- Generate one new Ed25519 private key outside the repository in a directory
  owned by the release operator with mode `0700`.
- Derive one Base64-encoded raw 32-byte public key from that private key.
- Replace `plugins.official_public_key` in the production and example Control
  configurations with the derived public key.
- Store the private key and exact public key as GitHub repository secrets for
  release-tag workflows.
- Verify that a formal V4 package build accepts the new key pair and that the
  configured root equals the repository secret value.

## Key And Secret Boundaries

The PEM private key remains outside Git and is mode `0600`. It is stored in
GitHub only as `ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY`; it is never written to a
repository configuration, release asset, transcript, or approval declaration.

`ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY` contains the exact one-line Base64 raw
Ed25519 public key. The release workflow compares it with
`config/config.prod.yaml` before signing formal V4 packages and again before
signing release evidence.

The canary and support declarations are separate signed, public JSON records.
They are not generated during root rotation because creating either one is an
operational release approval. `ANIXOPS_V4_CANARY_APPROVAL` and
`ANIXOPS_V4_SUPPORT_APPROVAL` are populated only after the corresponding
approved result binds the exact formal sixteen-package artifact cohort.

## Execution Sequence

1. Generate the new private key and derived raw public key on the signing
   workstation.
2. Assert the derived public key exactly equals the proposed configuration
   value before editing repository configuration.
3. Update the three configured root values together.
4. Run configuration and formal package verification against the new key pair.
5. Commit and push the public-root rotation.
6. Upload the private and public key files through `gh secret set` as
   repository secrets.
7. Build the exact V4 package cohort on the release candidate, complete canary
   and support signoff, then generate and upload the two approval records.
8. Create the protected V4 release tag only after all four required secrets
   are present.

## Failure And Rollback

No tag is created during root rotation. Before the first new-root release,
rollback is a source configuration revert and removal of the new repository
secrets. After a new-root release, old-root packages are intentionally not
trusted; recovery requires re-signing the selected package set with the active
root rather than restoring a second active root.

## Verification

- The key file mode is `0600`; the containing directory mode is `0700`.
- The derived raw public key is exactly 32 bytes before Base64 encoding.
- All `plugins.official_public_key` values match the derived public key.
- A `--formal-release` build and `--verify-release` verification succeed using
  the new root.
- GitHub reports the two key secret names without exposing their values.
