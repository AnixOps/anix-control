# 网络模型实施计划

状态：进行中（2026-10-28）。设计见 [README.md](README.md)（N1–N9 已决定）。所有者选择先做 ANX-Line 传输（面板转发规则经网络模型的 `anx_line` 传输落地，而不是新增转发引擎），所以按 §9 的顺序推进，每一步只做 `anx_line` 需要的部分，其他 kind 后续补。

## 合约（第 1 步，本 PR）

- `sdk/api/network/v1/network.proto`（`anixops.network.v1`，草案，`check_proto_golden.py` 的 draft 包，Control 开始提供 `network.v1` 后冻结为只增不改）。
- `sdk/network`：Hello 属性 `device_capabilities`、nodeconfig 成员 `network`、状态 PackageReport（`network` / `network.status` / `v1`）的编解码；`Check`（§10 中单台设备计划能检查的不变量）、`Unsupported`（设备不支持的 kind/版本、路由区间）、`HashOf` / `Stamp`。
- `contracts/network/v1/`：金样计划（ANX-Line 入口设备、出口设备、双线路单机）、能力与状态样例。Agent 与 anixcore 用同一份文件测试自己的解析和哈希。

与设计稿的差异（实现时定下的）：

- **计划按设备视角。** `Path` 只描述从本机用户入口出发的第一段：`egress_id`（本机出口）或 `transport_id`（第一段传输），另带 `final_egress_id`（最终出口，可能在别的设备上，`failover` 的 `egress_ids` 引用它）；后续各段在下一台设备的 `forwarding` 里（N3：每路径一条链路）。
- **Transport 带本机一端的角色。** `role` 为 `dial` / `accept` / `local`；`dial` 端引用本机 hop 出口，`accept` 端引用本机 hop 入口。
- **`anx_line` 设置就是公开边界的 Link。** 字段对应 anix-agent `contracts/anx` 的 mgmt.proto `Link`；`link_id` 必须等于传输 id（内核出站名 `anx-line:<transport id>`）；`link_key` 由 Control 生成并封存，只出现在计划里，不进状态、报告和日志；对端公钥与载体地址只在 `dial` 端。
- **状态走 PackageReport。** 与 forward 报告一致，不新增 `AgentToControl` oneof。
- **时间单位统一为毫秒**（`interval_ms`、`failback_delay_ms`）。

## Control 模型（第 2a 步）

- `internal/networkmodel`：整份文档存一行（`v4_kernel_network_model`，带 revision），`PUT /api/v4/kernel/network` 整份替换；规划器先跑通才落库（跨设备规则：各段首尾相接、传输连接两台不同设备、一条传输只承载一条路径，再加每份计划的 `sdk/network.Check`）。
- 链路密钥：每条 `anx_line` 传输首次出现时生成 32 字节随机密钥，`ca_kek` AES-256-GCM 封存（`v4_kernel_network_transport_key`），删除传输即删除密钥；`rotate-key` 轮换。
- 出口设备的 anx-server 公钥记在 `v4_kernel_network_device.public_keys_json`（第 2b 步由 NetworkStatus 写入）；没有公钥时路径列为 pending，不进计划；`failover`/`cross` 的出口都在 pending 路径上时暂按 `same_line`。
- 每台设备的计划哈希变化即 generation + 1；曾有计划、现在没有的设备下发空计划。
- 已定：accept 端在自己的计划里拿到监听地址：`AnxLineSettings` 的 `tcp_addr` / `quic_addr` / `faketcp_addr` 在 accept 端是本端监听地址（hop ingress 的 listen 地址，端口按载体取传输的 `listen.port` / `quic_port` / `faketcp_port`），dial 端仍是对端地址；`server_public` 只给 dial 端，`Check` 拒绝 accept 端带它。

## Agent Control 下发与状态（第 2b 步）

- 协商：Hello 列出 `network.v1` 且带合法 `device_capabilities`（`device_ref` 等于本会话节点），并且本会话协商了 config.v1，HelloAck 才列 `network.v1`；forward 节点同时获得 package-reports.v1。
- 每次 Hello 记录协商标志与能力（`v4_kernel_network_device.negotiated` / `capabilities_json`）；标志为真时设备配置文档带成员 `network`（无计划时只有 `device_ref`、generation 0），否则不带。
- 模型写入使设备 generation 变化后，提交即推送这些设备的配置（触发 `network`）；分钟级刷新兜底。
- `NetworkStatus` 以 PackageReport（`network` / `network.status` / `v1`）上报；只在协商了 `network.v1` 的会话接受，`device_ref` 必须是本节点；最新一份存 `status_json`，其 `public_keys` 写入公钥并重新规划（pending 路径随之进入计划）。

## 后续步骤

