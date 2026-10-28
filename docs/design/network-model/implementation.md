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
