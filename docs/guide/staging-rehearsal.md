# Staging Rehearsal Of Route Cutovers

How to rehearse moving the v2 Control package routes from `legacy` through
`shadow` to `native` on a local staging stack, one batch at a time, and how
to read the report the owner signs off (roadmap v4.1.0, R4/R5; decisions H4,
H6, H7 and H8).

The tooling is in `scripts/staging/`:

| File | What it is |
|---|---|
| `rehearse.sh` | the runner: `up`, `--batch N`, `--rollback`, `routes`, `status`, `logs`, `down` |
| `compose.yml` | the stack: PostgreSQL, the rehearsal Control, two reconciliation twins, the tool |
| `stagingctl/` | Go tool run inside the stack: package install, seeder, replayer, reconciler, reports |

## Rules

- **Synthetic data only (H4).** Nothing reads a backup or a production
  database. The seeder writes made-up users, plans, orders, tickets and nodes;
  addresses are from the documentation ranges `192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24` and `example.com`/`.org`/`.net`.
- **No route out.** Every container is on a Compose network with
  `internal: true`: background workers and handlers that would call
  Telegram, SMTP, payment providers, NodeX or nodes fail at once, the same way
  on every instance. Nothing is published on the host.
- **The rehearsal never switches the rehearsal instance to `native`.** It
  puts a batch's read routes in `shadow` and prints the command for `native`,
  which an operator runs only after the owner signs the batch off (H7). The
  only instance that runs a batch natively is the throwaway twin
  `recon-native`, on a copy of the seeded database.

## Batches

| Batch | Packages | Parity suites (`internal/tests/…`) |
|---|---|---|
| 1 | knowledge, ticket | knowledgecompat, ticketcompat |
| 2 | notification, platform, machine-telemetry, protocol-runtime | notificationcompat, platformcompat, machinetelemetrycompat, protocolruntimecompat |
| 3 | plan, order, payment, affiliate | plancompat, ordercompat, paymentcompat, affiliatecompat |
| 4 | subscription, forward, proxy-node, gost-mesh, wireguard | subscriptioncompat, forwardcompat, proxynodecompat, gostmeshcompat, wireguardcompat |

The roadmap named twelve packages; the four others with `native-flagged`
routes go where their routes belong: machine-telemetry (the admin dashboard
and traffic reports) and protocol-runtime (agent task and protocol template
reads) with platform, gost-mesh (NodeX status and connection tests) and
wireguard (key pair generation) with forward and proxy-node.
identity-platform is in no batch: identity group A moves only with the
identity cutover. `stagingctl batches` prints the assignment with route
counts and fails if a package with `native-flagged` routes is in no batch.

## Pass Criteria (H6)

A batch passes when all of these hold:

- **Read routes** (GET, `native-flagged`): each has at least
  `--min-requests` (default 200) requests compared in shadow by its package
  host, 0 mismatches, 0 native errors and has been in shadow for at least
  `--min-shadow-duration` (default 2h). The counts are the host's own shadow
  counters (`GET /api/v4/kernel/route-modes`); mismatches also count the
  stored samples (`GET /api/v4/kernel/route-modes/mismatches`) observed
  since the route entered shadow, so a host restart cannot hide one. The
  shadow start is the route's latest switch to `shadow` in the revision
  history, so a batch may be rehearsed in several runs.
- **Write routes** cannot run in shadow. They pass through
  - the batch's packagecompat parity suites, and
  - the **write reconciliation**: two twins start from the same copy of
    the seeded database (`staging_seed`), `recon-legacy` with every route
    `legacy` and `recon-native` with the batch's routes `native`. The same
    write requests (seeded ids, negative cases, permission errors) go to
    both in the same order. Their answers must match (status and body, with
    the volatile values listed per request masked), and afterwards the
    batch's tables must be equal row by row, except for the ignored columns
    the report lists per table with the reason (timestamps the handlers take
    from their clock, random tokens and the like).
- every route of the batch has a request generator.

## Requirements

Docker with Compose v2, Go (the version in `go.mod`), Python 3, `openssl` and
`curl` on the host. The stack runs on the existing `postgres:16-alpine` and
`gcr.io/distroless/static-debian12:nonroot` images and builds no image:
Control and `stagingctl` are static binaries built from the checkout and
mounted into the containers. About 1 GB of disk for the work directory and
the database volume, 2–3 GB of memory while the twins run.

## Bring The Stack Up

```bash
scripts/staging/rehearse.sh up
```

It builds Control and `stagingctl`, builds and signs every package at the
Control version (commercial edition, so batch 3's order, payment and
affiliate are included) with a throwaway key that the stack trusts, starts
PostgreSQL and Control (`app.edition: commercial`), registers, uploads and
enables every package as `UPGRADE.md` ("Package Install Window") describes,
seeds the synthetic data and snapshots the seeded database as the template
`staging_seed`.

- Work directory: `scripts/staging/.work` (ignored by git), or
  `STAGING_WORK`. It holds the binaries, the packages, the generated
  secrets (`staging.env`), the seed manifest and the reports.
- Seed: `STAGING_SEED` (default `20261002`). The same seed gives the same
  data set: personas, ids, names and amounts. The traffic rows the
  telemetry reports read are dated relative to the time of seeding.
- `STAGING_PACKAGES=knowledge,ticket` builds and installs only those
  packages (and identity-platform); CI uses it for batch 1.

The seeded personas (`stagingctl/specs.go`): the bootstrap super
administrator, a second administrator, a staff member, two members with
plans, orders, tickets and invitations, a member without any data, a banned
and an expired member (their logins are refused, so the tool signs tokens
for them as if they had logged in before), plus about 80 other members.

