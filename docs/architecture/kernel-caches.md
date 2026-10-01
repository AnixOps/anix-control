# Kernel Caches (KernelTelemetry, subscription summary)

Status: in place. Phase 2 of the 2026-10 plan, item 5 ("cache invalidation
events"). The contracts are `sdk/api/kerneltelemetry/v1` (new, served by
`internal/kerneltelemetry`) and one new method of
`sdk/api/kernelsubscriber/v1` (`internal/kernelsubscriber/summary.go`), on
local package bridge sessions and on the mTLS module listener.

## Why

Two v2 routes stayed bridged only because the kernel answers them from a
cache in its own memory (`internal/cache`, one per Control process):

| Route | Package | Cache | Answer |
|---|---|---|---|
| `GET /api/v2/admin/dashboard` | machine-telemetry | `stats:dashboard`, 60 seconds | counts over `v2_user` (protected), `v2_order`, the four legacy `v2_server_*` tables and `v2_server_log`, the online users (the size of the alive set `alive:users`, kernel memory), and `cached_at` |
| `GET /api/v2/user/subscription` | subscription | `user:subscription:<id>`, 30 seconds, also read by the legacy `GET /api/v2/user/dashboard` | the caller's e-mail, plan, traffic and expiry, the subscription link settings (read on every request, not cached), and `cached_at` |

Both routes take `refresh=true` to rebuild the cached answer. `cached_at` is
when the kernel built it, and for up to 60 or 30 seconds after a change the
cache, not the database, decides the answer. A native answer is
byte-identical only if it has the same entry: the same counts, the same
`cached_at` and the same staleness.

## Design: the kernel keeps the cache (option b, both routes)

Three designs were weighed per route:

| | (a) The module keeps its own cache | (b) The kernel keeps the cache, a typed read answers it | (c) Invalidation events tell a module when to drop its copy |
|---|---|---|---|
| `cached_at` | the module's build time: differs from the kernel's | the kernel's entry, in both modes | the module's build time |
| Staleness | a second 60/30-second window, out of phase with the kernel's | one window | fresher than legacy after a write, so the answers differ |
| `refresh=true` | rebuilds one cache of two; the other side stays stale | rebuilds the one cache | rebuilds one cache of two |
| Summary entry shared with the legacy user dashboard | split in two | shared | split in two |
| Online users | needs a contract read of the count anyway | a count in the answer | needs a contract read of the count anyway |
| Data access | the dashboard needs new views over `v2_user`, the order rows and the legacy server tables | none | as (a) |

Both routes use **(b)**. The kernel keeps its cache. The module reads the
cached answer through a typed contract method that calls the function the
legacy handler calls, on the same cache. In every runtime mode the answer
is therefore the same entry with the same `cached_at`. `refresh=true`
rebuilds the one cache both modes read.

There is nothing to invalidate, because the module holds no copy.
KernelSettings' generation counters (`settings-service.md`, "In-memory
copies") work the other way round: a module writes and the kernel reloads
its own copy. (c) would put a copy in the module, and the kernel has no
event to send it: both caches expire by TTL only, and no write drops them,
whether it comes from a legacy handler or a contract.

Only counts leave the kernel. The online set stays in its memory, and
`online_users` is its size.

## Contracts

| Method | Capability | Holder | Kernel function |
|---|---|---|---|
| `KernelTelemetry.GetDashboard` (new service, `anixops.kerneltelemetry.v1`) | `kernel.telemetry.dashboard.v1` | machine-telemetry | `StatsService.GetDashboardStats` |
| `KernelSubscriber.GetSubscriptionSummary` (new method) | `kernel.subscriber.summary.v1` (new family) | subscription | `StatsService.UserSubscriptionSummary` |

Both are authorized on every call, for official packages whose signed
release declares the capability, against the calling host's current
generation, as the other kernel contracts are. Only new methods, messages
and fields were added (`contracts/proto/descriptors.golden`). A host whose
bridge has no contract connection keeps both routes legacy.

- **`cached_at`** travels as the string `encoding/json` writes for the
  kernel's time: RFC 3339 with its fraction (trailing zeros trimmed) and its
  offset (`Z` for UTC). The module parses it (`time.RFC3339Nano`) and writes
  it back the same way. The parity cases cover a fraction with a `+08:00`
  and a `-05:00` offset, UTC on a whole second, and the kernel's local zone
  in the answers each side builds.
