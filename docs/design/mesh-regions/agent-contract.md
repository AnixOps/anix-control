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
      "kind": "managed",
      "native_keys": ["<静态公钥>"],
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
      "host_groups": [{ "cidr": "10.127.0.0/24", "gateway": "<实例 ID>", "backup_gateway": "<实例 ID>" }],
      "external_bridge": { "interface": "ext0", "routes": ["100.100.1.2/32", "192.168.50.0/24"], "allow": ["10.127.0.0/24", "10.150.128.7/32"] }
    }
  ],
  "takeover": { "process_names": [], "service_names": [], "config_paths": [] }
}
```

| 字段 | Control 的取值 |
|---|---|
| `id` | 地区代码，Agent 用作网络标识，去掉后其余字段即内核组网配置的扁平形式 |
| `name` / `secret` | `mesh-<代码>`；密钥由 Control 生成（`crypto/rand.Text`），只出现在期望配置中 |
| `kind` | 固定 `managed`：地区网络只接纳本方节点 |
| `native_keys` | 准入清单：该地区网络全部成员节点（第 3 节 Agent 上报的 `static_key`）与持有设备令牌的无 Agent 设备（第 4 节）的静态公钥，每项为 32 字节 X25519 公钥的标准 base64（带填充，44 个字符），排序去重。尚无公钥时为空列表，内核暂缓该网络但仍生成并上报本机公钥。任一公钥新增、变化或移出时，Control 立即推送该地区节点的配置，节点自身的新公钥先推送给其他成员、最后推送给该节点（规格 §8.3.1 第 6 条）；清单之外的对端握手被拒，失败原因 `peer_not_native` |
| `secure_mode` / `cipher` | 地区网络固定 `true`（R30）与 `aes-gcm` |
| `address` | 节点在该地区服务器段（网段前半）的固定地址，带地区前缀长度 |
| `role` / `listen` | 固定 `full`，监听 TCP 与 UDP 11010 |
| `peers` | 该地区其他边界节点的主机地址 |
| `subnets.advertise` | 仅边界节点：其他已启用地区的网段 |
| `routing` | `latency`，暂无自定义路径 |
| `dns` | 内网域名（R28）：`suffix` 为面板设置的后缀；`names` 为全部地区的名称，`name` 写作「名称.地区代码」（不含后缀），来源为节点、设备、宿主组成员与网关局域网设备。重名的名称每个地址各下发一条，内核解析时返回全部地址并标出重名，面板同样标出「重名」。面板下发的名称优先于成员主机名。`resolver_address`：本地区的固定解析地址（规范 §15.9.2 第 8 条），见下文 |
| `host_groups` | 宿主组（R21、R25）：网关所属地区的网络中列出该组；`gateway` / `backup_gateway` 为网关与备用网关上报的实例 ID（见第 3 节 `instance_id`）。网关未上报实例 ID 时不下发该组，备用网关未上报时省略 `backup_gateway` |
| `external_bridge` | 外部网络桥接（R36，规范 §15.12，内核能力 `mesh_external_bridge`）：只在桥接节点所属地区的网络中出现，非桥接节点不带该节。`interface` 为节点在外部网络中的网卡名；`routes` 为管理员配置的外部地址（单个地址写作 `/32`、`/128`）与白名单内子网，面板已按规范校验；`allow` 为允许经桥接访问的来源，由面板按成员身份（节点、持有地址与静态公钥的设备、网关属于本地区的宿主组）展开为当前的虚拟地址 `/32` 与宿主组地址段，按地址排序。成员地址变更、移出地区、撤销或删除时面板立即重新下发 |
| `takeover` | 部署层数据，由管理员在面板设置（后续阶段），仓库中无默认值；为空时省略 |

`dns`、`host_groups` 与 `external_bridge` 只在对应内核版本支持后生效（内核拒绝未知字段）：Agent 按本机内核版本省略内核尚不支持的部分（包括 `dns.resolver_address`；`external_bridge` 只交给列出 `mesh_external_bridge` 的内核），面板始终下发。

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
      "static_key": "<本机在该网络的静态公钥>",
      "address": "10.150.0.5/16",
      "forward_secrecy": true,
      "peers": [{ "address": "10.150.0.1", "subnets": [], "via": [], "hops": 1, "latency_ms": 12, "forward_secrecy": true }],
      "rejected_advertisements": 0,
      "overlaps": [],
      "alerts": [{ "code": "address_conflict", "address": "10.150.0.9", "peer": "", "detail": "" }],
      "externalBridge": { "allowDigest": "<SHA-256>", "routesDigest": "<SHA-256>", "allowCount": 2, "appliedAt": "2026-11-25T21:00:00Z" }
    }
  ],
  "takeover": { "found": false, "stopped": [], "error": "" }
}
```