## Rehearse A Batch

```bash
scripts/staging/rehearse.sh --batch 1                 # H6 thresholds: 200 requests, 2h
scripts/staging/rehearse.sh --batch 1 --smoke         # 20 requests, 2 minutes (CI)
scripts/staging/rehearse.sh --batch 2 --min-requests 300 --min-shadow-duration 3h
```

Steps:

1. switch the batch's `native-flagged` read routes to `shadow` on the
   rehearsal instance, as the super administrator (audited; routes already
   in shadow keep their start time);
2. replay read traffic: every route's request variants in turn (members,
   administrators, staff, anonymous callers; pagination, filters, empty
   results, unknown and invalid ids, permission errors), by default spread
   so the required requests cover the required time (`--rate` sets requests
   per second instead), until every route meets the thresholds; then read
   the counters and the mismatch samples;
3. run the batch's parity suites on the host (`go test`, SQLite; also
   PostgreSQL when `STAGING_PARITY_POSTGRES_DSN` holds a DSN whose database
   name contains `test`);
4. write reconciliation: create `recon_legacy` and `recon_native` from
   `staging_seed`, start the twins, switch the batch to `native` on
   `recon-native` only, replay the write requests on both, compare answers
   and tables, then remove the twins and their databases;
5. write `reports/batch-N/report.md` and `report.json` and print the
   result.

`--skip-parity` and `--skip-reconcile` leave a step out (the report then
fails for the missing result). The exit status is 0 only for PASS.

## Read The Report

`reports/batch-N/report.md` (and the same data in `report.json`):

- **Result** PASS or FAIL, and every reason for a FAIL.
- **Read routes:** per route the requests sent by this run, the requests the
  host compared since it started, mismatches, native errors, skipped shadow
  runs (all shadow slots busy), time in shadow and the samples API link; the
  status codes the replayer received; for routes with mismatches up to five
  samples with the request id and the JSON paths that differ. The admin page
  插件中心 → 路由模式 shows the same counters and samples.
- **Write routes:** per route the replayed requests and those whose answers
  differ, with a structural diff of each difference.
- **Table reconciliation:** per table the row counts on both twins, how many
  rows the replay changed against the seed, the differences with samples,
  and the ignored columns with the reason.
- **Parity suites:** each suite's result and time.
- **Next step:** for PASS, the exact commands that switch the batch to
  native on the rehearsal instance, for after the sign-off; the rollback
  command.

## Sign-Off And Native

Only after the owner signs the batch off (H7), run the commands the report
prints, for example:

```bash
scripts/staging/rehearse.sh routes set --package knowledge --mode native \
  --reason 'batch 1 signed off (H7): staging rehearsal passed' --yes
```

`rehearse.sh routes …` runs `anix-control routes …` inside the rehearsal
Control container (actor `system/cli`); `routes list` and `routes history`
show the state. Packages whose batch passes are the candidates for
"native by default" (H8). The four batches of R5 passed and were signed off,
so from 4.1.0 their 151 routes are native by default
(`config/package-route-defaults.json`): on a fresh stack built from 4.1.0,
`rehearse.sh routes list` shows them `native` with `SOURCE default` before
any switch. To rehearse from the 4.0 baseline (every route legacy), bring
the stack up with `STAGING_ROUTE_DEFAULT_MODE=legacy scripts/staging/rehearse.sh up`;
the write reconciliation's legacy twin always runs with
`package_routes.default_mode: legacy`. A route joins the default set only by
an explicit change of that file and of
`internal/service/testdata/rehearsed-routes.json`, after its batch passed.

The rehearsal Control runs the shipped `agent_control.mtls` default:
`required` from v4.2. The stack has no gRPC listener and no agents, so the
log carries a warning that no Agent can enroll; the rehearsed routes do not
use the legacy agent paths. `STAGING_AGENT_CONTROL_MTLS=preferred
scripts/staging/rehearse.sh up` rehearses with the 4.1 behaviour.

## Roll Back

```bash
scripts/staging/rehearse.sh --batch 1 --rollback
```

Returns every package of the batch to `legacy` in one revision per package
(`POST /api/v4/kernel/route-modes/rollback`). Hosts apply it within about 5
seconds.

## Tear Down

```bash
scripts/staging/rehearse.sh status          # containers and disk
scripts/staging/rehearse.sh logs control    # the last 200 lines
scripts/staging/rehearse.sh down            # containers, network and database volume
scripts/staging/rehearse.sh down --purge    # also the work directory
```

`down` keeps the built binaries and packages (and the reports) so the next
`up` starts quickly; the secrets and the seed manifest go with the database.

## CI

The job "Staging Rehearsal Smoke" runs `up` with `STAGING_PACKAGES=knowledge,ticket`
and `--batch 1 --smoke` in the full lane only (go_dev pushes, the nightly
schedule, manual runs and `ci:full` PRs). It is not a required check. The
reports are uploaded as the artifact `staging-rehearsal-batch-1`.

## Limits

- Control rate-limits each client address (administrator routes: 10
  requests per second). All traffic comes from the tool container, so the
  tool waits out a `429` and retries the request; `429` answers never become
  part of a result.
- Requests a kernel middleware answers (401, 403 before the package) never
  reach a package host, so they are replayed but not compared; the
  compared count is what the thresholds use.
- Both twins run Control's background workers on their copy (order expiry,
  retention, statistics). They act on identical data, but one may run a few
  seconds before the other; a table a worker changes during the replay can
  differ for that reason. The report shows the row and column; rerun the
  batch before reading it as a parity bug.
- Handlers that reach outside (test e-mail, Telegram, payment providers,
  connection tests) fail at once on both twins, so only their error paths
  are compared.
