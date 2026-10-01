# Subscriber Service (KernelSubscriber)

Status: in place. The contract is `sdk/api/kernelsubscriber/v1`, served by
`internal/kernelsubscriber` since F2d.

## Why

Every business domain still writes `v2_user` directly.
- **Order completion** writes plan, group, traffic, limits and expiry, and
  rewrites `v2_user_subscription_group`.
- **Administrator plan assignment** does the same.
- **Node user lists** filter `v2_user` for banned, expired, out-of-traffic
  and group.
- **Node and forward traffic** increment `v2_user.u/d`.
- **The monthly reset worker** zeroes `v2_user.u/d`.
- **Commissions** change `commission_balance`.

A domain cannot become a module while it writes a table other domains own,
so these writes move behind one kernel contract first.

## Ownership

The kernel owns the subscriber record:
- `v2_user` entitlement columns, traffic counters, subscription token, proxy
  uuid and balances;
- `v2_user_subscription_group`.

Identity owns accounts; the kernel keeps a projection of the identity columns
(`identity-service.md`).

Domain modules own their own data: plans, orders, payments, tickets and so on.
They change subscriber state only through `KernelSubscriber`, and read it
through the contract or the view `kapi_subscriber_entitlement_v1`. The view
carries entitlements and counters, but no token or uuid.

## Capabilities

Each method family needs its own signed capability, so a module holds only
what it uses:

| Capability | Methods | Typical holders |
|---|---|---|
| `kernel.subscriber.entitlements.v1` | ApplyEntitlement, AdjustEntitlement | order, plan, identity (admin edits) |
| `kernel.subscriber.traffic.v1` | RecordTraffic, ResetTraffic | proxy-node, forward, identity (admin reset) |
| `kernel.subscriber.credentials.v1` | ResetCredentials | subscription, identity (admin reset) |
| `kernel.subscriber.balance.v1` | AdjustBalance | affiliate, payment |
| `kernel.subscriber.directory.v1` | GetSubscribers, LookupBySubscriptionToken, ListActiveSubscribers, WatchSubscriberChanges | subscription, proxy-node, forward |
| `kernel.subscriber.groups.v1` | GrantSubscriptionGroup, RevokeSubscriptionGroup, RemoveSubscriptionGroupMembers | subscription |

As with `kernel.identity.v1`, only packages signed by the official root can
hold them. The kernel authorizes every call against the calling session's
package and generation.

## Semantics

- **Idempotency.** Every retryable write names a `request_id` (for traffic, a
  `batch_id`).
  - `v4_kernel_subscriber_request` records each one with its result, in the
    same transaction as the change.
  - A repeat returns the first result with `applied: false`.
  - Rows are kept for 90 days.
- **ApplyEntitlement.** It locks the subscriber row, then:
  1. sets plan, group, traffic and limits from the caller's `PlanSnapshot`;
  2. replaces the subscription groups;
  3. computes the expiry:
     - with `period`, from now, or with `renew_same_plan` from the current
       expiry when the subscriber holds the same plan and has not expired;
     - with `expires_at_unix`, exactly as given;
  4. zeroes the counters when `reset_traffic` is set.

  This is today's `OrderService.Complete` and `PlanService.AssignToUser`
  logic, with the plan read by the caller instead of the kernel.
- **Payment auto-activation (owner decision, 2026-09-30).** A payment callback
  that marks an order paid also completes it: `ApplyEntitlement` with
  `request_id = "order:<order id>"`, in the same flow. An administrator's
  "mark paid" completes an order with the same id, in the kernel or in the
  order module (`kernel.subscriber.entitlements.v1`), so an order's plan is
  granted once whichever path completes it.
- **Administrator assignment.** The plan module applies it with
  `request_id = "plan.assign:<plan_id>:<user_id>:<digest>"`. The digest
  covers the request's `Idempotency-Key` (else its request id) and the
  expiry. The kernel's legacy handler derives the same id, so a retry applies
  once whichever side serves it, and a new request is a new grant.
- **Commission withdrawals.** A withdrawal debits `commission_balance` with
  `AdjustBalance` (kind `COMMISSION`), `request_id = "affiliate.withdraw:<withdrawal id>"`;
  rejecting it refunds the amount with
  `request_id = "affiliate.withdraw.refund:<withdrawal id>"`. The kernel's
  legacy handlers use the same ids through `subscriber.AdjustBalanceTx`, in
  the withdrawal's own transaction; the affiliate module calls the contract
  (`packages/affiliate/native`). A debit below zero is refused under the
  row lock, so concurrent withdrawals cannot overdraw.
