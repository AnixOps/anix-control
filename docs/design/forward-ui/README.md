# v4.2 Forwarding UI (F5b, Gate H16)

The owner approved this design on 2026-10-04 (H16) with every decision
D1–D15 as recommended below, plus one change: the node status dot on the
route detail reflects only that route. F5b implements it.

- **Where it is.** The pages are in the core app, lazy-loaded, under
  `/admin/forward/overview`, `/admin/forward/routes` (with `/new`, `/{id}`
  and `/{id}/edit`) and `/admin/forward/inventory` (with `/{node_ref}`).
  - They open only when the forward package serves the v4 API: the
    extension catalog (`GET /api/v3/extensions`) lists its control route
    `/api/v4/plugins/forward/*`. Without it they lead to the flux-clone
    page.
  - The sidebar shows 转发 for them and keeps the flux-clone pages as
    转发（旧版） and 转发节点（旧版） until F5d.
- **Code.** `web/src/views/admin/forward/{Overview,Routes,RouteEditor,
  RouteDetail,Nodes,NodeDetail}.vue`, the parts in
  `web/src/components/forward/`, and the API client
  `web/src/api/forwardV4.js`. The operator guide is
  [`docs/guide/forwarding.md`](../../guide/forwarding.md).
- **The mockups are gone.** The dev-only route
  `/admin/__mockups/forward` and its mocked data were removed when the real
  pages landed. The data lives on as the e2e fixture
  `web/e2e/fixtures/forwardV4.js`, which drives the Playwright flow, the
  accessibility checks and the visual baselines of the real pages.
- **Screenshots.** `shots/` keeps the mockup screenshots the owner
  reviewed; `capture.mjs` took them from the mockup route and is kept for
  the record. The real pages' baselines are in
  `web/e2e/visual/__screenshots__/admin-forward-*`.

The sections below describe the screens as reviewed. "Implementation
notes" at the end lists where the pages differ.

## Screens

Every file in `shots/` is named `<screen>-<desktop|phone>-<light|dark>.webp`.

| # | Screen | Files |
|---|---|---|
| 1 | Route list | `01-routes-*` |
| 1b | Route list with two rows selected (bulk bar) | `01b-routes-bulk-*` |
| 1c | Route list, empty | `01c-routes-empty-*` |
| 2 | Route editor with violations | `02-editor-violations-*` |
| 2b | Route editor, valid, with the plan preview | `02b-editor-preview-*` |
| 3 | Route detail | `03-route-detail-*` |
| 4 | Node list | `04-nodes-*` |
| 4b | Node detail | `04b-node-detail-*` |
| 5 | Overview | `05-overview-*` |

### Navigation

- The forwarding area has three sections: 概览 / 路由 / 节点. They are a
  segmented control at the top, like the other suites.
- Detail pages and the editor sit under their section, with a back link.
- The sidebar keeps one 转发 item (see D1 for where it lives).

### 1. Route List (`01-*`)

- **Header.** 路由 with a count. 新建路由 is the primary action. "…" holds
  import and export of route JSON.
- **Toolbar.**
  - A search field (name, port, target).
  - Status chips with counts: 正常 / 降级 / 跳错误 / 已暂停 / 已强制暂停.
  - Selects for engine, node and label.
- **Columns.**
  - 名称, with its labels in mono.
  - 入口监听: `:port proto`, then the entry nodes, or the entry hostname and
    the number of entries for entry HA.
  - 跳链: engine chips joined by the next hop's link security. RAW is
    omitted, and `×2` marks a hop with two nodes.
  - 目标: the first target and "另 N 个".
  - 目标策略, plus the direct mode when it is not off.
  - 状态: one badge, the worst state.
  - 24 小时流量: the total, then ↑up ↓down.
  - "…" row menu.
