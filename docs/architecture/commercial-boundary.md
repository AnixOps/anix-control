# Commercial Boundary

Payment, order, coupon and invite commission are the commercial feature set.
Their source lives in the private `anix-control-commercial` repository
(AGPL-3.0 public repository with a CLA, community edition unlimited). The
public repository holds only the seams below, so it builds, tests and runs as
the community edition on its own. The commercial build checks out this
repository at a pinned commit, overlays the private files on it and builds the
same way. Invite codes (registration gating) stay in the community edition;
only commission, withdrawal and the affiliate package are commercial.

## The Set

`config/commercial-boundary.json` is the source of truth:

- `commercial`: the files that form the set, which exist only in the
  commercial build. Handlers (`order*`, `payment*`, `coupon*`), services
  (`order*`, `payment*`, `invite_commission*`),
  `internal/payment`, `internal/kernelorder` and the packages `payment`,
  `order` and `affiliate`.
- Four of the `commercial` files are the whole link of the set into code that
  is not commercial, so a build without them (the community edition) links
  none of it: `packages/builtin/commercial.go` (the three package hosts,
  `builtin.Register`), `internal/router/router_commercial.go` (the 46
  `/api/v2` routes, run by `SetupContributed` at the end of `Setup`),
  `cmd/server/order_payments.go` (the payment gateways and the reconciler,
  `registerContributedWorker`) and `internal/identitybridge/kernel_order.go`
  (the KernelOrder provider). The v2 route inventory reads every
  `router_*.go` next to `router.go`.
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

## Extension Points

The rest of Control no longer references the set; `allowed_dependents` is
empty. The four couplings that existed were cut like this:

| Was | Now |
|---|---|
| `admin.go` served the order admin routes | the handlers live in `internal/handler/order_admin.go`; the router binds them as before |
| the dashboard counted `model.Order` | `service.RegisterDashboardContributor`; `order_stats.go` fills the order and income figures |
| `NotifyOrderPaid(model.Order)` sat in the notification service | the method moved to `order_notification.go` |
| registration checked `model.InviteCode` in two places (`auth_service.go`, `kernelidentity`) | `service.RedeemInviteCode` calls the redeemer that `invite_registration.go` registers with `RegisterInviteRedeemer`; without one, a registration with an invite code is refused |

## The Commercial Routes Are Native

The 43 `/api/v2` routes of `affiliate` (10), `order` (13) and `payment` (20)
are extraction mode `native`: their legacy handlers (`internal/handler`
`order*`, `coupon*`, `admin_coupon*`, `payment*`, `invite.go`) are deleted,
`router.go` binds the bare gateway, and the package host answers alone. They
were already on the rehearsed route list
(`config/package-route-defaults.json`) and ran natively in every install;
the legacy handlers were only the fallback. The legacy-versus-native parity
tests (`internal/tests/ordercompat`, `paymentcompat`, and the parity tests of
`affiliatecompat`) went with them, since the oracle is gone; the package's own
tests (`packages/*/native`) and the three `affiliatecompat` lock tests stay.

What the kernel keeps of the set is what it still serves itself: the services
(`OrderService`, `PaymentGatewayService`, `InviteService`) behind
`KernelOrder`, the payment reconciler and invite-code administration, and the
models (the schema stays in the kernel). They move with the set.
