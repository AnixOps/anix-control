# Order Service (KernelOrder)

Status: in place. The contract is `sdk/api/kernelorder/v1`, served by
`internal/kernelorder` on local package bridge sessions and on the mTLS
module listener. Phase 2 of the 2026-10 plan (class D, cross-domain
transactions: "支付回调完成订单").

## Why

A paid payment callback changes three domains in one kernel transaction:

- the payment record and its gateway's statistics (`v2_payment_record`,
  `v2_payment_gateway`), which the payment module adopted;
- the order (`v2_order`: paid, then completed), which the order module
  adopted;
- the buyer's plan (`v2_user`, `v2_user_subscription_group`), which only
  the kernel writes (`subscriber-service.md`), with request id
  `order:<order id>` and the change log.

No module may write all three, so the four callback routes
(`POST /api/v2/payment/callback/:type`, `/payment/x402/callback`,
`/payment/stripe/webhook`, `/payment/paypal/webhook`) stayed bridged.
KernelOrder lets the payment module serve them: it writes its own rows, and
asks the kernel to apply the paid record to the order.

## Contract choice: a new service

`KernelOrder.CompleteOrderPayment` is a new service in a new proto package
(`anixops.kernelorder.v1`), not a KernelSubscriber method.

- **KernelSubscriber is about subscriber state.** Its callers send the
  change (a plan snapshot, a traffic batch) and the kernel applies it to
  `v2_user`. Completing a paid order is a decision the kernel makes from two
  other modules' rows: whether a payment record pays an order, then the
  order's status and the plan it grants. Putting it into KernelSubscriber
  would make that contract know orders and payments.
- **Separate authority.** The capability `kernel.order.complete.v1` lets a
  package pay orders and nothing else. A holder of
  `kernel.subscriber.entitlements.v1` (order, plan) cannot mark an order
  paid through it, and payment cannot grant an arbitrary plan: the kernel
  reads the plan from the order.
- **It can move.** If the order module later takes over its writes, it can
  serve the same contract itself without a KernelSubscriber change.

It follows the KernelSubscriber pattern: authorization per call, only new
calls and messages (the proto golden file grows), the contract in the CI
generated-code check and the change classifier.

## Capability

`kernel.order.complete.v1`, checked by `validateManifestCapabilities` and on
every call by `PackageHostOperations.AuthorizeCapability`, against the
calling host's current generation and verified signed release, for official
AnixOps packages only.

| Package | Capability | Why |
|---|---|---|
| payment | `kernel.order.complete.v1` | its callbacks complete the orders their payments pay |

## CompleteOrderPayment

The request names a payment record by `trade_no` and the order it names,
`order_id` (a cross-check). The kernel reads the record itself; it trusts
nothing else the caller says. In one transaction,
`service.CompleteOrderPaymentTx`:

1. locks the record (`FOR UPDATE`) and looks up its request id
   `payment:<trade_no>` in the request ledger: a repeat answers the first
   outcome with `applied: false`;
2. requires the record to be paid and to name `order_id`; otherwise the
   call fails (`FAILED_PRECONDITION`, `NOT_FOUND` for no record) and records
   nothing, so a later call may apply;
