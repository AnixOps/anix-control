# Settings Service (KernelSettings)

Status: in place. The contract is `sdk/api/kernelsettings/v1`, served by
`internal/kernelsettings` on local package bridge sessions and on the mTLS
module listener. Phase 2 of the 2026-10 plan.

## Why

System settings live in `v2_system_config`, a protected kernel table no
package may adopt: its values include secrets (the NodeX token, the SMTP
password), and its writes record audit entries in the protected
`v2_operation_log`. The backup configuration row (`v2_backup_config`) has the
same audit trail, and the kernel's backup service keeps it in memory.

About ten v2 routes stayed bridged only because of this: the e-mail
configuration and test send (notification), the backup configuration update
(platform), the invite configuration update (affiliate) and the NodeX status
and diagnosis (gost-mesh). KernelSettings lets a module read and write the
settings it owns while the kernel stays their only writer.

## Ownership: namespaces

The kernel maps every key to at most one namespace through its own table
(`service.SettingsNamespaces`, `internal/service/settings_namespaces.go`).
Exact keys and key prefixes may not overlap
(`TestSettingsNamespacesDoNotOverlap`). A call names one namespace; a key
outside it is `InvalidArgument`, and a key in no namespace is reachable
through no call.

| Namespace | Keys | Storage | Audit entry per write | Declared secrets |
|---|---|---|---|---|
| `mail` | `notification.email.*` | `v2_system_config` | `system_config`, as the e-mail configuration handler | `notification.email.config` (its JSON holds the SMTP password) |
| `invite` | `invite.*` | `v2_system_config` | `system_config`, as the invite configuration handler | none |
| `nodex` | `forward.runtime.nodex.*` | `v2_system_config` | `system_config`, as the system configuration handler | none (the token is caught by name) |
| `forward-runtime` | `forward.runtime_backend`, `forward.runtime.nodex_mode`, `forward.runtime.ansible.*`, `forward.runtime.iptables_ansible.*`, `forward.ansible.*` | `v2_system_config` | `system_config` | none |
| `backup` | `backup.<field>`: `enabled`, `auto_backup`, `schedule`, `retention_days`, `backup_database`, `backup_files`, `storage_type`, `storage_path`, `s3_bucket`, `s3_region`, `s3_endpoint`, `s3_access_key`, `s3_secret_key` | the first `v2_backup_config` row | `backup_config`, as the backup configuration handler | none (the S3 keys are caught by name) |

Everything else (`security.mfa.config`, the `scheduler.*` markers, site and
subscription settings, any key an administrator creates) is in no namespace.
`forward-runtime` has no holder yet; it fixes who owns the runtime keys next
to `nodex`.

## Capabilities

`kernel.settings.<namespace>.<access>.v1`, checked by
`validateManifestCapabilities` (known namespaces only) and on every call by
`PackageHostOperations.AuthorizeCapability`, against the calling host's
current generation and verified signed release, for official AnixOps
packages only, as for KernelIdentity and KernelSubscriber.

| Access | Methods |
|---|---|
| `read` | `GetSettings` |
| `write` | `PutSettings`, `DeleteSettings` |
| `secrets` | `GetSettings` answers secret values in clear; requires `read` of the same namespace in the manifest |

Holders:

| Package | Capabilities | Why |
|---|---|---|
| notification | `mail.read`, `mail.write`, `mail.secrets` | sends the test e-mail itself, with the stored SMTP password; administrators see it masked |
| affiliate | `invite.write` | writes the frontend settings; it reads them through `kapi_affiliate_settings_v1` |
| gost-mesh | `nodex.read`, `nodex.secrets` | calls NodeX with the shared token |
| platform | `backup.write` | writes the backup configuration; it reads the adopted row |

**A write can be a read.** A namespace that pairs an address with a secret
the kernel sends there (`nodex`: base URL and token; `mail`: SMTP host and
password) leaks the secret to whoever can write the address. Grant `write`
on such a namespace only to a package that may hold its secrets anyway.

## Secrets

A key is secret when the kernel masks it for administrators
(`service.IsSensitiveSystemConfigKey`: token, secret, password,
private_key, api_key, access_key and their variants) or when its namespace
declares it.

- **Reads.** Without `secrets`, a secret answers the placeholder `********`
  when it has a value and `""` when it has none, with `masked: true`.
  `stored`, `has_value` and the row's type, group and remark are always
  answered.
- **Writes.** An entry with `keep`, or a secret whose value is the
  placeholder, leaves the stored value: the placeholder is never stored for
  a secret. Its metadata (type, group, remark) still applies. A kept key
  with no stored value is not created.
- **Masked fields.** A JSON value can hold a secret its key's name does not
  reveal: the SMTP password, field `password` of
  `notification.email.config` (`service.SystemConfigMaskedFields`). The
  kernel's administrator answers (the e-mail configuration and the generic
  system configuration routes) show the value with that field as
  `********` and the rest as stored; the key is not sensitive as a whole,
  so an administrator still edits the value. A write, through the
  contract or the kernel's handlers, that sends the field as the
  placeholder keeps the stored field (`""` when none is stored); the rest
  of the value is written as sent. The notification package reads the
  value in clear (`mail.secrets`) for the test e-mail and masks it in its
  own answer, and sends the placeholder when an update keeps the password.

## Semantics

