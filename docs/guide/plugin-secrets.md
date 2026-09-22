# Plugin Secret Operations

Plugin credentials are stored as immutable encrypted file bundles. Control
keeps ciphertext in the database and reads the AES-256-GCM keyring only from
the deployment configuration. Agent receives decrypted bytes only inside an
authenticated operation envelope and materializes them as private files.

## Production configuration

Generate a 32-byte key outside the repository:

```bash
openssl rand -base64 32
```

Place the result in the deployed `config/config.yaml`:

```yaml
plugins:
  secret_encryption:
    active_key_id: "2026-09"
    keys:
      2026-09: "BASE64_ENCODED_32_BYTE_KEY"
```

Do not commit the populated file. Production startup requires this keyring
when `plugins.topology_execution_enabled` is true. Secret API calls and v2
operation dispatch also fail closed when the keyring is absent or invalid.

## Create and reference a bundle

Open **Control > Secrets**, create a lowercase Secret ID, and upload one or
more credential files. The UI and API return names, sizes, SHA-256 digests,
versions and audit records; they never return file contents, ciphertext or
nonces.

Plugin config values use an exact file reference:

```text
secret://mesh-edge@1/ca.pem
secret://mesh-edge@1/client.pem
secret://mesh-edge@1/client.key
```

A secure topology edge uses the matching bundle reference:

```json
{"secret_id":"secret://mesh-edge@1"}
```

Both endpoint vertex configs must reference at least one file from that exact
bundle version. Control verifies every referenced file at revision creation
and again when compiling a deployment plan.

## Rotation and rollback

1. Add a new version in **Control > Secrets**. Existing versions are immutable.
2. Create a new topology revision whose endpoint configs and edge `secret_id`
   all reference the new version.
3. Preview, apply to the approved canary, and verify plugin health.
4. Roll back by applying the previous topology revision if validation fails.
5. Delete an old version only after no topology, operation, deployment step or
   plugin configuration references it. Active and referenced versions are
   rejected by the API.

To rotate the database encryption key, add a new key ID and make it active.
New Secret versions use the new key. Keep every retired key in `keys` while a
stored version still names it. Removing an old key first makes those versions
undecryptable and dispatch fails closed.

## Agent storage and audit

Agent writes material under:

```text
<PluginRoot>/nodes/<node_id>/<plugin>/<version>/private/secrets/<secret-id>/<secret-version>/<file>
```

Directories use mode `0700` and files use `0600`. Symlinks, non-regular files,
permission widening and content changes for an immutable reference are
rejected. A successful replacement removes stale material; a failed lifecycle
operation restores the previous config and removes files created by that
attempt.

Control records create, version, delete and dispatch metadata. Agent journals
operation-bound references and SHA-256 digests. Neither audit contains secret
content. Database backups require the matching keyring to restore encrypted
Secret versions; back up the deployed config through the operator's protected
credential process.
