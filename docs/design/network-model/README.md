# 网络模型：入口、传输、出口

**状态：** 设计草案（2026-10-26），待所有者拍板 §17。尚未改代码；面板数据结构的大改动等本文定稿后再动，之后每个迁移步骤是独立的 PR。

**范围：** Control 的数据模型与 planner、anix-agent 的下发与 Linux 执行、执行内核（anixcore，`AnixOps/networkcore_anixops` 的 `apps/anixcore`）的出口与选路。面板和 Agent 要按本文逐步补齐。

来源：用户 2026-10-26 的决定——所有网络部分都必须拆成**入口、传输、出口**三个组件；哪怕是海外直连，也是三个组件放在同一台机器上。IX 只是国内段入口，传输走专线，出口大多是 NAT 直出，少数是已配好 NAT 的独立 IP。双网卡机器（例如 CN2 + 9929，每块网卡一个 v6 入口、一个 v4 出口，再加服务商增值的 v4 NAT 入口）按同一模型处理。这是基础设施里最核心的一层，后面会接入更多、更复杂的方案，所以本文把扩展点放在第一位。

## 1. 原则

1. **组件而不是"节点"。** 面板和 agent 不再把一台机器当成一个不可分的节点，而是管理它上面的入口、出口，以及机器之间的传输。一台机器可以同时扮演多个角色，角色属于"路径中的位置"，不属于机器。
2. **设备挂线路，线路挂入口和出口。** 线路就是一个上联（通常是一块网卡加它的网关），入口和出口都属于某条线路，这样"同线路进出""交叉""故障切换"才有意义。
3. **分流只在入口做一次。** 用户流量在第一个入口被分类、选定整条路径；之后的中继只按入口给出的路径转发，不再查规则（类似 MPLS 的标签转发）。这让中继无状态、可替换，也避免两处规则不一致。
4. **类型是数据。** 入口、出口、传输都有 `kind` 和带版本的 `settings`；接入新方案就是新增一个 `kind`，不改表结构，不改其他 kind 的代码。
5. **一处拥有一件事。** 策略路由由 agent 写，地址绑定由内核做，健康判断有唯一的拥有者（见 §6.3），避免两个进程抢同一张路由表。

## 2. 术语

| 术语 | 含义 | Linux 里对应什么 |
|------|------|------------------|
| 设备 Device | 一台机器（有 agent 的受管机器，或 Panabit 这类不受管的外部设备） | 一台主机 |
| 线路 Line | 设备上的一个上联：网卡 + 网关 + 运营商标签（CN2、9929、IX 专线…） | `eth0` + 默认网关 |
| 入口 Ingress | 流量进入我们系统的一个地址：监听地址 + 对外地址 + 协议 | `listen()` 的套接字；NAT 入口是服务商的端口映射 |
| 出口 Egress | 流量离开设备的一个方式：源地址 + 网关 + 健康检查 | `bind(src)` + `ip rule from <src> table T` |
| 传输 Transport | 连接两个组件的载体：同机时是进程内转发，跨机时是一条链路（WireGuard、ANX-Line、iWAN、SS-2022 中继…） | 进程内转发 / 隧道 |
| 路径 Path | 一个入口到一个最终出口的有序组件链 | 一组策略路由 + 链路 |

## 3. 两种视图

**全局视图**（用户看到的）：每条路径只有一个用户入口、一段传输、一个最终出口。

```
入口 ──────────── 传输 ──────────── 出口
(用户连的地址)  (中间所有机器和链路)  (流量真正上公网的源地址)
```

**设备视图**（agent 管的）：传输是由若干"跳"组成的，每一跳在上一台设备的出口和下一台设备的入口之间。中继设备上只有"跳入口"和"跳出口"，没有用户入口，也不做分流。

```
设备 1          设备 2            设备 3
[用户入口]──┐   ┌─[跳入口]──┐     ┌─[跳入口]──┐
            │   │           │     │           │
         本机转发 链路1    本机转发 链路2   本机转发
            │   │           │     │           │
            └─[跳出口]┘     └─[跳出口]┘     └─[最终出口]──▶ 公网
```

入口分两种用途（`audience`）：`user` 给用户连，`hop` 给上一跳连。出口也分两种：`internet` 直接上公网，`hop` 通往下一跳。同一个地址可以同时承担两种用途，但在模型里是两条记录，这样权限、限速、监控才能分开。

## 4. 面板数据模型（anix-control）

下列是逻辑模型，字段名是建议，最终以 anix-control 的迁移为准。

### 4.1 设备 Device

```
Device {
  id, name
  managed: bool            // false = 外部设备（Panabit、IPLC 盒子），只作为路径上的一个点，不下发
  agent_ref                // 受管设备的 agent 身份名
  capabilities             // agent 上报：支持的 ingress/egress/transport kind 与版本
  region, tags
}
```