- **`refresh`** is set when the request's first `refresh` value is exactly
  `true`, as with gin's `c.Query`. Any other value answers the cached
  entry.
- **Errors.**
  - A failed dashboard build answers `INTERNAL` with the kernel's error
    text, a database error, which the legacy handler shows the
    administrator after `获取统计失败: `. The module shows the same text.
  - A summary for an unknown subscriber is `NOT_FOUND`, and a user id of 0
    or one beyond 32 bits is `INVALID_ARGUMENT`. The module answers both
    `用户不存在`, as the legacy handler does. Any other failure answers
    `获取订阅信息失败`.
  - Neither a failed build nor an unknown subscriber caches anything.
- **What a holder can read.**
  - machine-telemetry reads the dashboard's counts, which administrators
    already see.
  - subscription can read any subscriber's summary by id: the e-mail, plan
    id and name, traffic, expiry and the link settings. The summary has no
    token, uuid or key; a test checks the answer's field names. The route
    passes the caller's own id.

## Semantics

- **Shadow mode.** A GET in shadow runs both sides.
  - Without `refresh`, both read the same entry. The answers match unless
    the entry expires between the two reads.
  - With `refresh=true`, each side rebuilds, so the answers differ in
    `cached_at` (a counted mismatch) and the snapshot is built twice.
- **Per process.** The caches live in the Control process that serves the
  contract. A network module reads them through the module listener, the
  same as a local host. Control runs as a single replica
  (`container-deployment.md`), like KernelSettings' generations.
- **Unchanged staleness.** No write invalidates either cache, legacy or
  contract. A change shows after 60 or 30 seconds, or on `refresh=true`, as
  before.

Two defects were fixed on the way:

- **The user dashboard showed the link settings.** The summary handler
  wrote the link settings into the cached entry, which the legacy
  `GET /api/v2/user/dashboard` answers too. For up to 30 seconds after a
  summary read, the dashboard showed `subscribe_path` and
  `subscribe_domains`, and concurrent requests wrote the entry while
  others encoded it. `UserSubscriptionSummary` now answers a copy, so the
  dashboard never shows them. identity's native dashboard never did.
- **Network modules could not change memberships.** The module listener did
  not forward KernelSubscriber's membership methods (`GrantSubscriptionGroup`,
  `RevokeSubscriptionGroup`, `RemoveSubscriptionGroupMembers`). A network
  module got `Unimplemented` where a local host was served.
  `TestModuleContractServersForwardEveryMethod` now checks every method of
  every contract the listener serves.

## Parity

Both routes are `native-flagged`. The parity tests run the real kernel
servers in process over gRPC on the native side's database. Each side's seed
starts the kernel's cache afresh. Besides the answers, the tests compare the
cache entry each side leaves: its counts, whether its `cached_at` is the
seeded one or a new build, and, for the summary, that it holds no link
settings.

| Route | Test | Cases (each on SQLite and PostgreSQL) |
|---|---|---|
| `GET /api/v2/admin/dashboard` | `internal/tests/machinetelemetrycompat` | 15 |
| `GET /api/v2/user/subscription` | `internal/tests/subscriptioncompat` | 21 |

The cases cover:

- answers built from the rows, and from none;
- a cached answer, compared byte for byte, `cached_at` included;
- `refresh` set to `true`, `1`, `TRUE`, `True`, empty, `false`, and given
  twice in either order;
- an expired entry, and another user's entry;
- link domains absent or unreadable, and the default subscription path;
- unknown, zero and out-of-range user ids;
- failed builds, with and without a cached entry.

A mutation run (14 mutations of the module handlers, the kernel servers and
the summary copy) failed the tests every time.

## Open

- **Write-through invalidation.** Dropping a subscriber's summary when a
  plan or traffic change commits would make both modes fresher together.
  It changes the v2 answers, so it is a product decision, not a parity fix.
- **identity's native user dashboard.** It computes its summary on every
  request and reads the e-mail from identity. Reading
  `GetSubscriptionSummary` instead would give it the kernel's cache and
  `cached_at` as well. identity-platform would then need
  `kernel.subscriber.summary.v1`.
