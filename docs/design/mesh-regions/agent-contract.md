# 组网：Control 与 Agent 的接口约定

状态：已与 Agent 约定（2026-11-25），Control 侧实现见 `internal/meshregion`，Agent 侧见 anix-agent `docs/mesh.md`。组网在节点内核中运行；Control 只做规划与展示。代码与文档统一称「组网」，标识符为 `mesh`；对接的现有程序称「现有组网程序」（Agent 中为 `existing`），不出现其产品名。

## 1. 能力协商

- Agent 在 `Hello.capabilities` 中列出 `Capability{name: "mesh", version: "v1"}`（`agentcontrol.CapabilityMesh`）。只有带内核的节点列出。
- Control 在门控功能 `mesh`（`internal/features`，发布版默认关闭）开启时，对列出该能力的 proxy 与 forward 节点回 `mesh.v1`；forward 节点同时获得 `package-reports.v1`。
- 下发不依赖协商；上报只在协商后接受。

## 2. 下发：配置成员 `mesh`

节点期望配置文档（两种格式）在节点属于已启用地区时带成员 `mesh`；不带该成员表示节点不运行组网，Agent 停止已运行的组网网络。

```json
{
  "networks": [
    {
      "id": "cn",
      "name": "mesh-cn",
      "secret": "<网络密钥>",
      "secure_mode": true,
      "cipher": "aes-gcm",
      "address": "10.150.0.5/16",
      "role": "full",
      "listen": [{ "transport": "tcp", "port": 11010 }, { "transport": "udp", "port": 11010 }],
      "peers": [{ "server": "edge-1.example.com", "port": 11010, "transport": "tcp" }],
      "subnets": { "accept": [], "advertise": [] },
      "routing": { "policy": "latency", "paths": [] },
      "dns": { "enabled": true, "suffix": "mesh.internal", "names": [{ "name": "nas.cn", "address": "10.150.128.7" }] },
      "host_groups": [{ "cidr": "10.127.0.0/24", "gateway": "<实例 ID>", "backup_gateway": "<实例 ID>" }]
    }
  ],
  "takeover": { "process_names": [], "service_names": [], "config_paths": [] }
}
```

| 字段 | Control 的取值 |
|---|---|
| `id` | 地区代码，Agent 用作网络标识，去掉后其余字段即内核组网配置的扁平形式 |
| `name` / `secret` | `mesh-<代码>`；密钥由 Control 生成（`crypto/rand.Text`），只出现在期望配置中 |
| `secure_mode` / `cipher` | 地区网络固定 `true`（R30）与 `aes-gcm` |
| `address` | 节点在该地区服务器段（网段前半）的固定地址，带地区前缀长度 |
| `role` / `listen` | 固定 `full`，监听 TCP 与 UDP 11010 |
| `peers` | 该地区其他边界节点的主机地址 |
| `subnets.advertise` | 仅边界节点：其他已启用地区的网段 |
| `routing` | `latency`，暂无自定义路径 |
| `dns` | 内网域名（R28）：`suffix` 为面板设置的后缀；`names` 为全部地区的名称，`name` 写作「名称.地区代码」（不含后缀），来源为节点、设备、宿主组成员与网关局域网设备。重名的名称不下发，只在面板标出「重名」。面板下发的名称优先于成员主机名 |
| `host_groups` | 宿主组（R21、R25）：网关所属地区的网络中列出该组；`gateway` / `backup_gateway` 为网关与备用网关上报的实例 ID（见第 3 节 `instance_id`）。网关未上报实例 ID 时不下发该组，备用网关未上报时省略 `backup_gateway` |
| `takeover` | 部署层数据，由管理员在面板设置（后续阶段），仓库中无默认值；为空时省略 |

`dns` 与 `host_groups` 只在对应内核版本支持后生效（内核拒绝未知字段）：Agent 按本机内核版本省略内核尚不支持的部分，面板始终下发。

普通节点 `networks` 只有所属地区一项；边界节点在每个已启用地区各一项。任一地区、成员或边界节点变更时，Control 立即推送受影响节点的配置。

## 3. 上报：`mesh.status`

`PackageReport`：`plugin_id` `mesh`，`kind` `mesh.status`，`version` `v1`，每 60 秒一次，变化时立即上报。

```json
{
  "instance_id": "2f6d7c1e-0b8a-4c55-9e0e-6a1f3b9d2c10",
  "networks": [
    {
      "id": "cn",
      "running": true,
      "error": "",
      "address": "10.150.0.5/16",
      "forward_secrecy": true,
      "peers": [{ "address": "10.150.0.1", "subnets": [], "via": [], "hops": 1, "latency_ms": 12, "forward_secrecy": true }],
      "rejected_advertisements": 0,
      "overlaps": [],
      "alerts": [{ "code": "address_conflict", "address": "10.150.0.9", "peer": "", "detail": "" }]
    }
  ],
  "takeover": { "found": false, "stopped": [], "error": "" }
}
```

