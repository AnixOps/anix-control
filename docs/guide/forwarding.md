# Forwarding (v4.2)

This guide covers the v4.2 forwarding pages for administrators: routes,
hop chains, the forwarding inventory and diagnosis. They run on the forward
package's v4 API ([`../forwarding/v4-api.md`](../forwarding/v4-api.md)).
The design and its decisions are in
[`../design/forward-ui/README.md`](../design/forward-ui/README.md).

The flux-clone forwarding pages (转发（旧版）, 转发节点（旧版）) were removed in
v4.2 (F5d); their paths, and `/admin/forward`, open 概览. Their old guide
is archived in [`../archive/flux-panel-clone.md`](../archive/flux-panel-clone.md).

## Before You Start

- **The forward package.** The pages appear when the installed forward
  package serves `/api/v4/forward/*`. The sidebar then shows 转发. Without
  it, the sidebar has no forwarding entry and a link to the pages leads to
  插件中心, where the package is installed.
- **Permissions.** Any administrator may read and write, subject to the
  package's `forward.api` access group grants. Deleting a route or a node
  needs a super administrator (an administrator who is not staff). Others see
  删除 disabled.
- **Nodes.** A route can only use nodes in the forwarding inventory:
  forward nodes, and proxy nodes added with 加入转发清单… on the proxy node's
  page.

## The Sections

The segmented control at the top switches between them. Each page says when
it last refreshed. Pages refresh by themselves while the tab is visible:
nodes and route detail every 15 seconds, overview and routes every
30 seconds, traffic at most once a minute. The refresh button reloads now.

### 概览 (Overview)

- Route counts (running, paused, paused by Control), online nodes and how
  many lag their desired generation, 24-hour traffic, and what needs
  attention.
- 需要处理 lists hop errors, open circuit breakers, lagging nodes and routes
  Control paused, each with a direct action.
- Traffic is the raw bytes metered at each route's entry hop, without
  billing multipliers.

### 路由 (Routes)

- One row per route: entry listen, hop chain, first target, target
  strategy, status and 24-hour traffic.
- **Status** is the worst of: 已强制暂停 (Control paused it: quota used up
  or expired), 已暂停, 跳错误, 降级, 同步中, 正常.
- **Filters.** Search, status, engine, node and label filter the loaded
  routes in the browser. The note under the table says so. When the
  statistics are truncated, the page says that too.
- **Row menu.** 编辑, 暂停 / 恢复, 诊断, 复制为新路由, 删除….
  - A route Control paused cannot be resumed by hand: the menu offers
    提高配额… or 延长到期… instead, which opens the editor's 限额 section.
    The route resumes at the next plan once the limit allows it.
- **Bulk.** Select rows to pause, resume or delete them. Each route is its
  own request, four at a time. The summary says how many failed and offers
  a retry; pause and resume can be undone. Bulk delete asks you to type
  `删除 N 条路由` (`delete N routes` in English).
- **Import and export.** "…" exports the selected routes (or all) as JSON
  and imports routes from pasted JSON, one request per route.

### 节点 (Nodes)

- Forward nodes and proxy nodes in the inventory, with status, channel
  (Agent over mTLS, or Ansible), Agent version, engines, hops, generation
  (applied / desired) and hop errors.
- **添加转发节点** adds a forward node (name, host, channel, region and an
  optional port range). An Agent node then joins with the install command
  from its page.
- **A node's page** shows its health (generation, state hash, hop errors,
  upstreams), the hops it hosts, its engines, the Agent, and its
  **转发设置**: port range, reserved ports, addresses (dial order) and
  labels. Saving replans every route. Routes that no longer plan are listed;
  the nodes keep their generation until they plan again.
- A node used by a route cannot be disabled or deleted; the page names the
  routes.
- A proxy node's page here has only its forwarding settings. Its name,
  address and protocols are on the proxy node page.

## Creating or Editing a Route

新建路由 opens the editor as a full page. The 规划预览 panel on the right
(below the form on narrow screens, and behind 查看预览 on phones) asks
Control to plan the route without storing it.