- **Status order.**
  1. 已强制暂停 · 配额用尽 / 已到期 (`enforced`).
  2. 已暂停 (`paused`).
  3. 跳错误 (a `HopError` on one of the route's hops).
  4. 降级 (an upstream that is unhealthy or circuit-open).
  5. 同步中 (a node lags its desired generation).
  6. 正常.
- **Row menu.** 编辑, 暂停 / 恢复, 诊断, 复制为新路由, then 删除… after a
  separator.
  - On an enforced route, 暂停 / 恢复 becomes 提高配额… or 延长到期…, which
    opens the editor's 限额 section. Resume does nothing on a route Control
    paused (see D8).
- **Bulk bar** (`01b`): 暂停 N 条, 恢复, 添加标签, 删除…. There is no bulk
  endpoint, so each runs the per-route calls (see D9).
- **Empty state** (`01c`): explains what a route is, with 查看节点 and
  新建路由. Toolbar and note are hidden.
- **Full-page captures.** The floating bulk bar (`01b`) and the editor's
  sticky phone bar appear mid-page or at the page end. They are fixed to the
  viewport in the browser.
- **Phone.** `UiDataTable` cards show the name, the listen address, the hop
  chain, the status and the traffic.

### 2. Route Editor (`02-*`, `02b-*`)

The editor is a full page: the form on the left and a sticky 规划预览 panel
on the right. Below 1100 px the panel stacks under the form. On phones a
sticky bottom bar shows the status, 查看预览 and 保存.

- **Problem summary.** A red box at the top lists each violation as
  `field`, the zh-CN text and the `code`. Clicking one scrolls to the field
  and focuses it.
  - Each field shows its own error, mapped from the violation `field` path:
    `listen.port`, `hops[1].ingress.security`, `targets[0].host`,
    `policy.direct`, `limits.expires_at_unix_ms`, and so on.
  - A hop card with an error gets a red border.
  - Save stays disabled while the preview has violations.
- **The seeded draft** (`02`) shows three common mistakes:
  - `port_reserved`: entry port 443 on sha-iepl-01.
  - `link_unsupported`: an nftables entry straight into a TLS exit.
  - `target_not_allowed`: a private target under PUBLIC_ONLY.
- **基本信息.** Name and labels (`key=value` chips).
- **入口监听.** Address, port (自动 / 指定), and protocol (TCP / UDP /
  TCP+UDP). With more than one entry node, an 入口域名 field appears for
  DNS-based entry HA.
- **跳链.** A vertical list of hop cards, each joined to the next by an
  arrow that names the link (`TLS · mux`).
  - The role is set by position (入口 / 中转 / 出口) and is read-only.
  - ↑ / ↓ reorder a hop, and the bin removes it. 添加一跳 is at the end.
  - **节点.** Chips show the failover priority (P0, P1… in `node_refs`
    order; 入口 on the entry hop) and each node's engine chips. An
    unavailable engine is struck through and its reason is in the tooltip.
    A combobox adds nodes; its options describe engines, region and port
    range, and disabled nodes cannot be picked.
  - **引擎.** A select with nftables and gost. An engine that one of the hop's
    nodes lacks is disabled, and the option says which node lacks it.
    anixops (experimental) is listed only when the setting flag is on (D12).
  - **接入链路** (hops after the entry). RAW / TLS / WSS / QUIC / gRPC.
    Options that the previous engine cannot originate, or that this engine
    cannot terminate, are disabled with the reason.
  - **Fix suggestions.** A `link_unsupported` error shows two actions in
    place: 在前面插入 gost 中转, and 改为 RAW.
  - **多路复用.** A switch, disabled on RAW.
  - **服务器名称.** Optional. By default the link uses the node identity.
  - **监听端口.** 自动 (sticky planner allocation) or 指定.
  - **拨号地址.** Disabled with several nodes (`requires_single_node`).
- **目标.** Rows of host, port, weight and priority, plus the 目标策略 select
  (公网 only / 允许内网).
- **负载均衡与故障转移.**
  - The 下一跳策略 and 目标策略 selects describe each strategy. For
    LEAST_CONN on nftables, the help says it is approximate until conntrack
    counts arrive.
  - 直连模式 is a radio group: 关闭 / 优先直连 / 强制直连.
  - The health-check and circuit-breaker fields show the H21 defaults as
    placeholders and help: 5000 ms, 2000 ms, 3 failures, 30000 ms. Empty
    means default.
  - A switch turns active health checks off.
- **限额.** Bandwidth (Mbps), quota (GB), max connections and expiry.
  - The description says limits land on the entry: bandwidth and
    connections count per entry node, and the quota is global and enforced by
    Control.
  - An enforced route opens with a banner explaining that raising the limit
    resumes it on the next plan.
- **规划预览** (`02b`). It stands for `POST /routes/preview` and runs
  automatically 1 s after typing stops (D4).
  - With violations: "N 个问题 · 未规划", the codes with the API's English
    messages, and a note that a refused plan allocates nothing.
  - When valid: totals for nodes, ports and generation changes, then one card
    per node and hop: listen address and port, ingress security, upstreams
    with priority and link, mark, and generation `n → n+1`.
  - Warnings (LEAST_CONN on nftables, entry HA without a hostname) show as
    amber notes.
  - "请求 JSON (protojson)" expands the exact request body. It doubles as a
    copyable API example.

### 3. Route Detail (`03-*`)

- **Header.** Back link, name, status badge, id and revision, and the
  one-line hop chain. Actions: 诊断, 暂停 / 恢复, 编辑 (primary), and "…"
  (复制为新路由, 查看 JSON, 删除路由…).
- **Degraded banner.** In plain words: which upstream is open, when it is
  next tried, and that traffic moved to the others.
- **Metric cards.** 24 h down (with a sparkline), 24 h up, active
  connections (with the per-entry cap), and entry latency.
- **链路与健康.** One column per hop, then the targets.
  - Each node shows its status dot (hop error / lagging / ok), allocated
    port and generation (applied / desired).
  - The dot reflects only this route (owner decision, H16): red for a hop
    error of this route, amber for an unhealthy or circuit-open upstream of
    this route, blue while the node lags its desired generation (a
    node-wide state, said in words), green otherwise. In the mockup it was
    the node's overall state.
  - Under each node are its upstreams with 健康 / 不健康 / 熔断 badges, RTT,
    "N 秒后试探 · 不在轮转" for an open breaker, and "连续失败 N 次".
  - The hop's balance strategy is shown underneath.
- **流量.** Hourly up and down from `GET /routes/{id}/stats` series, with a
  24 h / 7 d / 30 d control. It uses `UiChart` with the accessible table
  view.
- **上游健康.** A flat table: hop, node, upstream, state, in rotation,
  consecutive failures, RTT and checked time. It states the H21 breaker rule.
- **节点状态.** Converged or "落后 N 代" per node, with the report time. Each
  node links to its page.
- **配置.** A grouped list: listen, direct mode, health check and breaker
  (marked "默认" when defaulted), limits with their per-entry or global
  scope, labels and updated time.

### 4. Node List (`04-*`)

- **Header actions.** 安装 Agent, which opens the existing O1
  `AgentInstallSheet` (a real component that calls nothing until 生成), and
  添加转发节点 (primary).
- **Chips.** 转发节点 / 代理节点 (`kind`), and 同步落后 / 有跳错误.
- **Columns.**
  - 节点: name, `node_ref` and region.
  - 状态: 在线 / 离线 / 已停用.
  - 类型与通道: 转发节点 or 代理节点; Agent · mTLS or Ansible (no Agent).
  - Agent: the version, with "可升级到 4.2.0" when it is older.
  - 引擎: chips. An unavailable engine is struck through and its reason is in
    the tooltip.
  - 跳数.
  - 代（已应用 / 期望）: "落后 N 代", 已收敛 or 未上报.
  - 跳错误.
  - "…" row menu: 转发设置, 安装或重装 Agent, 重置节点状态…, 启用 / 禁用…,
    删除节点….
- A note explains the generation pair and the polling interval (D5).

### 4b. Node Detail (`04b-*`)

- **健康.** The latest report time, and tiles for applied / desired
  generation, state hash (whether it matches), hops with an error count, and
  healthy / total upstreams. Below them, each `HopError` with its route, hop
  and message, and a link to the route.
- **承载的跳.** The node's desired `NodeHop`s: route, role and hop index,
  engine, port, mark, and running or paused.
- **引擎能力.** Per engine: version, link securities, strategies, and badges
  for UDP, IPv6, 限速, 配额 and 连接数. Above the list: the Agent version,
  kernel and cgroup.
- **转发设置** (`PUT /nodes/{ref}/settings`). Port range, reserved ports,
  addresses (one per line, in dial order) and labels.
  - A note says saving replans every route, and that moved ports come back
    as warnings.
  - 放弃 / 保存设置.
- **Agent.** Transport, version and forward.v1 negotiation, plus
  生成安装命令, which opens `AgentInstallSheet` for this node.
- **危险操作.** 禁用节点… and 删除节点… are disabled while routes use the
  node. The text names the routes, matching the `node_in_use` refusal
  (D7).

### 5. Overview (`05-*`)

- **Metric cards.** 路由 (running, paused and enforced counts), 在线节点
  (with the lagging count), 24 小时流量 (with a trend word and sparkline),
  and 待处理.
- **流量.** Up and down for every route over 24 h.
- **流量排行.** The top 5 routes, with usage bars relative to the first.
- **需要处理.** Hop errors (node · route · message), open breakers, lagging
  nodes ("已应用第 17 代，期望第 18 代"), and enforced routes. Each has a
  direct action.

## Conventions the Mockups Fix

- **Status words.** 正常, 降级, 跳错误, 同步中, 已暂停, 已强制暂停 · 配额用尽,
  已强制暂停 · 已到期. Every badge has a word; colour is never the only signal.
- **Engine chips.** Short mono names (`nft`, `gost`, `anixops`) with a
  token-coloured dot (`--chart-1`, `--chart-3`, `--chart-5`).
- **Link labels.** `RAW`, `TLS`, `WSS`, `QUIC`, `gRPC`, plus `·mux`.
- **Roles.** 入口 / 中转 / 出口 by position; 第 N 跳 is 1-based in the UI.
  `hop_index` in the API is 0-based.
- **Generations.** Always shown as applied / desired. "落后 N 代" uses the
  info tone, because lagging is normal for up to a minute after a change.
- **Traffic.** Raw metered bytes from the entry hop, with no multiplier. The
  list note says so.

## Decisions

The owner approved each recommendation on 2026-10-04 (H16).

- **D1. Where the final UI lives.**
  - The forward package manifest declares its WebUI home at
    `/admin/extensions/forward` (`anixops.webui/v1`).
  - That contract is an `h()` mount with five legacy CSS classes. It has no
    access to `Ui*`, the tokens' components, `UiChart` or the router.
  - *Decision (approved):* build F5b in the core app as `/admin/forward/*`, with
    pages lazy-loaded behind the forward package being installed. Keep the
    package's WebUI entry as a link to it. Widening the WebUI host contract to
    expose `Ui*` is a larger, separate decision.
- **D2. Editor as a full page or a sheet.**
  - *Decision (approved):* a full page (as mocked). The hop chain, targets and
    live preview do not fit a 560–760 px sheet, and on phones the page with a
    sticky bottom bar beats a bottom sheet.
  - Quick edits (pause, labels, limits) stay in the row menu and the detail
    page.
- **D3. List filters the API lacks.**
  - `GET /routes` filters by `owner` and `node_ref` only. Status, engine and
    label filter the loaded page, and the note under the table says so.
  - *Decision (approved):* keep them client-side for v4.2. Admin route counts are
    small: 100 per page, at most 1000.
  - Add `status`, `engine` and `label` query filters to F5a only if
    installations above about 500 routes appear.
- **D4. Preview cadence.**
  - *Decision (approved):* debounce `POST /routes/preview` to 1 s after the last
    change, plus the explicit refresh icon.
  - Cancel in-flight requests, and never preview while a required field is
    empty.
  - The planner call is cheap for one route (`PlanRoute`), but it audits as
    `preview`. Consider not auditing previews (see D13).
- **D5. Polling.**
  - *Decision (approved):* node list and route detail every 15 s, overview every
    30 s, and pause while the tab is hidden. Show "更新于 N 秒前" and a
    refresh button.
  - Nodes report on their own schedule, so faster polling adds no
    information.
  - Traffic (`/stats`) refreshes every 60 s at most.
- **D6. The 24 h traffic column.**
  - *Decision (approved):* one `GET /stats` (window 24 h, no `route_id`) per list
    load, joined to the rows by `route_id` on hop 0. Do not make one call per
    row.
  - When an answer says `truncated`, show "数据已截断，仅显示前 2000 条路由 /
    20000 个时段" under the table or chart, never silently.
- **D7. The can-delete signal.**
  - The API has no per-object `can_delete`. A `DELETE` needs a super
    administrator (`403 super_admin_required`), and node disable or delete is
    refused while routes use the node (`409 refused` with `node_in_use`).
  - The web app has no super-administrator flag either. Route modes get one
    from their own answer (`can_switch`).
  - *Decision (approved):*
    - F5a adds a `can_delete` flag to the list answers (`GET /routes`,
      `GET /nodes`), the same pattern as `can_switch`.
    - Until it does, show 删除… to every administrator and turn the 403 into
      an inline 「仅超级管理员可删除」 message.
    - Node disable and delete stay disabled while routes use the node, naming
      the routes (as mocked; the client already knows from the routes).

- **D8. Paused vs enforced.**
  - *Decision (approved):* show enforced (quota or expired) as a different badge
    from manual pause, and never offer 恢复 for it. Offer 提高配额… or
    延长到期…, which opens the editor's 限额 section.
  - When a route is both paused and enforced, show enforced. Resuming it
    still needs the operator.
- **D9. Bulk actions without bulk endpoints.**
  - *Decision (approved):* run per-route pause, resume and delete calls, at most 4
    at a time, each with its own `Idempotency-Key`. Report "已暂停 5 条，1 条
    失败" with a retry for the failures, and offer 撤销 for pause and resume.
  - Bulk delete needs a typed confirmation and goes through the super
    administrator check.
  - Ask for bulk endpoints only if this proves slow.
- **D10. `Idempotency-Key` and `revision_conflict` in the editor.**
  - *Decision (approved):* generate one key when the editor opens and reuse it
    for retries of the same save, so a double click or a network retry never
    creates two routes. Make a new key after a successful save or an edit.
  - On `409 revision_conflict`, keep the form, show 「此路由已被他人修改（修订
    7 → 8）」, and offer 查看差异并重新应用 or 放弃我的修改.
- **D11. LEAST_CONN on nftables.**
  - Until the Agent supplies conntrack counts (F3b), LEAST_CONN on nftables
    behaves as weighted random.
  - *Decision (approved):* keep the strategy selectable and show the help text
    plus a preview warning, as mocked. Do not hide it.
- **D12. Showing the anixops engine.**
  - *Decision (approved):* hide ENGINE_ANIXOPS and LINK_SECURITY_ANIXOPS
    everywhere unless the setting flag (`EnableAnixOps`) is on.
  - When the flag is on, label it 「anixops（实验）」 and list only nodes that
    advertise it. Existing routes that use it always show it, whatever the
    flag.
- **D13. Audit noise from previews.**
  - The audit middleware records `POST /routes/preview` like a write.
  - *Decision (approved):* exempt `preview` from the audit log, or record it at
    debug level. Otherwise D4's debounced previews flood the audit log.
    This is a small F5a follow-up.
- **D14. Entry HA hostname.**
  - DDNS provider setup is not in the v4 API yet (section 7.4).
  - *Decision (approved):* in v4.2, show `entry_hostname` as a plain field with a
    note that the operator manages DNS. Add the DDNS provider picker when the
    API exists.
- **D15. Proxy nodes in the inventory.**
  - Proxy nodes join the forwarding inventory through settings.
  - *Decision (approved):* list them in 节点 with a 代理节点 badge, as mocked. Their
    settings page offers 转发设置 only; the record fields belong to the proxy
    node page.
  - A proxy node that is not in the inventory appears only behind a
    "加入转发清单…" action on the proxy node page.

## Implementation Notes

Where F5b differs from the screens above, and why.

- **Paths.** The flux-clone pages already own `/admin/forward`,
  `/admin/forward/nodes` and `/admin/forward/nodes/{id}`, so the node
  inventory is `/admin/forward/inventory`. The package's own WebUI page
  (`/admin/extensions/forward`) is a link to the overview, and its menu
  entry is hidden while 转发 is shown.
- **List status.** One list load reads `GET /routes`, `GET /nodes`, one
  `GET /stats` and `GET /observability/targets`, plus `GET /nodes/{ref}`
  for at most 20 nodes that report hop errors. 降级 therefore comes from the
  last hop's target health; an unhealthy relay upstream shows on the route
  detail.
- **Online.** `NodeSummary` has no Agent session state, so a node counts
  as online while its latest report is at most 3 minutes old.
- **anixops (D12).** Control does not serve `forward.anixops_experimental`
  yet (F6 A1–A5). The pages read one flag (`useForwardFlags`), which is off
  until that lands; routes that already use anixops always show it.
- **D7.** `can_delete` is served (`GET /routes`, `/nodes`,
  `/ansible-machines`). Delete is disabled with 「仅超级管理员」 when it is
  false; a `403` from an older package still shows inline.
- **D13.** `POST /routes/preview` is logged at debug level and never
  written to the audit table.
- **D14.** With the L2 DNS API, the entry hostname has the binding picker
  (provider, zone, DDNS or CNAME, A/AAAA, TTL, paused), the area has a
  fourth section, DNS (providers), and the route page an 入口高可用 card
  polled every 30 s. Provider writes and unbinding follow D7's
  super-administrator signal; a `403` is shown inline. The kind is a radio
  group (five kinds, each with what its credential needs).
- **Left out.** The node list's header 安装 Agent (the install sheet needs a
  node; it is on each row and node page), 重置节点状态 (no API; the CLI has
  `forward reset-node`), and the Agent upgrade hint (O4's campaigns own
  upgrades).