- `static_key`：本机内核在该网络的静态公钥（内核 `GET /v1/mesh` 的 `static_keys`，按网络名对应），格式同第 2 节 `native_keys`。Control 只采信 Agent 通道的值：新值或变化的值写入该节点在该地区的成员记录并列入 `native_keys`；格式不合规的值、已属于设备或其他节点的值不采用，地区列表中该节点标出「公钥与其他成员重复」（此前已存在的重复同样标出）。
- `alerts.code`：`peer_not_native`（拒绝未准入节点，`peer` 为对端节点 ID；内核失败原因同名，Agent 也可原样放在网络的 `failure.code`，界面一并显示）、`native_keys_pending`（等待准入清单：清单为空或不含本机公钥，内核暂缓该网络，`detail` 为 `empty` 或 `self_missing`；同样可放在 `failure.code`。未运行的网络都因此暂缓时，节点状态显示「暂缓」而非「未运行」）、`native_keys_unsupported`（本机内核不支持准入清单，Agent 扣下该网络不交给内核；内核降级重启后也会出现）、`invalid_kind`（网络 `kind` 既不是 `managed` 也不是 `legacy`，区分大小写，Agent 扣下该网络，其他网络照常运行）、`address_conflict`（地址冲突）、`address_unmanaged`（地址未受管，R29）、`unsupported`（内核无组网功能）、`start_refused`（内核按 R12 拒绝启动）、`takeover_pending`（现有组网程序仍在运行，等待接管）、`external_bridge_unsupported`（本机内核不支持外部网络桥接，Agent 扣下 `external_bridge`）、`external_bridge_apply_failed`（桥接配置未能生效）、`external_bridge_ipv6_forwarding_off`（未开启 IPv6 转发）、`external_bridge_lock_held`（本机已有其他进程持有桥接锁，本节点不声明 `mesh_external_bridge`，规范 #359；持锁进程可能是挂起但规则仍生效的旧实例，带此告警时该网络的地址保留一律不释放，同 `external_bridge_downgrade_residue`）、`external_bridge_downgrade_residue`（降级到不支持桥接的内核后旧桥接规则可能残留，须清理后删除记录文件；带此告警时该网络的地址保留一律不释放）。界面对未知代码原样显示。
- Control 校验载荷为带 `networks` 数组的 JSON 对象，保存每个节点最新一份（`v5_mesh_status`）。
- `instance_id`：内核的节点实例 ID（组网状态快照的 `self.id`，首次启动时生成，重启与升级不变），1–64 个字母、数字或连字符，首字符为字母或数字，不合规时按未上报处理。宿主组网关的实例 ID 新增或变化时，Control 立即推送该地区节点的配置。
- `externalBridge`：桥接节点内核**已生效**的桥接配置摘要，按内核状态（networkcore `docs/architecture/mesh-status.md`，规范 #339）原样转述，三态：对象为已生效的配置；`null` 表示确知该网络未运行桥接；不带该项表示状态未知（下发失败或读不到内核状态，内核可能仍按旧名单桥接）。`allowDigest` 的算法：`allow` 各条目写成 CIDR 文本（IPv4 点分十进制，IPv6 按 RFC 5952 小写压缩，单个地址也带前缀长度），去重后按字节序排序，以 `\n` 连接（末尾无换行），取 SHA-256 小写十六进制；测试向量以规范 mesh-status.md 的「externalBridge 摘要测试向量」表为准（面板测试逐项校验），例如空名单为 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`，`10.150.0.2/32`、`10.127.0.0/24`、`fd7a:115c:a1e0::5/128`、`10.150.0.128/32` 为 `c2125301582f23d00252ec7ea3ae881359c66f9333fcae94ba95c64931257366`（排序为 `10.127.0.0/24`、`10.150.0.128/32`、`10.150.0.2/32`、`fd7a:115c:a1e0::5/128`）。规范 §15.12 第 4 条要求地址回收后先撤名单再分配：某地址或宿主组地址段从桥接节点的名单中移出后，面板把它记为「待桥接确认」，不再分配给其他成员或宿主组，直到该节点上报的 `allowDigest` 等于面板对当前下发名单算出的摘要、`appliedAt` 晚于移出时刻，且当前名单不含它；节点未运行桥接时（无论是否仍应桥接），以移出之后收到的、该网络 `externalBridge` 为 `null` 的上报为准；不带该项但该网络带告警 `external_bridge_unsupported`（Agent 从不向该内核下发桥接）视同 `null`；该网络带告警 `external_bridge_downgrade_residue`（降级到不支持桥接的内核，旧桥接规则可能残留，规范 §15.12 第 9 条）或 `external_bridge_lock_held`（本机其他进程持有桥接锁，持锁者可能是挂起但规则仍生效的旧实例，规范复核 Z2）时一律视为状态未知，不论是否同时为 `null` 或带 `external_bridge_unsupported`，均不释放；不带该项或该网络不在上报中视为状态未知，不释放。以上条件须由移出之后收到的连续两次上报同时满足才释放，迟到或乱序的单次上报不能单独释放；观测时间早于已保存上报、且按面板接收时间在其后 2 分钟内到达的 `mesh.status` 整份丢弃并记警告日志（同一节点多条连接并发上报时按观测时间取最新）；超过 2 分钟视为节点时钟回拨，照常接收。桥接节点离线未确认期间一直保留，桥接列表标出 `bridge_unconfirmed` 并列出这些地址。设备地址池已满且唯一可回收的闲置地址在桥接名单中时，面板先从原设备收回该地址（本次分配回 `pool_full`），待桥接节点确认后再分配；同一时间每个地区只收回一个。节点离开该地区时，为它保留的地址随之释放（其静态公钥同时移出准入清单）。
- 「控制面失联」由 Control 按最后接收时间判断（超过 3 分钟），Agent 不上报。