3. locks the order and re-checks that the record pays it (`recordPaysOrder`,
   the #73 guard): the order exists, it is the record's user's, the record's
   `amount` covers its total to the cent, and it is still pending. Otherwise
   the order is left unchanged and the outcome is `REFUSED`, with a reason;
4. marks the order paid (`status = 1`, `paid_at`);
5. completes it in a savepoint (`completeOrderTx`): grants its plan with
   request id `order:<order id>`, then `status = 3`. If the grant fails, the
   order stays paid for an administrator and the outcome is `PAID`;
6. records the outcome under `payment:<trade_no>` (method
   `complete_order_payment`, the record's user, `{"order_id", "outcome",
   "reason"}`).

| Outcome | Order | Plan |
|---|---|---|
| `COMPLETED` | paid, then completed | granted once (`order:<id>`) |
| `PAID` | paid | not granted; an administrator completes it |
| `REFUSED` | unchanged | unchanged; the payment stays recorded |

| Reason (`REFUSED`) | When |
|---|---|
| `the order does not exist` | the record names no order row |
| `the order is another user's` | a record created before #73 named another user's order |
| `the payment is below the order's total` | `round(amount × 100) <` the order's total in cents |
| `the order is not pending (status N)` | the order was cancelled, completed, or paid by another payment after this one was created |

The ledger is `v4_kernel_subscriber_request`, kept 90 days. A repeat after
that applies the checks again and finds the order no longer pending, so it
changes nothing. An id already held by another method is
`FAILED_PRECONDITION`, not a replay. The kernel's own legacy callbacks call
the same function in their transaction, so both write identical rows.

## The payment module's two steps

A paid callback in `packages/payment/native` (after the provider's
signature, and its amount or token checks):

1. **Record the payment.** In one transaction on the package's adopted
   tables: lock the record, require it pending, mark it paid (status,
   gateway trade number, notification, `paid_at`) and add it to its
   gateway's statistics. Commit.
2. **Complete the order.** `KernelOrder.CompleteOrderPayment(trade_no,
   order_id)`, for a record that names an order.

Every repeat of a callback whose record is already paid runs step 2 again,
then answers as the kernel answers a processed payment.

- **No order is paid without a paid record.** Step 2 reads the record in the
  kernel's transaction and refuses one that is not paid; step 1 commits
  before step 2 starts.
- **No paid record is lost.** Step 1 commits on its own and step 2 never
  writes the record.
- **A repeat converges.** Step 2 is idempotent per trade number and step 1
  refuses a record that is no longer pending, so a repeat counts the payment
  once and completes the order once.

| Failure | State left | What follows |
|---|---|---|
| before step 1 commits | nothing written | the provider delivers again |
| crash after step 1 | record paid, order pending | no answer was sent: the provider delivers again; the repeat finds the record paid and runs step 2. If it never does, the reconciler runs step 2 |
| step 2 fails (kernel unreachable, `Unavailable`, `Internal`) | record paid, order pending | the handler fails, the gateway answers 502, the provider delivers again. If it never does, the reconciler runs step 2 |
| step 2 refuses | record paid, order unchanged | final: the callback answers as the kernel's does |
| the grant fails inside step 2 | order paid | outcome `PAID`, logged, as before |

EPay delivers again until it reads `success`, Stripe and PayPal after any
non-2xx answer. A provider that never delivers again would leave a paid
record and a pending order; the kernel's reconciler (below) runs step 2 for
it. The kernel's legacy callbacks also run step 2 on a repeat of a paid
payment (`PaymentGatewayService.FinishPaidOrder`), so a route can switch
between legacy and native at any time, even between the two steps.

## The reconciler

`service.OrderPaymentReconciler` is a kernel worker that runs step 2 for a
paid record whose callback did not. Every Control process starts it with
the other kernel workers (`cmd/server/order_payments.go`); it runs once at
start, then every five minutes, and stops with the process. It stays in the
kernel for the reason step 2 does: completing an order grants its plan,
which only the kernel writes; the plugin-only worker gate lists it among
the kernel workers.

A run reads, in batches of 100 by record id and at most 10 batches, the
records that:

- are paid (`v2_payment_record.status = 1`);
- name an order that exists and is pending (`v2_order.status = 0`);
- were paid more than two minutes ago (the grace: the live callback runs
  step 2 right after step 1 commits, and the reconciler stays out of it);
- were paid within the request ledger's 90 days;
- have no row under `payment:<trade_no>` in the request ledger.

Each record is applied in its own transaction with
`CompleteOrderPaymentTx(trade_no, order_id)`, the function step 2 calls: the
same request id, row locks and checks (the record's user, a pending order,
the amount covering its total), the same outcomes and the same ledger rows.
A larger backlog continues on the next run.

| Record | Outcome |
|---|---|
| pays its pending order | `COMPLETED` (order paid and completed, plan granted with `order:<id>`), or `PAID` if the grant fails, as for a callback |
| fails the checks | `REFUSED`: the reason is recorded and logged, the order is left unchanged, and the record is not read again |
| applied by a callback, or by another process, while the run read it | `applied: false`; nothing changes |
| paid within the grace, more than 90 days ago, or with an outcome already | not read |

- **Idempotent.** The reconciler writes no payment record and decides
  nothing itself. The ledger row makes each trade number apply once, so a
  second run, a callback's repeat afterwards, or two processes at once
  change nothing more. Both lock the record, then the order, so two
  processes do not deadlock; the second reads the first's outcome.
- **90 days.** A refusal's ledger row lives 90 days. A record paid earlier
  than that is not read, so a refusal is never re-run after its row is
  pruned, and records from before the reconciler older than that stay for
  an administrator.
- **Failures.** A record whose transaction fails (a database error) records
  nothing and is read again on the next run.
- **Logs.** A line per completed order and per failure, and a summary per
  run that read a record; `CompleteOrderPaymentTx` logs refusals and failed
  grants.

## The callbacks in the payment module

- **Signatures**, ported as is (`signatures.go`): EPay's MD5 over the
  query (the kernel's gateway registry serves EPay only), Stripe's
  HMAC-SHA256 with a five-minute tolerance, x402's HMAC-SHA256 over the
  canonical fields, and PayPal's `verify-webhook-signature` API. The secrets
  come from the adopted `v2_payment_gateway`: the first enabled gateway of
  the type, as the kernel's `GetByType`. The PayPal webhook needs outbound
  HTTPS from the payment host to `api-m.paypal.com` (or the sandbox), which
  only the kernel needed before.
- **Checks**, as the kernel's: EPay's and PayPal's amount within a cent of
  the record's `actual_amount`; x402's token and amount (#89); PayPal pays
  on completed captures only (#74).
- **Answers** are the kernel's, byte for byte, in every state both can be
  in. The only difference is the failure the kernel cannot have: step 2
  failing. The kernel's single transaction fails as a whole and answers its
  error (EPay `fail`, x402 `500`, Stripe and PayPal `200` "already processed
  or error: ..."). The module answers 502 instead, so the provider delivers
  again. For Stripe and PayPal this is a fix: the kernel's `200` stops
  redelivery after a rollback, and the payment is lost.

## Parity

`internal/tests/paymentcompat` runs the real KernelOrder server in process
over gRPC on the native side's database and compares, after each request,
`v2_payment_record`, the gateway statistics, `v2_order`, the subscribers'
entitlements and subscription groups, the request ledger and the change log,
on SQLite and PostgreSQL: 78 callback cases on each (EPay 18, x402 22,
Stripe 18, PayPal 20), including repeats, the state a failure between the
steps leaves, and the refusals (short amount, wrong or unaccepted token,
another user's order, an order completed, cancelled or paid by another
payment, a forged or stale signature). Checks on the rows themselves cover
what parity cannot show, since both sides complete orders through the same
function. The PayPal API is a fake behind `http.DefaultTransport`, which
both sides' clients use; its calls are part of the compared state.
`TestCallbackConvergesAfterAFailureBetweenItsSteps` fails step 2 once and
shows the repeat completing the order once.

## Behavior changes in the kernel

- A paid callback no longer marks an order that is not pending paid
  (cancelled, completed, or paid by another payment): the payment is
  recorded and the order is left for an administrator. Before, it was marked
  paid and completed again; the grant was already once per order.
- Every paid record that names an order leaves a `payment:<trade_no>` row
  in the request ledger.
- A repeat of a paid payment re-applies it to its order, which changes
  nothing unless a failure between the module's steps left the order
  pending.
- The reconciler completes, two minutes after its payment, an order a paid
  record left pending, and records a refusal for a record that does not pay
  its pending order, including records written before this change.

## Not in scope

- Affiliate commission and notifications on completion (none, as before).
- The order module serving the contract.