现有的 `Node`（一台机器）迁移为 `Device`；`Node` 上的协议配置留在用户入口上（§4.3）。

### 4.2 线路 Line

```
Line {
  id, device_id, name           // "cn2", "9929", "ix-专线"
  interface                     // "eth0"；外部设备可以为空
  carrier                       // 运营商/线路标签：cn2 / cmi / 9929 / ix / iplc / ...
  families: [v4, v6]
  gateway_v4, gateway_v6        // 可选；出口没有单独给网关时用这里的
  bandwidth_mbps, cost, tags    // 选路与计费用
}
```

### 4.3 入口 Ingress

```
Ingress {
  id, line_id, name
  kind: direct | nat | ...      // 可扩展，见 §8
  audience: user | hop
  family: v4 | v6
  listen: { address, port }     // 设备上实际 bind 的地址；可以是 "::"
  public: { host, port_map }    // 对外地址；direct 时 port_map 为空，nat 时是服务商的端口映射
  protocols: [...]              // audience=user：用户协议（VLESS、SS-2022、ANX…）
                                // audience=hop：传输协议（WireGuard、ANX-Line…）
  pairing: PairingPolicy        // §5，只在 audience=user 上有意义
  enabled
}
```

- **监听地址与对外地址分开。** v4 NAT 入口在机器上监听的是内网地址的某个端口，用户连的是服务商的公网地址和映射后的端口；两者都要记录，订阅里给用户的是对外地址。
- **NAT 入口也是入口，挂在它实际进入的线路上**（服务商增值的 v4 NAT 通常从某条线路进来，就挂那条）。
- 现有 `NodeEntry`（`internal/nodeentry`，`direct` / `nat`，`Host` + `PortMap`）就是 `audience=user` 的入口的雏形，可以无损迁移：`Host` → `public.host`，`PortMap` → `public.port_map`，再补上 `line_id`。

### 4.4 出口 Egress

```
Egress {
  id, line_id, name
  kind: nat_direct | dedicated_ip | hop | ...
  audience: internet | hop
  family: v4 | v6
  source                        // 源地址；nat_direct 时是本机该线路的地址
  gateway                       // 缺省取线路的网关
  routing: { table, rule_priority }   // 0 = 由 agent 在自己的区间里分配
  health: { targets, interval, timeout, failure_threshold, recovery_threshold }
  enabled
}
```

- `nat_direct`：机器自己的地址出公网（MASQUERADE 或用户态直接 bind 源地址）。
- `dedicated_ip`：独立 IP，NAT 已在上游配好；本机只需要按源地址选路。
- `hop`：通往下一跳的出口，下一跳的入口在 `Transport` 里引用。
- 这是现有 WireGuard 中继 `network_policy.paths`（`internal/service/wireguard_validation.go`，Agent `core/wireguard/network_policy.go`；`interface` / `source` / `gateway` / `priority` / `routing_table` / `rule_priority` + `health_check`）和 Agent `plugin/nategress`（出口网卡、策略表、MASQUERADE、健康检查）的泛化：前者的每条 path 就是一个出口，`priority` 进入 §5 的故障切换顺序。

### 4.5 传输 Transport

```
Transport {
  id
  from_egress_id                // 上一台设备的 hop 出口；同机时为空
  to_ingress_id                 // 下一台设备的 hop 入口；同机时为空
  kind: local | wireguard | anx_line | iwan | ss2022_relay | external | ...
  settings                      // 按 kind 定义并带版本：密钥、承载、MTU、复用...
  external_ref                  // kind=external：不受管的链路（例如 IPLC），只登记
}
```

`local` 表示同一台设备内的转发（海外直连的情况），这样单机部署也是"入口 → 传输 → 出口"三段，代码路径一致。

### 4.6 路径 Path

```
Path {
  id, name
  ingress_id                    // 用户入口
  segments: [transport_id...]   // 按顺序；只有一个 local 时就是单机直连
  egress_id                     // 最终出口（audience=internet）
  labels                        // 供分流规则引用，例如 "jp", "streaming"
  enabled
}
```

路径是可以由面板自动生成的：给定入口、目标出口和配对策略，planner 沿着线路和传输找出组件链。现有 kernel-forward 的 `Route.Hops`（`sdk/forward/model`）（`ENTRY`、`RELAY`、`EXIT`，按节点粒度）可以看成粒度更粗的路径；新模型把粒度细化到线路上的入口和出口。

## 5. 入口与出口的配对

配对策略挂在用户入口上，决定"从这个入口进来的流量从哪个出口出去"：