- Control 只接受组网成员节点的上报；不在任何地区的节点的上报按 `not_assigned` 拒绝。
- `peers[].address`：落在某地区设备地址池中的地址视为在用，Control 据此刷新该设备的「最近使用」时间（R29：连续 30 天未使用才可回收）。
- 地区网段的最后一个可用地址是该地区的固定解析地址（规范 §15.9.2 第 8 条），面板不把它分配给任何成员或设备，并以 `dns.resolver_address` 下发给本地区的节点与设备；节点不自行推算。过渡：该地址仍被保留之前分配的设备占用时，面板暂不下发，地区列表标记 `resolver_address_held`，设备标记 `reserved_address`；设备换址或离开地区后，各节点随配置更新收到该地址。

## 4. 无 Agent 设备（R31）

未安装 Agent 的设备（无 Agent 节点的安装命令、手机与个人电脑的客户端）经 HTTP 接入，不走 Agent Control 流。功能 `mesh` 关闭时以下接口一律回 404 `feature_disabled`；接口按来源地址限流。

1. 管理员在 节点 → 组网 → 设备 签发一次性接入凭证（`mse_` 开头）：
   - 新设备：指定主机名（一个 DNS 标签）、地区（可不指定，由设备上报的时延决定，面板无法核实）与有效期（1 至 168 小时，默认 24）；
   - 重新接入已有设备：指定设备（`device_id`），沿用其主机名、地区、角色与地址。
   - 角色 `role`：`access`（默认，接入端）或 `full`（完整节点，适用于路由器与服务器，须指定地区）。完整节点使用服务器段中的固定地址 `address`（签发时指定，或留空在接入时取最低可用地址）。
   凭证只显示一次，Control 只保存其 SHA-256。已使用的凭证不可撤回，保留为该设备的接入记录。