1. **基本信息.** Name and `key=value` labels.
2. **入口监听.** Address, port (自动 or 指定) and protocol. With more than
   one entry node, 入口域名 appears. Point that name at the entry nodes
   yourself, or turn on 让此主机名始终指向健康的入口 below it and pick a
   DNS provider, zone, mode (DDNS or CNAME), record types, TTL and 暂停:
   Control then keeps the name on the healthy entries. The binding is
   written after the route is saved
   ([Forward Entry HA Through DNS](forward-entry-ha.md)).
3. **跳链.** Hop 1 is the entry, the last hop the exit, the others relays.
   - Add nodes to each hop. With several nodes, their order is the failover
     priority.
   - Pick the engine (nftables or gost) and, from hop 2 on, the ingress
     link (RAW, TLS, WSS, QUIC, gRPC). Options a node or the previous
     engine cannot handle are disabled with the reason.
   - If the previous engine cannot originate the link, the hop offers
     在前面插入 gost 中转 or 改为 RAW.
   - ↑ / ↓ reorder hops; inside a hop card, Alt+↑ / Alt+↓ do the same.
4. **目标.** Host, port, weight and priority, and whether private addresses
   are allowed.
5. **负载均衡与故障转移.** Next-hop and target strategies, direct mode,
   health checks and circuit breaker (empty fields use the defaults: 5 s
   interval, 2 s timeout, 3 failures, 30 s open). Least connections on
   nftables is approximate until the Agent reports conntrack counts; the
   form and the preview say so.
6. **限额.** Bandwidth and connections per entry node, a global quota, and
   an expiry.

The preview runs one second after you stop typing, and only once the name,
every hop's nodes and the targets are filled in. Problems appear at the top
and next to their fields; click one to jump to it. 保存 stays disabled
while the preview reports problems.

**Someone else saved first.** If the route changed since you opened it,
the editor keeps your form and says 「此路由已被他人修改（修订 7 → 8）」.
查看差异并重新应用 shows what changed and saves your form on top of the
latest revision; 放弃我的修改 loads the latest revision. A retried save of
the same form never applies twice.

### DNS

- **DNS 服务商** (`/admin/forward/dns`) lists the DNS accounts that entry
  high availability writes through: kind, endpoint, the names of the stored
  credentials (never their values) and how many bindings use each.
- Adding, editing and deleting a provider needs a super administrator;
  other administrators see the list read-only. A provider in use cannot be
  deleted. Setup per provider: [Forward Entry HA Through DNS](forward-entry-ha.md).

## Route Detail and Diagnosis

- **入口高可用** (on a route with an entry hostname or several entries)
  shows the DNS state, the published and desired records, each entry
  node's reason, the last error and the next attempt. It refreshes every
  30 seconds; 解除绑定 (super administrator) can delete the published
  records too.

- The chain shows every hop's nodes with their port, generation and
  upstreams (健康, 不健康, 熔断 with the time to the next probe). The dot
  before a node reflects this route only: red for a hop error, amber for an
  unhealthy upstream, blue while the node syncs.
- Traffic covers 24 hours, 7 days or 30 days; 上游健康 lists every upstream
  with its state, failures and latency.
- **诊断** runs `POST /api/v4/forward/routes/{id}/diagnose` (up to 25 s) and
  shows three stages: Control's records, the nodes' probes, and Control's
  own dials. Each step has a status (通过, 失败, 不确定, 跳过) and a stable
  code such as `unreachable` or `circuit_open`. A node whose Agent cannot
  probe is marked, and Control dials public addresses in its place.

## Troubleshooting

| You see | Check |
|---|---|
| The 转发 item is missing | The forward package is not installed, is disabled, or predates the v4 API; or your access group lacks `forward.api`. |
| 转发服务暂时不可用 | The package host has no `ForwardControl` connection. Check the package in 插件中心. |
| A route stays 同步中 | The node has not applied its desired generation. Open the node page; a node reports at least every minute. |
| 跳错误 | The node page lists the error per route and hop. Diagnose the route for its listen and port-conflict checks. |
| Saving node settings lists routes | The new range or reserved ports leave those routes without a port. Widen the range or move the routes. |