- **Writes run in the kernel**, in one transaction with:
  - the change itself, through the code the legacy handlers use
    (`SystemConfigService.Set` and `Delete`, `service.LoadBackupConfig` and a
    full-row save);
  - the namespace's audit entries, built by the same functions the legacy
    handlers call (`service.SystemConfigAuditInput`,
    `service.BackupConfigAuditInput`): module `system`, the actor's user id,
    client IP and user agent (cut to the 255-character column), and the
    username: the user's e-mail, looked up by id
    (`service.AuditUsername`), as the legacy handlers look it up when the
    package bridge relays a request with the actor's id only. A
    `system_config` entry names the key, its group and type, whether it is
    sensitive and has a value, whether a stored secret was kept
    (`preserve_existing`) and, for a value with masked fields, those fields
    (`masked_fields`) and the ones set (`masked_fields_with_value`); never
    a value. The e-mail and invite configuration handlers record the same
    entry for the key they write;
  - the request ledger row.
- **Backup writes** read the first row, creating the defaults when there is
  none, apply the fields in the entries, save the whole row and record one
  `backup_config` audit entry naming the S3 keys kept
  (`preserved_sensitive_fields`). With no entries they save the row as it
  is and record the entry, as the handler does for a request that changes
  nothing. Backup fields cannot be deleted.
- **Idempotency.** Every write names a `request_id` (at most 128 bytes).
  `v4_kernel_settings_request` (a new table) records it with its namespace,
  method, calling package and result. A repeat answers the first result
  with `applied: false`; an id used for another namespace, method or
  package is `FailedPrecondition`. Rows are kept 90 days, pruned hourly
  with the subscriber ledger. Modules derive their ids from the request's
  `Idempotency-Key`, else its request id (`platform.backup_config:<digest>`,
  `notification.email_config:<digest>`, `affiliate.invite_config:<digest>`).
  The legacy handlers keep no ledger: a retried legacy write applies again,
  as before.
- **Limits.** At most 64 keys per call, keys of at most 100 bytes without
  spaces or control characters, values of at most 256 KiB without NUL, the
  system configuration handler's types (`string`, `number`, `boolean`,
  `json`, `bool`, `int`).

### In-memory copies

The kernel keeps copies of settings in memory. Every write bumps its
namespace's generation (`service.BumpSettingsGeneration`) after the
transaction commits and before the call returns. A copy remembers the
generation it loaded at, read before it reads the database, and reloads
when it moved, so a copy is never older than its generation. A legacy
writer that keeps its own value as its copy does so only when no other
write came between (`BumpSettingsGenerationAfter`).

| Copy | Namespace | Readers |
|---|---|---|
| `BackupService.config` (the router's system handler) | `backup` | backup creation takes its storage path and type from it |
| `InviteService.config` (the admin and user invite handlers, the identity bridge's) | `invite` | invite code expiry, commission computation. The affiliate module writes the adopted `v2_invite_config` first, then the frontend settings through `invite`, which bumps it |
| `NotificationService.emailConfig` (the notification handler) | `mail` | only the test send, which reloads it first: nothing to refresh |

The generic system configuration handler bumps the namespace of the key it
writes or deletes as well. The NodeX settings, the forward runtime keys and
the subscription link settings are read from the database on every use.
Generations are per process: Control runs as a single replica
(`container-deployment.md`).

## Routes

Moved to native handlers over the contract (`native-flagged`), with byte
parity on SQLite and PostgreSQL against the real KernelSettings server in
process; the parity tests also compare `v2_system_config`, the audit rows
and the kernel's in-memory copies:

| Route | Package | Parity cases |
|---|---|---|
| `GET /api/v2/admin/notification/email/config` | notification | 10 |
| `PUT /api/v2/admin/notification/email/config` | notification | 21 |
| `POST /api/v2/admin/notification/test` | notification | 15, against a test SMTP server |
| `PUT /api/v2/admin/invite/config` | affiliate | 34 |
| `GET /api/v2/admin/forward/nodex/status` | gost-mesh | 21, against test NodeX servers |
| `GET /api/v2/admin/forward/nodex/doctor` | gost-mesh | 21 |
| `PUT /api/v2/admin/system/backup/config` | platform | 14 |

A host whose bridge has no contract connection keeps these routes legacy.
The NodeX probe and the test e-mail now leave from the package host; with
network modules that is the module's network, not Control's.

**Kept bridged: the generic system configuration routes** (platform: list,
get, put, delete `/api/v2/admin/system/configs[/:key]`). They work on any
key of `v2_system_config`. A native version needs a grant over every key,
and that grant is every secret, held at all times:

- the single-key answer returns secrets in clear (all but the masked
  fields), so it needs every namespace's `secrets`;
- a write can point the NodeX or SMTP address anywhere, which reads the
  secret the kernel sends there;
- the routes reach keys other domains own (identity's
  `security.mfa.config`, forward's runtime backend, the scheduler markers).

Bridged, a secret passes through the platform host only in the answer to an
administrator's own request, as the kernel answers it. An `all` namespace
for official packages would remove four bridged routes at the price of a
standing grant over every secret and every domain's settings; it is not
granted. The backup creation, deletion and restore stay kernel-owned
(archives on the kernel's disk).

Forward has no runtime settings write route: its runtime backend is read
through `kapi_forward_runtime_settings_v1`, and the runtime keys change only
through the generic routes and the startup bootstrap.

## Not in scope

- **WatchSettings.** No module caches settings yet; a stream of namespace
  changes can be added as a new RPC.
- **Generations across processes**, for more than one Control replica.
- **The platform adoption of `v2_backup_config`.** Platform adopted the
  table in place (#72), so it can read the S3 keys and write the row
  directly; `backup.write` adds no authority. Reading the configuration
  through KernelSettings instead would let the adoption go.