- **Administrator resets.** identity-platform resets a user's traffic and
  subscription token with `ResetTraffic` and `ResetCredentials`, with
  `request_id = "identity.reset_traffic:<user_id>:<digest>"` or
  `"identity.reset_subscribe:<user_id>:<digest>"`; the digest covers the
  request's `Idempotency-Key` (else its request id). The kernel's legacy
  handlers derive the same ids, so a retry applies once whichever side
  serves it. `ResetCredentials` and `AdjustEntitlement` answer `NotFound`
  for a subscriber that does not exist and record nothing.
- **Forward traffic reset.** The Flux-style `POST /api/v2/user/reset`
  (type 1) resets a subscriber's traffic with `ResetTraffic`,
  `request_id = "forward.reset_traffic:<user_id>:<digest>"` (the request's
  `Idempotency-Key`, else its request id), in the kernel's legacy handler
  and in the forward module (`packages/forward/native`). Forward flow
  accounting stays in the kernel: a forward's counters, the subscriber's
  traffic (`subscriber.RecordTrafficTx`) and the tunnel permission's traffic
  change in one transaction, and exhaustion pauses forwards on their nodes.
- **Subscription group membership.** `v2_user_subscription_group` rows,
  one per group a subscriber holds, each with its own expiry, traffic and
  renewal price. `internal/subscriber` holds the engine functions; the
  kernel's legacy handlers and the contract call the same ones.
  - `GrantSubscriptionGroup` (`GrantSubscriptionGroupTx`) creates the
    membership, or sets the given fields of an existing one and keeps the
    others, as the v2 route does.
  - `RevokeSubscriptionGroup` (`RevokeSubscriptionGroupTx`) deletes one.
  - `RemoveSubscriptionGroupMembers` (`RemoveSubscriptionGroupMembersTx`)
    deletes every membership of a group, whether or not the group still
    exists. The subscription module calls it before it deletes the group's
    templates, plan links and node protocol links and the group itself, so
    a retry after a failure in between finds no members and completes.

  Each runs under the row locks of the subscribers it changes (taken in id
  order) and records its request id. Grant and revoke answer `NotFound` for a subscriber, a
  group or (revoke) a membership that does not exist, and record nothing;
  the message says which is missing.

  The administrator routes derive their request ids from the request's
  `Idempotency-Key` (else its request id), on both sides, so a retry applies
  once whichever side serves it:
  - `subscription.grant:<user>:<group>:<digest>`, whose digest also covers
    the granted fields, so a key reused for other values is a new grant;
  - `subscription.revoke:<user>:<group>:<digest>`;
  - `subscription.delete_group:<group>:<digest>`.

  The ledger methods are `grant_subscription_group`,
  `revoke_subscription_group` and `remove_group_members` (user 0).
- **RecordTraffic.**
  - Adds `(upload, download) × rate` to each subscriber's counters in one
    transaction.
  - A subscriber who crosses the transfer limit is returned in
    `exhausted_user_ids` and emits a `REMOVE` change.
- **ResetTraffic.** Zeroes counters. The caller decides whom to reset: proxy
  node's monthly worker, or an administrator.
- **Active subscribers.** A subscriber is active when:
  - `banned = 0`;
  - `expired_at` is null or in the future;
  - `u + d < transfer_enable`: a zero limit serves nothing, as node user
    lists always have;
  - their primary group (`v2_user.group_id`) is one of the requested groups,
    as today's node lists filter. Subscription groups serve subscription
    links, not node lists.
- **Change feed.**
  - `v4_kernel_subscriber_change` is an append-only log (cursor, user id,
    kind), written in the same transaction as any change that can alter
    activity or node-visible fields: entitlements, ban, uuid, traffic
    exhaustion or reset, deletion, and subscription group membership.
  - **Membership changes.** Subscription groups do not decide whom a node
    serves (`subscriber.Active` reads the primary group only), but
    `Subscriber.subscription_group_ids` is part of every `UPSERT`, and they
    decide what a subscription link renders. A membership write therefore
    appends a row for a subscriber when it changes which groups they hold,
    or until when, and the subscriber is active:
    - a grant that creates the membership or changes its expiry;
    - a revocation;
    - each active member of a group whose members are removed.

    A grant that only changes the traffic or renewal price appends nothing.
    An inactive subscriber is on no watcher's list: whatever makes them
    active again (an entitlement, an unban, a traffic reset) appends its own
    row, which carries the groups they hold then.
  - Rows are kept for 7 days; an older cursor gets `RESYNC`.
  - Expiry is time-based and emits nothing. Consumers compare `expires_at`
    with their clock and re-list periodically.