| 策略 | 含义 | 默认 |
|------|------|------|
| `same_line` | 从哪条线路进，就从同一条线路出（CN2 进 CN2 出） | 是 |
| `cross` | 指定另一条线路的出口（9929 进、CN2 出） | |
| `failover` | 有序出口列表，健康检查失败按顺序切换，恢复后延迟回切（沿用 WireGuard `failback_delay_seconds`） | |
| `rules` | 由分流规则选路径：域名/IP/用户/端口 → 路径标签（现有面板「转发 → 分流」的 egress 路由即属此类） | |

```
PairingPolicy {
  mode: same_line | cross | failover | rules
  egresses: [egress_id...]      // cross 时一个，failover 时有序多个
  rules: [{ match, path_label }]
  fallback: path_id             // rules 没命中时
}
```

约束：

- **地址族可以跨。** v6 入口、v4 出口是常态。内核 DNAT 不能跨地址族，所以这类入口必须由用户态进程终止（anixcore 是用户态，天然满足）；纯 nftables 转发只能用于同族。
- **分流只在用户入口做一次。** 中继设备收到的流量已经带着路径，中继不查规则。路径如何随流量携带由传输决定：每条路径一个链路/端口（今天就能做），或在传输帧头里带路径标签（取决于传输协议是否支持，见 §8 第 5 条）。
- **用户可见性。** 订阅里每个用户入口单独成一项（现有 `NodeEntry` 已是"每入口每协议一项"），名字带线路标签，用户可以自己选 CN2 还是 9929。

## 6. 下发与执行

### 6.1 Control → agent：每台设备一份网络计划

Control 为每台受管设备算出一份带代数（generation）的计划，agent 整份应用、整份回报，沿用 kernel-forward 的 `NodeState` / `generation` / `state_hash` 做法：

```
NetworkPlan {
  generation, hash
  lines:      [...]             // 本机的线路
  ingresses:  [...]             // 本机要监听的入口（含 hop 入口）
  egresses:   [...]             // 本机的出口（含 hop 出口）
  transports: [...]             // 一端在本机的链路
  paths:      [...]             // 以本机为用户入口的路径（含分流规则）
  forwarding: [...]             // 本机作为中继时：入站链路/标签 → 出站链路或出口
}
```

agent 的职责：

1. 校验设备能力（kind 不支持则整份拒绝并回报原因）。
2. 为每个出口写策略路由（§6.2），表号与优先级在 agent 自己的区间里分配并记录归属，撤销时只删自己写的规则（与 `nategress` 的 ownership 检查一致）。
3. 把入口、出口、路径、转发表交给执行内核（anixcore：`PUT /v1/network`；迁移期的 xray / sing-box 按各自 adapter，能力见 §6.3.1）。
4. 回报每个组件的状态：已应用、健康、当前生效的出口、错误。

### 6.2 Linux 出口：源地址 + 策略路由

每个出口一张路由表：

```
ip -4 route replace default via <gateway> dev <interface> src <source> table <T>
ip -4 rule  add from <source>/32 table <T> priority <P>
```

v6 出口同理（`ip -6`）。内核侧只需要在出站套接字上 `bind(source)`，路由就由 `ip rule from` 选中正确的网卡和网关，不需要 `SO_BINDTODEVICE` 或 fwmark；需要按标记分流的特殊 kind 再引入 fwmark。NAT 直出若源地址不是网卡本身的地址，再加 MASQUERADE/SNAT（`nategress` 已有）。

### 6.3 anixcore

- **多监听。** 每个入口一个监听，入站连接的上下文带 `ingress_id`。监听 `::` 时已是双栈（PR #111）。
- **出口 = 绑定源地址的直连出站。** 现有 `DirectOutbound` 增加可选源地址（TCP `bind` 后 `connect`，UDP `bind(source:0)`），每个出口一个实例，命名 `egress:<id>`。
- **路径选择。** 用户入口按配对策略/规则选出路径；第一段是 `local` 就直接交给出口，否则交给对应传输的出站（WireGuard、ANX-Line、iWAN 出站已存在或在建）。现有 `egress::Router`（域名分流）是 `rules` 模式的一个实现，迁移后它选的是路径而不是一个 SS-2022 服务器。
- **中继转发。** 收到 hop 入口的流量后按 `forwarding` 表交给下一段，不查用户规则。
- **健康检查的拥有者：内核。** 每个出口用自己的源地址探测（这样探测走的就是那条线路），失败切换在进程内完成（毫秒级、不改路由表），结果回报给 agent；agent 只在计划变化时改路由表。WireGuard 这类由 agent 直接管理的传输保持现状，健康仍由 agent 做。
- **iWAN 在用户态终止。** iWAN 是三层隧道，但 anixcore 在进程内终止它（地址池、网关、DNS 都在内核里），解出来的 TCP/UDP 流量和其他入口一样带 `ingress_id` 进入路径选择，可以按规则分流，不需要打开 `ip_forward`，也不需要在 Linux 上为地址池写 SNAT。

#### 6.3.1 内核与适配器

