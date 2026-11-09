# Entry Pools, Canary Addresses And Leak Tracing

Node entries (节点 → 入口, `internal/nodeentry`) can be dealt out to
subscribers in pools, so that a leaked subscription exposes only its pool's
addresses and can be traced back to the subscribers who had it (anx
`docs/FUTURE-IMPROVEMENTS.md` F-55, `internal/entrypool`).

## Rules

- An entry with a **pool** is offered only to the subscribers of that pool.
  An entry without one is shared: every subscriber gets it, except on nodes
  that have an entry of the subscriber's own pool (then only that one).
- A node whose entries are all in other pools is **not offered** to the
  subscriber at all: never its bare host, which would defeat the pools.
- A subscriber's pool, first match wins:
  1. an administrator's **assignment** (`v5_user_entry_pool`);
  2. the **quarantine** pool (`quarantine` unless renamed) while the account
     is younger than `quarantine_days`, or has no plan (new and trial
     accounts);
  3. `shard-<user id % shards>` when `shards` is 2 or more;
  4. none: shared entries only.
- Subscriptions follow on their next update; nodes are not reconfigured
  (entries are addresses that already reach the node, such as extra IPs,
  NAT ports or relays).

## Typical Setup

1. Policy: `PUT /api/v4/kernel/entry-pools/policy`
   `{"shards":4,"quarantine_days":7}`.
2. Give each busy node one entry per shard (`pool: shard-0` …), plus an entry
   in `quarantine` on the nodes new accounts may use. Leave the node's
   shared entries for nodes you do not shard.
3. Watch `GET /api/v4/kernel/entry-pools`: each pool's entries and how many
   subscribers it has, and why (assigned, quarantine, shard).

## Canary Address For One Subscriber

To check whether one subscriber leaks their subscription: assign them to a
pool of their own (`POST /api/v4/kernel/entry-pools/assign`
`{"user_ids":[42],"pool":"canary-42","reason":"suspected reseller"}`) and
give a node an entry in `canary-42` with an address nobody else has. Probes or
blocks on that address mean their subscription is out.

## Tracing A Leak

1. An address shows up where it should not (a public subscription dump,
   probes, a block): `GET /api/v4/kernel/entry-pools/leak?host=<address>`
   (or `entry_id=`) answers its pool and the subscribers of that pool (up to
   500 ids).
2. To narrow a pool down, split it:
   `POST /api/v4/kernel/entry-pools/split`
   `{"pool":"shard-2","into":["shard-2a","shard-2b"]}` assigns its
   subscribers alternately to the two halves. Give each half a fresh entry,
   retire the leaked one, and see which half leaks next. Repeat.
3. Assigning an empty pool returns subscribers to the policy.

Every write is in the operation log (module `entry_pool`).

## Client Failure Reports

AnixOps Net clients report, per node of their subscription, how their
connections ended: `POST /{subscribe_path}/{token}/report` (the
subscription's own address plus `/report`, so `https://panel.example.com/s/<token>/report`).

```json
{"client":"anixops-net/0.3.0","platform":"ios","network":"cellular","window_secs":600,
 "nodes":[{"node":"HK 1","server":"hk.example.com","proto":"anx",
           "ok":120,"fallback":2,"timeout":3,"refused":0,"reset":1,"after_handshake":1,
           "keepalive_lost":0,"other":0,"stopped":false,"clock_skew_secs":2}]}
```

- `network` is `wifi`, `cellular`, `ethernet` or `other`; `window_secs`
  60–86400; at most 200 nodes; each count 0–1,000,000; unknown fields are
  ignored. Counts only, never a destination.
- Answers: `204` taken, `404` unknown token (or a host the subscription is
  not served on), `400` out of bounds, `429` when the same subscriber
  reported within 10 minutes (with `Retry-After`; the client drops the
  report).
- Reports add up per subscriber, hour, node and network
  (`v5_client_failure_report`, kept 30 days).
  `GET /api/v4/kernel/client-failures?hours=24` answers them per node, most
  failing first: outcomes by cause, subscribers that reported the node and
  that gave up on it, outcomes per network. A node many subscribers fail on
  while its own status is fine is blocked on the user side; failures on
  every node of one entry pool point at that pool's addresses.