- `alerts.code`：`address_conflict`（地址冲突）、`address_unmanaged`（地址未受管，R29）、`unsupported`（内核无组网功能）、`start_refused`（内核按 R12 拒绝启动）、`takeover_pending`（现有组网程序仍在运行，等待接管）。界面对未知代码原样显示。
- Control 校验载荷为带 `networks` 数组的 JSON 对象，保存每个节点最新一份（`v5_mesh_status`）。
- `instance_id`：内核的节点实例 ID（组网状态快照的 `self.id`，首次启动时生成，重启与升级不变），1–64 个字母、数字或连字符，不合规时按未上报处理。宿主组网关的实例 ID 新增或变化时，Control 立即推送该地区节点的配置。
- 「控制面失联」由 Control 按最后接收时间判断（超过 3 分钟），Agent 不上报。

- Control 只接受组网成员节点的上报；不在任何地区的节点的上报按 `not_assigned` 拒绝。
- `peers[].address`：落在某地区设备地址池中的地址视为在用，Control 据此刷新该设备的「最近使用」时间（R29：连续 30 天未使用才可回收）。

## 4. 无 Agent 设备（R31）

未安装 Agent 的设备（无 Agent 节点的安装命令、手机与个人电脑的客户端）经 HTTP 接入，不走 Agent Control 流。功能 `mesh` 关闭时以下接口一律回 404 `feature_disabled`；接口按来源地址限流。

1. 管理员在 节点 → 组网 → 设备 签发一次性接入凭证（`mse_` 开头），指定主机名（一个 DNS 标签）、地区（可不指定）与有效期（1 至 168 小时，默认 24）。凭证只显示一次，Control 只保存其 SHA-256。
2. 设备以凭证接入：

   ```
   POST /api/v5/mesh/device/enroll
   {"token":"mse_…","device_key":"<设备身份密钥，16 至 128 位 base64 字符>","latencies":{"cn":35.2,"eu":180}}
   → 201 {"data":{"device_id":5,"device_token":"msd_…","config":{"networks":[…]}}}
   ```

   - `device_key`：设备应持久保存。同一 `device_key` 再次接入沿用原设备、地区与地址，旧设备令牌失效。
   - `latencies`：设备到各地区的往返时延（毫秒，按地区代码），凭证未指定地区时取已启用地区中时延最低者。
   - 凭证未知、已使用或已过期回 401 `unauthorized`。
3. 此后以 `Authorization: Bearer <device_token>` 调用：
   - `GET /api/v5/mesh/device/config`：回 `{"data":{"networks":[…]}}`，形状与第 2 节配置成员 `mesh` 的 `networks` 相同（无 `takeover`）：`role` 为 `access`、`listen` 为空，地址取自设备地址池，`peers` 为该地区的边界节点。地区停用时 `networks` 为空，设备应停止该网络。设备每分钟读取一次。
   - `POST /api/v5/mesh/device/report`：请求体即第 3 节 `mesh.status` 载荷（不超过 256 KiB），成功回 204，不带任何指令。超过 3 分钟无上报显示为「控制面失联」。
   - `POST /api/v5/mesh/device/region-change`：`{"region":"eu","reason":"可选，至多 500 字"}`，申请迁移到其他地区，回 201；管理员在「地区变更」中批准后设备获得新地区的地址，下次读取配置即生效。每台设备同时至多一个待审批申请。
   - 令牌被吊销或设备被删除后回 401；重新接入需新凭证。
4. 每次读取配置或上报都记为设备地址的使用；连续 30 天未使用的地址可回收给其他设备，设备再次读取配置时重新分配。

## 5. 上报：`mesh.region_request`

节点所有者在节点上申请迁往另一地区（R24，内核命令 `mesh_request_region`）时，Agent 发送 `PackageReport`：`plugin_id` `mesh`，`kind` `mesh.region_request`，`version` `v1`，协商规则同 `mesh.status`。

```json
{ "region": "eu", "reason": "节点已迁至法兰克福" }
```

- `region` 为目标地区代码，`reason` 最多 500 个字符，可省略。
- Control 以上报会话的节点身份记录一条来源为 `member` 的待处理申请，节点只能为自己申请。目标地区不存在、已是当前地区、节点不在组网中或已有待处理申请时按 `bad_payload` 拒绝。
- 不回执；网络管理者在面板批准后，节点经配置推送获得新地区的网络与地址，拒绝时配置不变。

## 6. 不在本约定内

Agent 节点不使用一次性接入凭证，直接以 mTLS 身份取得配置。接管设置（`takeover`）的面板入口、转发节点加入组网在后续阶段另行约定。