1. **内核**（线程「内核支持网络模型出口与路径」）：`DirectOutbound` 源地址、`egress:<id>` 出站、`PUT /v1/network` 读同一份 `NetworkPlan`，入站 `ingress` 键挂入口。
2. **Control**：`Line` / `Ingress` / `Egress` / `Transport` / `Path` 表（`NodeEntry` 双写），`anx_line` 链路密钥用 `ca_kek` 封存，planner 生成每设备计划随 `ConfigSnapshot` 下发（只给协商了 `network.v1` 的会话），接收 `NetworkStatus`，出口设备的 anx-server 公钥从状态的 `public_keys` 取。
3. **Agent**：`network.v1` 执行器——出口策略路由（20000–20999，抽自 `nategress` 与 WireGuard `network_policy`）、`anx_line` 链路：`dial` 端写 anx-node 的 `anx_line.links`，`accept` 端经 anx-server 管理面 `AddUsers`/`AddLinks`；状态由 `LinkStats` 填。
4. **转发规则并入路径**（N8）：一条转发规则是一条目标固定的路径，入口设备上 forward 入站 `outbound = anx-line:<transport id>`。

## 第 5 步：转发规则整体迁入网络模型，删除 gost（N8）

所有者在 2026-10-28 决定：转发规则一次性迁入网络模型，删掉 gost；AnixOps 中继也并入网络模型；nftables（内核 DNAT）保留，作为不经过我们内核的纯端口映射引擎。

**传输按链路是否可信来选（所有者 2026-10-28）**

- ANX-Line（`anx_line`）不带外衣，只用在国内或可信链路上：用户入口直连 IX、专线（IPLC/IEPL）入口，以及我们自己控制的两端之间。
- 跨境段（经公网出境）走 ANX-A 系列，也就是带外衣的 ANX（anx-node 的 `anx` 入站形态）。网络模型为它新增传输类型 `anx`。跨境段不用 WireGuard（会被 GFW 封锁），也没有过渡方案：`anx` 落地之前不迁移跨境的规则（所有者 2026-10-28）。
- ANX-A 一期：传输类型 `anx` 由 ANX-Line 链路的 l2 形态承载（C1/TCP 加 A2 外衣），复用多跳、统计和换钥；主规格的 ANX 客户端出站放二期（内核线程 2026-10-28 确认）。
- 规划器按段选择：入口到 IX 或专线入口用 `anx_line`，经公网出境的段用 `anx`。不允许把 `anx_line` 放在公网跨境段上；由于 Line 的可信与否只有 Control 知道，这条规则在规划器里检查，不在 Agent 或合约校验里检查。

客户端只连入口端口，gost 只在我们自己的节点之间做链路，所以迁移对用户透明，切换时链路会重连一次。

**一条规则映射成什么**

- 入口设备上的一个端口转发（`PortForward`）。
- 若干条传输，按跳串起来：可信段用 `anx_line`，跨境段用 `anx`（ANX-A）。这相当于商业中转面板里的「隧道」，多条规则可以共用。
- 最后一跳设备上的出口。

**内核要补的能力（替代 gost 前必须先有）**

1. forward 入站支持多个目标：轮询、随机、IP 哈希、最少连接、按优先级故障切换，并带健康检查和熔断。参数直接沿用 forward.v1 的 `Policy`，目标在最后一跳解析和拨出。
2. 中继：内核已有（anx-server 按 SetLinkRoutes 把上一跳的链路转给下一条链路）。Agent 在中继设备上把 `forwarding` 的「transport → transport」下发成 SetLinkRoutes。
3. 入口高可用：同一个 `PortForward` 下发到多个入口设备。
4. 每条规则的限速、最大连接数和 PROXY protocol（v1/v2）。目标选择、健康检查和 PROXY 头都在入口的 forward 入站做，中继对字节透明，所以多跳也成立。

PortForward 由 Agent 转成 anixcore 的 forward 入站（出站为 `egress:<id>` 或 `anx-line:<id>`），状态和字节取自 `/v1/inbounds` 的统计；内核的 `PUT /v1/network` 不读 port_forwards。

**合约**

- forward.v1：
  - `Engine.ENGINE_NETWORK = 4`、`LinkSecurity.LINK_SECURITY_NETWORK = 7`（链路走网络模型的某条传输，`anx_line` 或 `anx`）、`ProxyProtocol.PROXY_PROTOCOL_V1 = 3`。SDK 校验默认拒绝这三个新值，Control 用 `validate.Options.EnableNetwork` 打开。
  - `LinkTransport.network_transport_id = 6`。
  - `Hop.network_egress_id = 7`。
  - 迁移完成后，gost 的取值（ENGINE_GOST，以及 TLS / WSS / QUIC / GRPC 链路）只保留编号，规划器拒绝这些取值。
- network.v1：
  - `NetworkPlan.port_forwards`，消息为 `PortForward{id, listen, protocols, targets, oneof next}`，之后再加 policy 和 limits。
  - `NetworkStatus.port_forwards`，携带累计字节，Control 写入 `KernelForwardCounter`。