模型本身不绑定内核：组件的 `kind` 由设备 `capabilities` 声明的内核能力决定能否下发。最终目标是 anixcore 取代 xray 和 sing-box，迁移期间它们只作为后备。

| 能力 | anixcore | xray（迁移期） | sing-box（迁移期） |
|------|----------|----------------|--------------------|
| 多入口监听、带 `ingress_id` | 是 | 每入口一个 inbound tag | 每入口一个 inbound tag |
| 出口绑定源地址 | `DirectOutbound` 加源地址 | `sendThrough` | `inet4_bind_address` / `inet6_bind_address` |
| 规则分流（`rules`） | `egress::Router` | routing rules | route rules |
| 进程内出口健康检查与切换 | 是 | 部分（`observatory` + balancer） | 部分（`urltest`） |
| iWAN 入口 / 传输 | **只有 anixcore** | 否 | 否 |
| ANX、ANX-Line | **只有 anixcore** | 否 | 否 |

iWAN 和 ANX 是我们私有内核里的协议，planner 只会把它们放到声明了 anixcore 对应能力的设备上（§10 第 5 条）。

### 6.4 Agent 合约（增量，不破坏现有字段）

`anix.agent.v1` 只能增长（`internal/tests/protocompat`、`contracts/proto/descriptors.golden`），所以网络计划不新增 RPC，按现有数据面的做法走：

- **能力协商。** 新增能力名 `network.v1`。Agent 在 `Hello` 里声明，Control 在 `HelloAck.server_capabilities` 里确认；没协商的 Agent 继续按今天的方式运行（单线路、无出口绑定）。
- **计划随配置快照下发。** `ConfigSnapshot.config_json` 增加顶层成员 `network`（即 §6.1 的 `NetworkPlan`），整份替换；`config_revision` / `config_hash` 照旧，Agent 回 `ConfigStatus`。这样计划与协议配置同一个版本号，不会出现"入口已换、出口还是旧的"的中间态。
- **组件状态回报。** 新增 `AgentToControl` 负载 `NetworkStatus`（追加 oneof 编号，仅 `network.v1` 时发送）：

```
NetworkStatus {
  config_revision
  components: [{
    kind: line | ingress | egress | transport
    id
    state: applied | degraded | down | unsupported | error
    detail                      // 人看的原因
    active_egress_id            // 用户入口：当前生效的出口（failover 时会变）
    rtt_ms, loss_ppm            // 最近一次健康探测
    changed_at_unix_ms
  }]
}
```

- **线路发现。** `ObservedState` 增加本机网卡、地址、默认网关列表（只读事实），面板据此建议线路与出口，管理员确认后才写入模型；Agent 从不自己改模型。
- **错误码。** `ConfigStatus.error_code` 增加 `network_capability_missing`（计划里有本机不支持的 kind）、`network_route_conflict`（表号/优先级被非 Agent 的规则占用）、`network_source_missing`（出口源地址不在本机）。

### 6.5 Linux 细节

- **表号与优先级区间。** Agent 只使用自己的区间（建议表 `20000–20999`、规则优先级 `20000–20999`，可在 Agent 配置里改），所有写入记录在状态文件里，启动时对账：区间内不是自己写的规则报 `network_route_conflict` 而不是覆盖。现有 WireGuard `network_policy` 与 `nategress` 的表号迁入同一区间。
- **`rp_filter`。** 多线路机器上从 eth1 进来、回包走 eth0 会被严格反向路径过滤丢掉；Agent 对挂了入口的网卡把 `rp_filter` 设为 2（loose），并在回报里写明。
- **回程对称。** 入口回包必须从进来的线路回去：用户态监听在具体地址上时，回包源地址就是该地址，`ip rule from <入口地址>` 命中该线路的表。所以每条线路至少有一条按入口地址的规则，即使这条线路上没有出口。监听 `::` 的入口要用 `IP_PKTINFO`/`IPV6_RECVPKTINFO`（UDP）或按目的地址 accept 后的本地地址（TCP）回包。
- **转发与 NAT。** 只有 `kind=nat_direct` 且源地址不是网卡地址、或需要转发内核态流量（WireGuard）时才打开 `ip_forward` 并写 SNAT/MASQUERADE；纯用户态代理不需要。iWAN 地址池由 anixcore 在用户态终止（§6.3），同样不需要。
- **MTU/MSS。** 出口可选 `mtu`、`advmss`（写到策略路由上，`nategress` 的 `PolicyRoute` 已有字段）；隧道类传输自己声明 MTU，planner 把一条路径上最小的 MTU 回报到用户入口，用于 UDP 分片决策。
- **DNS。** 出口可选自己的解析器，解析请求同样绑定出口源地址，避免"走 9929 出去、却用 CN2 解析出来的 CDN 地址"。缺省使用设备的解析器。
- **动态地址。** 线路地址由 DHCP/SLAAC 分配时，出口的 `source` 可写 `auto`：Agent 用该网卡的首选地址并在变化时重写规则、回报新地址；面板据此刷新订阅里的对外地址。

