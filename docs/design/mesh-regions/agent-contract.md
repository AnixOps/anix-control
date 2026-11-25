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

## 4. 不在本约定内

入网凭据与无 Agent 节点的只读上报（R31）、设备地址池（R29）、地区变更审批（R27）在后续阶段另行约定。Agent 节点不使用一次性入网凭据，直接以 mTLS 身份取得配置。
