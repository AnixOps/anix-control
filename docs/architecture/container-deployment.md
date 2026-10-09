# Container Deployment And The Road To Multiple Replicas

Date: 2026-09-30

Containers are the primary way to run Control. The systemd installer installs
the release tag you name (it never resolves "latest"). Operator steps are in
[`../DEPLOYMENT.md`](../DEPLOYMENT.md); this page records the design and what
still blocks running more than one replica.

## What A Control Container Is

| Concern | Design |
|---------|--------|
| Image | `ghcr.io/anixops/anix-control`, built by CI from the release binary, frontend and signed `identity-platform` bootstrap package (`linux/amd64`, `linux/arm64`, SBOM, provenance, cosign keyless signature). `Dockerfile` target `source` builds from a checkout for development and edge images. |
| Configuration | No file needed: built-in production defaults plus `ANIX_CONTROL_*` variables, secrets as `ANIX_CONTROL_*_FILE`. `anix-control -print-env` lists everything. A YAML file still works (`ANIX_CONTROL_CONFIG`). |
| State | None in the container. All state is in PostgreSQL. Verified package copies (about 0.5 GB for the official package set: each `.anxp` is copied whole because hosts re-verify its digest) and host sockets and binaries are rebuilt from the database on demand. Compose keeps the package copies on the `plugin-artifacts` volume (`/var/lib/anixops`, disk) and the rest in the `/tmp` tmpfs; Kubernetes uses a disk-backed `emptyDir` at `/tmp`. |
| Schema | `anix-control migrate` (Compose one-shot service, Helm init container) and the server both run the same preparation under a PostgreSQL advisory lock. Production never alters existing tables: an empty database gets the full schema, an existing one only missing tables. |
| Health | `/livez` (liveness), `/readyz` (startup done, not draining, database ping), `/health` (legacy). On `SIGTERM` readiness fails first, the listeners stay open for `server.shutdown_drain_delay`, then the ordered shutdown runs (allow 90 s). |
| Security | uid 10001, read-only root filesystem, all capabilities dropped, `no-new-privileges`, `tini` as PID 1. `/tmp` must be writable and allow `exec` (package host binaries). |
| Logs | `log.format: json` writes JSON lines to stdout. |
| Time | Business-day boundaries (traffic resets) use the process time zone: set `TZ` (the image defaults to `Asia/Shanghai`). The PostgreSQL session time zone is `database.timezone`. |

Deployment assets:

- `docker-compose.prod.yml` + `config/deploy/compose/`: Control only, external PostgreSQL.
- `config/deploy/helm/anix-control`: Kubernetes, one replica, `Recreate`.
- `docker-compose.yml`: development stack with a throwaway PostgreSQL.

## Single-Instance Work Is Lease-Guarded

Some background work must run in exactly one process per database. It runs
only in the process that holds the `control.singleton-workers` lease
(`v4_kernel_lease`, `internal/lease`):

- the forward runtime job executor and forward agent bridge worker (they
  requeue interrupted jobs when they start, which is safe only for the single
  executor);
- the forward flow reset and node monthly reset workers (also idempotent per
  calendar day through `scheduler.*.last_day` markers);
- the gost and ansible stats workers (the traffic cursor is also row-locked);
- the forward latency prober.

The lease lasts 30 s and is renewed every 10 s. A leader that cannot renew
stops the workers after 20 s, before anyone else can take over; a process that
shuts down releases the lease so the next one takes over at once.
`anixops_lease_leader{lease=...}` on `/metrics` shows which process leads.

This makes a rolling update or an accidental second process safe for these
workers. It does not make Control multi-replica.

## Still Single-Instance (Next Round: HA)

The Helm chart refuses `replicaCount > 1` and uses `Recreate` until these are
done:

| Area | Why it blocks replicas | Direction |
|------|------------------------|-----------|
| Package hosts | `/api/v2` routes dispatch to host processes started only in the process that ran the lifecycle operation (`internal/pluginhost`, `internal/plugincontrol`). Other replicas answer `package_unavailable`. | Every replica reconciles and starts the enabled hosts itself; lifecycle operations stay durable and single-claimed. |
| Agent gRPC streams and agent WebSockets | `AgentControlManager` and `AgentHandler` keep connections in memory; admin actions on another replica see the node as offline. | Record stream ownership (node, replica) in the database and forward admin requests to the owning replica. |
| Plugin operation dispatcher | `recoverOnce` resets every dispatching/running node operation at start, including another replica's. Dispatch is off by default. | Record the dispatching replica and recover only operations of replicas whose heartbeat expired. |
| Online users / device limits | `cache.InitMemory` backs UniProxy alive lists; each replica sees a fraction. | Shared store (PostgreSQL table or Redis). |
| Login and registration rate limits, per-IP limiters, the users' subscription reset limit (legacy handler) | In-memory; N replicas allow N times the attempts. Once identity is authoritative, identity-platform serves login and the subscription reset with counters in its shared `throttle` table. | Shared counters. |
| Backups | Written to the local filesystem; database backups are SQLite-only. | Object storage, `pg_dump`-based. |
| Short caches (dashboard 60 s, subscription 30 s) | Per replica. | Acceptable; TTL-bounded. |