## Delivery

- **F2a. Entitlement engine** (in place).
  - `internal/subscriber.ApplyEntitlementTx`, with the request ledger
    `v4_kernel_subscriber_request`.
  - `OrderService.Complete` applies `order:<id>` once, and
    `PlanService.AssignToUser` keeps the subscription groups, as in v2.
  - Payment callbacks complete the order in a savepoint: a failed activation
    leaves the payment recorded and the order paid, and is logged.
  - Admin entitlement edits (`SubscriberEntitlements`) move in F2d with the
    contract's AdjustEntitlement.
  - Originally planned scope:
  - Add `internal/subscriber` holding the entitlement, credential and balance
    logic, plus the request ledger.
  - Move `OrderService.Complete`, `PlanService.AssignToUser`, admin
    entitlement edits (`SubscriberEntitlements`) and `KernelIdentity.UpdateSubscriber`
    onto it.
  - Payment callbacks auto-complete orders.
  - The legacy v2 handlers then call the same code the contract will serve.
- **F2b. Traffic ledger** (in place).
  - `internal/subscriber.RecordTrafficTx` and `ResetTrafficTx` are the only
    writers of `v2_user.u/d`. They are used by node reports, the v2board
    traffic service, forward flow and manual resets.
  - Rows are updated in id order, so concurrent reports cannot deadlock.
  - A batch id is applied once. Legacy node reports carry none and are
    therefore not deduplicated, as before.
  - Exhaustion (crossing the transfer limit) is reported for F2c's change
    feed.
  - Originally planned scope:
  - `RecordNodeTrafficReport`, forward flow accounting and the monthly reset
    worker go through `RecordTraffic` and `ResetTraffic` semantics, with batch
    idempotency.
- **F2c. Directory and change feed** (in place).
  - `subscriber.Active` is the one definition of an active subscriber; node
    user lists (UniProxy and v2board gRPC) use it.
  - The change log is `v4_kernel_subscriber_change`. It is written by the
    engine (entitlement, exhaustion, reset), by `UpdateUserTx` when a
    node-relevant column changes, and on user creation and deletion.
  - A kernel worker prunes it hourly after 7 days. `ChangesAfter` answers
    resync for older cursors.
  - The view `kapi_subscriber_entitlement_v1` is added.
  - Originally planned scope:
  - `GetActiveUsersForNode`, the v2board gRPC `UserService` and UniProxy user
    lists read `ListActiveSubscribers`.
  - The change log feeds `UserChanges`.
  - The view `kapi_subscriber_entitlement_v1` is added.
- **F2d. Serve the contract** (in place).
  - The five capabilities are in the grammar.
  - `PackageHostOperations.AuthorizeCapability` checks each call against the
    host's current generation and verified signed release, as for
    KernelIdentity.
  - The contract is served on local package bridge sessions and on the mTLS
    module listener, where calls resolve to the bound instance's generation.
  - Idempotent writes go through the request ledger; entitlement edits and
    credential resets go through `UpdateUserTx`, so revocations and the
    change log apply.
  - `WatchSubscriberChanges` polls the change log every second, re-checks
    authorization every 30 seconds and answers `RESYNC` for pruned cursors.
  - Originally planned scope:
  - The capabilities join the grammar.
  - `KernelSubscriber` is served on the module listener and package bridge,
    with authorization and parity tests between legacy callers and contract
    callers.
- **F2e. Subscription group membership** (in place).
  - The capability `kernel.subscriber.groups.v1` and the three calls
    `GrantSubscriptionGroup`, `RevokeSubscriptionGroup` and
    `RemoveSubscriptionGroupMembers` were added to the contract; only new
    calls and messages, so v4.0.0 hosts are unaffected.
  - The legacy v2 membership routes (granting a user a group, taking it
    away, deleting a group) go through the same engine functions. Before,
    they wrote no change-log row, so a watcher's copy of a subscriber's
    groups went stale.
  - The subscription module serves the three routes natively through the
    contract (`internal/tests/subscriptioncompat`).

## Not in scope

- Plan and order data: they belong to the plan and order modules.
- Multi-currency and refunds beyond balance adjustments.
- Per-node traffic statistics (`v2_stat_*`): they stay with proxy node.