## 7. 例子

### 7.1 海外直连（单机）

| 组件 | 内容 |
|------|------|
| 设备 | 东京 VPS，一条线路 `eth0` |
| 入口 | `user` / v4 direct `203.0.113.10:443`，VLESS |
| 传输 | `local` |
| 出口 | `internet` / `nat_direct` 源 `203.0.113.10` |

三个组件在一台机器上，模型与多跳完全一致。

### 7.2 CN2 + 9929 双网卡，加服务商 v4 NAT 入口

```
设备 sh-relay
├─ 线路 cn2  (eth0, gw4 10.0.1.1, gw6 2001:db8:1::1)
│   ├─ 入口 cn2-v6     user  v6 direct  [2001:db8:1::10]:443
│   ├─ 入口 cn2-v4nat  user  v4 nat     对外 198.51.100.7:30000-30099 → 本机 10.0.1.10:443-542
│   └─ 出口 cn2-v4     internet v4 nat_direct  src 10.0.1.10  gw 10.0.1.1  表 20101
└─ 线路 9929 (eth1, gw4 10.0.2.1, gw6 2001:db8:2::1)
    ├─ 入口 9929-v6    user  v6 direct  [2001:db8:2::10]:443
    └─ 出口 9929-v4    internet v4 nat_direct  src 10.0.2.10  gw 10.0.2.1  表 20102
```

- 两个 v6 入口 + 一个 v4 NAT 入口 = 三个用户入口；两个 v4 出口。订阅里出现三项（每项再乘以协议数），名字带 `CN2-v6`、`9929-v6`、`CN2-v4NAT`。
- 默认 `same_line`：`cn2-v6`、`cn2-v4nat` → `cn2-v4`；`9929-v6` → `9929-v4`。
- 想让 9929 进、CN2 出：`9929-v6` 设为 `cross` → `cn2-v4`。
- 想要线路互备：`failover: [cn2-v4, 9929-v4]`，CN2 出口探测失败时进程内切到 9929，恢复后延迟回切。
- agent 写的策略路由：

```
ip route replace default via 10.0.1.1 dev eth0 src 10.0.1.10 table 20101
ip rule  add from 10.0.1.10/32 table 20101 priority 20101
ip route replace default via 10.0.2.1 dev eth1 src 10.0.2.10 table 20102
ip rule  add from 10.0.2.10/32 table 20102 priority 20102
```

v6 进 v4 出跨地址族，由 anixcore 在用户态终止；这台机器若只跑 nftables 转发则只能做同族转发。

### 7.3 腾讯云 → IX → 日本出口 → 日本落地

用户连腾讯云，腾讯云把流量送到 IX，IX 经专线到日本出口机，日本出口机再送到日本落地机，落地机上公网。共 4 台设备、3 段跨机传输：

| 设备 | 入口 | 出口 | 角色 |
|------|------|------|------|
| 腾讯云（上海） | `user`：公有协议或 ANX（用户入口，**分流在这里做**） | `hop`：腾讯云线路 → IX | 用户入口 |
| IX 机 | `hop`：来自腾讯云 | `hop`：IX 专线 → 日本 | 中继（国内段入口在这里结束） |
| 日本出口机 | `hop`：专线对端 | `hop`：→ 日本落地 | 中继 |
| 日本落地机 | `hop`：来自日本出口机 | `internet`：`nat_direct` 或 `dedicated_ip` | 最终出口 |

```
路径 sh→jp-land
  入口    腾讯云/user-v4 (VLESS)
  传输    T1 腾讯云 → IX        ANX-Line 或过渡期 WireGuard
          T2 IX → 日本出口机    专线；ANX-Line 或 WireGuard
          T3 日本出口机 → 落地   ANX-Line / WireGuard / SS-2022 中继
  出口    日本落地/internet-v4 nat_direct
```

全局视图里：入口 = 腾讯云，传输 = T1+T2+T3（IX 与日本出口机都是传输的一部分），出口 = 日本落地。"日本出口机"在这条路径里不是最终出口，但同一台机器可以在另一条路径里当最终出口（加一个 `internet` 出口即可），这正是"角色属于路径而不属于机器"。

如果某段运营商侧已经提供了链路（比如 IPLC 盒子），把那一段登记成 `kind=external` 的传输，两端的设备登记为 `managed=false`，路径仍然完整。

### 7.4 iWAN：客户端协议，也是传输

iWAN 在模型里有两个角色，都只由 anixcore 实现：

1. **用户入口协议。** 用户用 iWAN 官方客户端（iwand、OpenWrt、Panabit）直接连我们的机器，iWAN 是该用户入口 `protocols` 里的一项（面板 #353 已上线）。
2. **传输。** 两台受管设备之间用 iWAN 作为一段 `kind=iwan` 的传输。

