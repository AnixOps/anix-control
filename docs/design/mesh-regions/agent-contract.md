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
      "routing": { "policy": "latency", "paths": [] }
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
| `takeover` | 部署层数据，由管理员在面板设置（后续阶段），仓库中无默认值；为空时省略 |

普通节点 `networks` 只有所属地区一项；边界节点在每个已启用地区各一项。任一地区、成员或边界节点变更时，Control 立即推送受影响节点的配置。

## 3. 上报：`mesh.status`

`PackageReport`：`plugin_id` `mesh`，`kind` `mesh.status`，`version` `v1`，每 60 秒一次，变化时立即上报。

```json
{
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
- 「控制面失联」由 Control 按最后接收时间判断（超过 3 分钟），Agent 不上报。

## 4. 无 Agent 设备（R29、R31、R27）

未安装 Agent 的设备（节点的安装命令、个人设备的客户端）经 HTTP 接入，不走 Agent Control 流。功能 `mesh` 关闭时以下接口一律回 404 `feature_disabled`；接口按来源地址限流。

1. 管理员在 节点 → 组网 签发一次性接入凭证（`mse_` 开头），指定设备类型、名称、地区（可不指定）与有效期（1 至 168 小时，默认 24）。凭证只显示一次，Control 只保存其 SHA-256。
2. 设备以凭证接入：

   ```
   POST /api/v5/mesh/device/enroll
   {"token":"mse_…","device_key":"<设备自选的稳定标识>","name":"可选","latencies":{"cn":35.2,"eu":180}}
   → 201 {"data":{"device_id":5,"device_token":"msd_…","config":{"networks":[…]}}}
   ```

   - `device_key`：8 至 128 位字母、数字或 `._:-`，设备应持久保存。同一 `device_key` 再次接入沿用原设备，同一地区内地址不变。
   - `latencies`：设备到各地区的往返时延（毫秒，按地区代码），凭证未指定地区时取已启用地区中时延最低者。
   - 凭证未知、已使用或已过期回 401 `unauthorized`。
3. 此后以 `Authorization: Bearer <device_token>` 调用：
   - `GET /api/v5/mesh/device/config`：回 `{"data":{"networks":[…]}}`，形状与第 2 节配置成员 `mesh` 的 `networks` 相同（无 `takeover`）。节点设备 `role` 为 `full`，地址取自服务器段；个人设备 `role` 为 `access`、`listen` 为空，地址取自设备地址池。`peers` 为该地区的边界节点。地区停用时 `networks` 为空，设备应停止该网络。设备每分钟读取一次。
   - `POST /api/v5/mesh/device/report`：请求体即第 3 节 `mesh.status` 载荷（不超过 256 KiB），成功回 204，不带任何指令。超过 3 分钟无上报显示为「控制面失联」。
   - `POST /api/v5/mesh/device/region-change`：`{"region":"eu","reason":"可选，至多 200 字"}`，申请迁移到其他地区，回 201；管理员批准后设备在新地区获得新地址，下次读取配置即生效。每台设备同时至多一个待审批申请。
   - 设备被吊销后令牌回 401，地址释放；重新接入需新凭证。
4. 地址：地区网段前半为服务器段（偏移 1 至半段减 1），后半为设备地址池（不含广播地址）。个人设备连续 30 天未使用（未读取配置或上报）后，其地址可分配给其他设备；该设备再次出现时重新分配。

## 5. 不在本约定内

Agent 节点不使用一次性接入凭证，直接以 mTLS 身份取得配置。接管设置（`takeover`）的面板入口、转发节点加入组网在后续阶段另行约定。