2. 设备以凭证接入：

   ```
   POST /api/v5/mesh/device/enroll
   {"token":"mse_…","device_key":"<设备身份密钥，16 至 128 位 base64 字符>","latencies":{"cn":35.2,"eu":180},"static_key":"<可选，静态公钥>"}
   → 201 {"data":{"device_id":5,"device_token":"msd_…","config":{"networks":[…]}}}
   ```

   - `device_key`：设备应持久保存。`device_key` 只是标识，不证明持有：新设备凭证遇到已登记的 `device_key` 回 409 `mesh_conflict`。重新接入须使用为该设备签发的凭证；设备已登记密钥时须与之一致（否则 401），未登记时以本次密钥登记。重新接入后旧设备令牌失效。
   - `static_key`：可选，设备在所在地区网络的静态公钥（格式同第 2 节 `native_keys`，不合规回 400）。设备通常在取得配置后才有该网络的公钥，此时省略，由接入后首次上报提交（第 3 项）。为已有设备签发的凭证重新接入时，以本次提交的值替换原值（省略则清空，待首次上报）：更换公钥只能经重新接入。凭证首次使用后 10 分钟内的重试视同同一次接入：设备尚无公钥时登记本次的值，已有不同的公钥回 409。
   - `features`：可选，本机内核的能力名单（同下文读取配置），决定应答中的配置；省略时应答中没有地区网络。
   - `latencies`：设备到各地区的往返时延（毫秒，按地区代码，至多 64 项），凭证未指定地区时取已启用地区中时延最低者。
   - 凭证使用一次即作废，并发请求只有一个成功。接入应答丢失时，同一凭证与同一 `device_key` 在首次使用后 10 分钟内可再次接入，得到新令牌，此前的令牌失效。
   - 请求体不超过 16 KiB（否则 413）。凭证未知、已使用或已过期回 401 `unauthorized`。