典型路径：用户用 iWAN 官方客户端连一台打通大陆与海外的跨境机，跨境机的出口在海外，再接到落地机，落地机上公网。

```
iWAN 客户端 ─(iWAN)─▶ 跨境机 ──────── 传输 ────────▶ 落地机 ─▶ 公网
                    用户入口(iwan)    hop 出口(海外侧)    hop 入口   internet 出口
```

| 设备 | 入口 | 出口 | 角色 |
|------|------|------|------|
| 跨境机（大陆 ↔ 海外） | `user`：iWAN（大陆侧线路，**分流在这里做**） | `hop`：海外侧线路 → 落地机 | 用户入口 |
| 落地机 | `hop`：来自跨境机 | `internet`：`nat_direct` 或 `dedicated_ip` | 最终出口 |

- 跨境机上 anixcore 在用户态终止 iWAN，按规则选路径（例如国内直出走跨境机自己的大陆侧出口，其余走落地机），不需要内核转发地址池。
- 跨境机 → 落地机这一段可以是 `ss2022_relay`、`anx_line`，或 `iwan`（两端都是 anixcore 时）。
- 一台跨境机也可以直接用海外侧线路的 `internet` 出口当最终出口，这时路径只有一段 `local` 传输。

**Panabit 不动的情况。** 如果 iWAN 段由第三方 Panabit 设备提供（`App ─▶ Panabit 入口 ═IPLC═▶ Panabit 香港 NAT ─▶ 落地机`），两端登记为 `managed=false` 的设备、中间是 `external` 传输，我们只下发落地机的入口和出口。

**未加密。** iWAN 没有加密和完整性校验，经过 iWAN 段（不论是入口还是传输）的路径在面板上标注"未加密段"（§13、N9）。

## 8. 扩展点

这一层会不断接入新方案，扩展只通过以下几个口子，不改核心表结构：

1. **新的 `kind`。** 入口（例如 anycast、端口跳跃、CDN 回源）、出口（例如 IPv6 出口池、按用户分配的独立 IP、经上游代理出）、传输（例如多路径、FEC、新的隧道协议）各自登记一个 kind，带自己版本化的 `settings` 校验器。设备通过 `capabilities` 声明支持哪些 kind 的哪些版本，planner 只把设备支持的组合下发给它。
2. **一段传输可以有多条链路。** `Transport` 允许挂多个成员（多路径聚合、主备、按流哈希），对路径来说仍是一段。
3. **出口池。** 配对策略里的出口列表可以换成"出口组"（按标签选出口，加权、最少连接、按用户粘性），与 kernel-forward 的 `BalanceStrategy` 对齐。
4. **选路策略插件。** `PairingPolicy.mode` 可扩展（时延/丢包驱动、成本优先、时间段），但始终只在用户入口执行一次。
5. **路径标签的携带方式由传输声明。** 每种传输声明自己能否在帧里带路径标签；不能的就按"每路径一条链路/端口"退化，planner 自动选。
6. **外部设备与外部传输。** 运营商专线、第三方盒子、对方的 SD-WAN 都以 `managed=false` / `external` 接入，模型不要求我们控制路径上的每一个点。

### 8.1 首批 kind

| 组件 | kind | 主要 settings | 执行者 |
|------|------|---------------|--------|
| 入口 | `direct` | 监听地址/端口、协议列表（含 `iwan`，只有 anixcore） | 内核 |
| 入口 | `nat` | 监听地址、`port_map`（对外端口段 → 本机端口段） | 内核；端口映射由服务商完成 |
| 出口 | `nat_direct` | 源地址（或 `auto`）、网关、MASQUERADE 开关 | Agent 写路由，内核绑定 |
| 出口 | `dedicated_ip` | 源地址、网关 | 同上，不写 NAT |
| 出口 | `hop` | 源地址、网关（通往下一跳的那条线路） | 同上 |
| 传输 | `local` | 无 | 内核进程内 |
| 传输 | `wireguard` | 密钥、对端、MTU、`allowed_ips` | Agent（现有 `core/wireguard`） |
| 传输 | `anx_line` | 承载、密钥轮换、实验开关 | 只有 anixcore |
| 传输 | `ss2022_relay` | 方法、密钥（Control 生成） | 内核（现有分流中继） |
| 传输 | `iwan` | 账号、服务器、MTU、掩码开关 | 只有 anixcore |
| 传输 | `anixops_relay` | 现有 forward 的 AnixOps 中继链路（`LinkSecurity`/`Carrier`） | forward 引擎 |
| 传输 | `external` | 只登记，不下发 | 无 |

