# Commercial Boundary

Payment, order, coupon and invite commission are the commercial feature set.
They are planned to move to the private `anix-control-commercial` repository
(AGPL-3.0 public repository with a CLA, community edition unlimited; ships
after 5.0.0). This page records where the set ends today and how the gate keeps
it separable. It changes no behavior: Control is still one product and every
official package is built in (`docs/RELEASING.md`).

## The Set

`config/commercial-boundary.json` is the source of truth:

- `commercial`: the files that form the set. Handlers (`order*`, `payment*`,
  `coupon*`, `invite*`), services (`order*`, `payment*`, `invite*`),
  `internal/payment`, `internal/kernelorder` and the packages `payment`,
  `order` and `affiliate`.
- `wiring`: files that may reference it because they only connect it: models
  (the schema stays in the kernel), the router, migrations, `cmd/` and
  `packages/builtin`.
- `symbols`: the services and models that identify the set.

Plans (`plan` package, `model.Plan`) are not in the set. A plan is also the
traffic and group entitlement the subscription path reads
(`internal/subscriber`, `service/subscriber_entitlements.go`), which the
community edition needs. Only the purchase of a plan is commercial.

## The Gate

`config/scripts/check_commercial_boundary.py` (its unit test runs in the Go
Quality Gates script tests) fails when a non-test Go file outside the set uses
a commercial symbol and is not in `allowed_dependents`. The list is a ratchet:
a dependency that disappears must be removed from it, and a new one needs the
owner's approval.

## Remaining Couplings

These five files still reach into the set. Each has to go behind a neutral
extension point before the move:

| File | Coupling | Extension point |
|---|---|---|
| `internal/handler/admin.go` | the order admin routes use `OrderService` | the order routes register from the package, not the admin handler |
| `internal/service/stats_service.go` | dashboard counts `model.Order` | a statistics provider the order package fills |
| `internal/service/notification_service.go` | `NotifyOrderPaid(model.Order)` | an event the order package emits |
| `internal/service/auth_service.go`, `internal/kernelidentity/server.go` | registration checks `model.InviteCode` | an invite-code validator the affiliate package provides |

## Why The Legacy Handlers Stay

Every `/api/v2` route of these packages is `native-flagged`: the package host
has a native implementation and the kernel's legacy handler stays as the
fallback for every route mode (`docs/architecture/package-extraction.md`). A
route becomes `native`, and its legacy handler is deleted, only through the
rehearsal and owner sign-off that the extraction document defines. That
deletion therefore belongs to the extraction milestones, not to the boundary
work, and the move to the private repository follows it.