3. 此后以 `Authorization: Bearer <device_token>` 调用：
   - `GET /api/v5/mesh/device/config`：回 `{"data":{"networks":[…]}}`，只含本设备的成员信息，字段固定为：`id`、`name`、`kind`、`native_keys`、`secret`、`secure_mode`、`cipher`、`address`、`role`、`listen`、`peers`（该地区的边界节点）、`subnets`、`routing`、`dns`（第 2 节）。设备拒绝其他字段，面板不经此接口下发任何指令。配置按设备内核的能力名单（本机内核 `GET /v1/status` 的 `features`，规范 mesh-status.md）下发：请求带查询参数 `features`（逗号分隔）时以之为准，否则取设备最近一次上报的顶层 `features`；接入请求体中的 `features` 只决定接入应答中的配置，不保存（内核升级或回退后会过时）。未列出 `mesh_native_keys` 时不下发任何地区网络（`networks` 为空）：地区网络只接纳清单中的公钥，内核不处理 `kind` 与 `native_keys` 就不做准入；设备列表按最近一次上报标记「未上报准入能力」（`native_keys_unsupported`），读取配置时的查询参数不计入。未列出 `mesh_dns` 时不下发 `dns` 节；未列出 `mesh_resolver_address` 时 `dns` 中不含 `resolver_address`。查询参数 `features` 至多 4 KiB、64 项，接入请求体中的 `features` 至多 64 项，超出回 400。设备应在接入请求（`features`）与每次读取配置时都带上能力名单。接入端：`role` 为 `access`，`listen` 与 `subnets` 为空，地址取自设备地址池。完整节点：`role` 为 `full`，`listen` 为 TCP 与 UDP 11010，地址为服务器段中的固定地址，`subnets.advertise` 为管理员确认的局域网网段（`PUT /api/v4/kernel/mesh/devices/{id}/subnets`），`subnets.accept` 为空。地区停用时 `networks` 为空，设备应停止该网络。设备每分钟读取一次。
   - `POST /api/v5/mesh/device/report`：请求体即第 3 节 `mesh.status` 载荷，另加顶层 `features`（字符串数组，本机内核的能力名单，可省；不超过 256 KiB；`peers[].latency_ms` 为 0 至 600000 的数值或 `null`，每个网络至多 1024 个对端），成功回 204，不带任何指令。设备尚无静态公钥时，取上报中 `id` 为所在地区代码的网络的 `static_key` 登记，并推送该地区节点的配置；已登记后上报不同的值不予采用，设备列表标出「公钥不一致」，尚未登记的已接入设备标出「未登记公钥」。只有当前有效的设备令牌发出的上报才能登记（重新接入或吊销后，旧令牌在途的上报不予采用）；已属于成员或其他设备的公钥不予登记，标出「公钥与其他成员重复」，接入时提交这样的公钥回 409。设备经批准迁往其他地区时清空公钥（网络名改变，内核为新网络生成新公钥），由迁移后的首次上报重新登记。最近一次上报超过 3 分钟，或接入后从未上报且超过 3 分钟未读取配置，显示为「控制面失联」。
   - `POST /api/v5/mesh/device/region-change`：`{"region":"eu","reason":"可选，至多 500 字"}`，请求体不超过 16 KiB，申请迁移到其他地区，回 201；管理员在「地区变更」中批准后设备获得新地区的地址，下次读取配置即生效。每台设备同时至多一个待审批申请。
   - `POST /api/v5/mesh/device/subnet-request`（R35，仅完整节点）：`{"cidrs":["192.168.1.0/24"]}`，请求体不超过 16 KiB，申请通告本机局域网网段，回 202（无应答体）。`cidrs` 为私有 IPv4 网段，主机位须为 0，至多 16 项，不合规回 400 `invalid_request`；空数组撤回待审批申请；接入端回 409 `mesh_conflict`。每台设备同时至多一个待审批申请，新申请替换旧申请。管理员在设备详情中批准（`POST /api/v4/kernel/mesh/devices/{id}/subnet-request/approve`，请求体 `{"subnets":[…]}` 须与当前申请一致，否则 409）后，申请的网段替换已确认网段，检查同 `PUT …/subnets`（与地区、主机组总网段、保留网段、其他完整节点或外部网络桥接重叠回 409），设备下次读取配置即生效；驳回（`DELETE /api/v4/kernel/mesh/devices/{id}/subnet-request`）保留原已确认网段。不提供查询申请状态的接口，设备以读取到的 `subnets.advertise` 为准。
   - 令牌被吊销或设备被删除后回 401；重新接入需为该设备签发新凭证。
4. 吊销与删除使设备的静态公钥移出 `native_keys`（吊销同时释放该公钥，重新接入时重新提交），Control 立即推送该地区节点的配置，各节点随即关闭并拒绝该设备的会话；删除后其地址可能分给其他设备。设备仍持有地区网络密钥，但地区网络只接纳清单中的公钥；如仍需更换网络密钥，在地区列表中「更换网络密钥」（`POST /api/v4/kernel/mesh/regions/{id}/rotate-secret`）：新密钥立即推送到该地区全部节点，设备在下次读取配置时获得，期间该地区连接短暂中断。
5. 读取配置或上报都记为设备地址的使用（最近通信时间每分钟、地址使用时间每小时至多写入一次）；连续 30 天未使用的地址可回收给其他设备，设备再次读取配置时重新分配。完整节点的固定地址不回收。

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