后续候选：入口 `anycast`、`port_hopping`、`cdn_origin`；出口 `pool`（按标签选出口组）、`per_user_ip`（用户独享出口 IP）、`upstream_proxy`（经第三方代理出）；传输 `multipath`、`fec`、`gre`/`vxlan`（三层互联）。

## 9. 与现有结构的迁移

| 现有 | 迁移到 |
|------|--------|
| anix-control `Node`（一台机器） | `Device` + 一条默认 `Line` |
| `NodeEntry`（direct / nat，Host + PortMap） | `Ingress(audience=user)`，补 `line_id` |
| 节点 `Host` 与协议端口（无 entry 时） | 默认线路上的一个 `direct` 用户入口 |
| WireGuard 中继 `network_policy.paths` | 每条 path 一个 `Egress`；`strategy=failover` + `priority` → `PairingPolicy.failover` |
| agent `nategress` 插件 | `Egress(kind=nat_direct)` 的 Linux 执行器 |
| kernel-forward `Route.Hops`（ENTRY/RELAY/EXIT） | `Path`；每个 hop 的节点 → 该设备上的 hop 入口/出口 |
| 「转发 → 分流」egress 路由 | `PairingPolicy.rules`，规则目标从 SS-2022 服务器改为路径标签 |
| anixcore `egress::Router` + `DirectOutbound` | 路径选择 + `egress:<id>` 绑定源地址的出站 |

迁移顺序（每步可独立上线、可回退）：

1. anixcore：`DirectOutbound` 支持源地址；`/v1/network` 接受入口/出口/路径（与 `/v1/egress` 并存）。
2. anix-agent：出口策略路由执行器（从 `nategress` 和 WireGuard `network_policy` 抽出公共部分），回报组件状态。
3. anix-control：新增 `Line`、`Ingress`、`Egress`、`Transport`、`Path` 表；`NodeEntry` 双写迁移；面板"设备 → 线路 → 入口/出口"页面。
4. planner：由配对策略自动生成路径与每设备 `NetworkPlan`；kernel-forward 路由改由路径生成。
5. 传输：WireGuard 过渡，ANX-Line 就绪后替换。

## 10. 校验规则（planner 不变量）

Control 在保存和生成计划时检查，违反即拒绝并指出是哪条记录：

1. 出口的 `source` 与 `gateway` 同地址族；`source` 属于该线路（Agent 回报的地址里有它，或是 `auto`）。
2. 同一设备上监听地址+端口+L4 协议不冲突（含 hop 入口和 forward 产品分配的端口）；`nat` 入口的 `port_map` 段互不重叠、长度一致。
3. 用户入口与最终出口地址族不同时，入口所在设备必须声明用户态终止能力（内核 DNAT 不跨族）。
4. 路径无环；每段传输的 `from_egress` 与上一段落在同一设备、`to_ingress` 与下一段落在同一设备；最后一段之后是 `audience=internet` 的出口。
5. 每段传输两端设备都声明支持该 `kind` 的同一版本（`managed=false` 的一端除外）；用户入口里的每个协议也必须被该设备的内核支持（`iwan`、ANX 只能落在声明了 anixcore 能力的设备上）。
6. `failover` 列表里的出口都在用户入口所在设备上（或经同一条路径可达）；`cross` 的出口不能等于 `same_line` 的出口（否则提示改用默认）。
7. 表号、规则优先级在 Agent 区间内且同设备不重复（`0` 由 planner 分配）。
8. hop 入口不能出现在订阅里；用户入口必须至少配对一个可用出口。
9. 规则模式（`rules`）里引用的路径标签至少对应一条启用的路径，否则落到 `fallback`。

## 11. 状态、监控与计费

- **组件状态** 来自 `NetworkStatus`（§6.4），面板在设备页按线路展示，用户入口额外显示"当前出口"。
- **路径健康** = 路径上所有组件健康；任一跳 `down` 时路径 `down`，`failover` 切到下一个出口时路径 `degraded` 并告警（走现有告警推送，出现和恢复各一条）。
- **流量统计** 只在用户入口计（与今天按节点统计一致，避免多跳重复计费）；每个出口、每段传输另计不计费的线路流量，用于看各线路带宽占用和运营商账单对账。
- **倍率** 可挂在用户入口（例如 CN2 入口 1.5 倍、9929 入口 1 倍），取代今天挂在节点上的 `Rate`；没挂时继承设备的倍率。

## 12. 订阅生成

- 每个启用的用户入口 × 该入口的每个协议 = 订阅里一项（沿用 `NodeEntry` 今天的规则）；名字为 `<设备名> <线路标签>-<地址族>[-NAT] <协议>`，例如 `上海 CN2-v6 VLESS`、`上海 CN2-v4NAT VLESS`。
- `nat` 入口给的是对外地址和映射后的端口；没被 `port_map` 覆盖的协议端口不出现在该入口下。
- 出口不出现在订阅里，但可以作为说明附在名字后（例如 `→ 日本`），由路径的 `labels` 决定。
- 用户被限制只能用某些线路时，在订阅分组层面按入口过滤（入口带 `tags`，订阅分组按标签选入口），不新增一层权限。