**Control 的规划（已实现）**

- ENGINE_NETWORK 路由规则：每一跳都是 ENGINE_NETWORK，入口之后每跳一个设备；入口端口必须显式给出；后续跳不填 port、dial_address、mux、server_name、path，链路填 `network_transport_id`，出口填 `network_egress_id`；不支持直连模式。不查转发节点库存，因为这些节点是网络模型设备。
- 自动补全（2026-10-30）：路由只需选设备，链路的 `network_transport_id` 和出口的 `network_egress_id` 可留空，由 Control 补齐。补出的组件只进计划、不写回文档，id 以 `auto-` 开头（文档不得使用）：没有线路的设备得到线路 `auto-<node>`；出口（及无链路路由的其他入口）没有互联网出口时得到 `auto-<node>-internet`（nat_direct，source auto）；留空的链路得到上一设备的跳出口 `auto-<from>-hop`、本设备的跳入口 `auto-<to>-hop`（监听 24443，对外地址取节点 host）和传输 `auto-<from>-<to>`（`anx`，A2 外衣，SNI www.apple.com），链路密钥与声明的传输一样生成。节点没有 host 时该链路留空、路由待定。声明的组件从不改动。`GET /api/v4/kernel/network` 的 `nodes` 列出所有已连上 Control 且未停用的 Agent 节点，路由编辑器从这里选设备。
- forward.v1 规划器只校验这类路由，不生成 NodeForwardState，也不分配端口。每次接受的规划在同一事务里重新规划网络模型：路由变成入口设备的 PortForward，链路按传输规划，到期或超配额的路由暂停后端口转发随之撤下。
- 入口没写监听地址时用 `::`。目标策略取 `policy.target`，限速和最大连接数取 `limits`，健康检查、熔断、PROXY 头照搬。
- NetworkStatus 的 port_forwards 计入路由的第 0 跳计数：字节比上次小就是内核重启，开新的计数周期（`network-<毫秒>`）；比已存计数旧的状态忽略；达到配额触发重新规划。
- 目标健康：`PortForwardStatus.targets`（`ForwardTargetHealth`：状态、连续失败、熔断截止、最近一次检查的往返和时间）由 Agent 从 anixcore 的 `GET /v1/inbounds/{tag}/targets` 取得。入口的检查经整条路径拨到目标，所以记作第 0 跳、入口设备的上游健康：`GET /routes/{id}/health`、延迟历史、诊断和 DNS 入口切换都读它，不再依赖 forward.v1 报告。
- 多入口（入口高可用）：入口跳可以有多个设备，路由需要 `listen.entry_hostname`。每个入口设备各有一份 PortForward。第一个设备走第 1 跳指定的 `network_transport_id`（单跳路由走出口跳的 `network_egress_id`）。其余设备由规划器选：从该设备出发、到达同一个入站的传输（按 id 取第一条启用、有链路密钥、且不往别处续接的），之后沿用路由余下的传输；单跳路由则选该设备上启用的互联网出口（同 kind 优先，再按 id）。选不到的入口单独挂起，原因里写明是哪个入口，其他入口照常运行。健康、延迟、诊断和 DNS 入口切换对每个入口设备分别判断。

**迁移**

- Control 提供一次性迁移：
  - 每对相邻跳生成或复用一条传输：两端之间是 IX、专线或国内链路时用 `anx_line`，经公网跨境时用 `anx`（A2 外衣）。
  - nftables 规则不迁移，原样保留。
  - 规则改成 ENGINE_NETWORK，出口沿用该节点的互联网出口。
  - 先 dry-run 列出变化，再执行。
  - 已实现：`POST /api/v4/kernel/network/migrate-gost`（`dry_run`）。新增组件以 `mig-` 开头；新加的跳入口监听 24443，拨号地址取跳的 dial_address 或节点第一个地址；两端都标了 link=iepl/iplc 才用裸 `anx_line`，否则用 `anx`，SNI 取原 gost 链路的 server_name，没有就列为受阻；入口沿用已分配端口，客户端不用改。一跳多节点（出口 HA）、设备没有线路的路由留在 gost 并列出原因。所有可迁的路由在同一次规划里一起改，规划被拒则都不改。
- 转发专用节点改为安装 anx-node 和 anx-server，不再装 gost。
- 最后删除：
  - Agent 的 gost 驱动（forward/gostswap、plugin/gostmesh），以及安装脚本里的 gost。
  - Control 的 `internal/gost` 和 gost 下发。

**顺序**

1. 合约。
2. 内核：多目标与策略、限速、PROXY protocol，以及 ANX-A 作为网络模型传输（anx-node 拨出、anx-server 接收）。
3. Agent：PortForward、中继转发、计数。
4. Control：规划与迁移。
5. 面板。
6. 灰度迁移。
7. 删除 gost。