## 13. 安全

- hop 入口只接受已鉴权的上一跳（传输自身的密钥或证书），并由 Agent 写防火墙规则只放行上一跳设备的地址；不出现在订阅、不对公网宣告。
- 传输密钥由 Control 生成、按设备下发并可轮换（与 WireGuard 现有轮换 `cmd/wgrotate` 一致），从不进订阅。
- `managed=false` 的外部设备与 `external` 传输视为不可信段：经过它的路径必须在两端之间有我们自己的端到端加密，或在面板上明确标注"未加密段"（iWAN 即属此类）。
- Agent 只执行计划、只回报事实，不自行创建入口或出口；线路发现结果需管理员确认。

## 14. 面板界面

- **设备页** 改为树：设备 → 线路（运营商标签、网卡、地址、带宽占用）→ 入口（用户/跳）与出口（当前健康、RTT），每个节点有启用开关。
- **新建向导**：选设备 → 面板根据 Agent 回报的网卡列出建议线路 → 每条线路勾选"v4/v6 入口""v4/v6 出口" → 可加服务商 NAT 入口（填对外地址和端口段）→ 选配对策略（默认同线路）。CN2+9929 这类机器三步完成。
- **路径页**：用图展示 入口 → 各跳 → 出口，标出外部段和未加密段；可从这里设置故障切换顺序和分流规则。
- **用户视角**：订阅里每个入口一项，名字带线路；不暴露出口和中继。

## 15. 边界情况

- **一块网卡多个地址**：一条线路下可以有多个出口（每个源地址一个），例如同一网卡上的两个独立 IP 分给不同用户组。
- **一个地址同时是入口和出口**：海外直连的常态，两条记录共享地址，互不影响。
- **服务商 NAT 入口的回程**：NAT 入口进来的连接在本机看是内网地址，回包走该内网地址所在线路的表；所以 NAT 入口也必须挂对线路（见待拍板 N4）。
- **线路整体故障**：该线路上的入口不可用（用户侧切换由客户端的节点选择完成），出口按 `failover` 切走；回报里两类故障分开。
- **只有 v6 的线路出 v4**：需要对端 NAT64 或另一条线路的 v4 出口（`cross`）；planner 在配对时检查地址族可达。
- **同一设备在不同路径里角色不同**：允许，按路径各自生成 `forwarding` 表项。
- **kernel-forward 的端口转发产品**：一条转发规则就是一条目标固定的路径（入口=分配的端口，出口=最后一跳的出口），沿用同一套出口与策略路由，不再各写一套。

## 16. 测试计划

- **单元**：校验规则 §10 每条一个正反例；planner 由模型生成 `NetworkPlan` 的金样（`contracts/` 下加跨仓库 golden fixture，Agent 与内核各自读同一份）。
- **netns 端到端**（扩展现有 "Forward Netns E2E"）：一个 netns 模拟双线路设备（两张 veth、两个网关 netns），验证同线路配对、交叉配对、拔掉一条线路后的故障切换与延迟回切、`rp_filter`、回程对称。
- **多跳**：四个 netns 串成 腾讯云 → IX → 日本出口 → 落地，验证中继不查规则、路径标签/每路径端口转发正确、只在入口计费。
- **兼容**：未协商 `network.v1` 的 Agent 收到的配置与今天逐字节相同。

## 17. 待拍板

| 编号 | 问题 | 建议 |
|------|------|------|
| N1 | `Node` 是否直接改名/迁移为 `Device` | 是；一台机器一个设备，协议配置下沉到用户入口 |
| N2 | 出口健康检查由内核还是 agent 做 | 内核（用出口源地址探测，进程内切换）；agent 管理的 WireGuard 维持现状 |
| N3 | 路径如何随流量到中继 | 先"每路径一条链路/端口"，传输支持帧内路径标签后切换 |
| N4 | 服务商 v4 NAT 入口挂哪条线路 | 挂它实际进入的线路；不确定时由 agent 探测后提示 |
| N5 | 中继是否允许做第二次分流 | 不允许（原则 3）；确有需要时拆成两条路径 |
| N6 | 计费点 | 只在用户入口计费，倍率挂在入口（§11） |
| N7 | 计划是否随 `ConfigSnapshot` 一起下发 | 是，同一版本号，不新增 RPC（§6.4） |
| N8 | 现有 forward 产品是否并入路径模型 | 并入，但放在迁移最后一步，先让代理节点用上 |
| N9 | 外部设备/外部传输的未加密段是否允许承载用户流量 | 允许但必须在面板标注（iWAN 现状） |
