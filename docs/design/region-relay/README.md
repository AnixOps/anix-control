# 地区控制面中继（region relay）

状态：第一步（中继程序与 Control 的 PROXY v2 接入）已实现；Agent 侧与地区设置见第 8 节的分步计划。

## 1. 需求

来源：项目需求表 R33、R34（2026-11-25，由 owner 提出并确认）。

- **R33 地区控制面中继。** 地区内放一台中继机，主面板只与中继通讯，中继再与本地区所有节点（装 Agent 的节点，以及只读上报在线状态与延迟、不装 Agent 的机器）通讯。中继无网页界面、无 API，只做纯转发：主面板与节点之间端到端认证和加密，中继看不到也改不了控制内容，不保存密钥和配置。建议两台互为备份（可与地区边界节点同机），主面板到中继走专线骨干；中继或主面板不可达时，节点按最后一次配置继续运行。先做中国大陆。
- **R34 大陆节点一律经中继，中继兼做发布文件缓存。** 中国大陆的 Agent 一律连本地区中继，不回退直连面板。中继同时缓存签名的安装包和升级包；节点照常验签，中继没有签名私钥，无法生成或修改发布文件。

## 2. 方案

中继只做四层透传（TCP），不终止 TLS。

```
大陆节点 Agent --TLS（面板证书，按面板名校验；Agent 客户端证书）--> 中继 :50051
              中继 --TCP 原样转发，前置 PROXY v2 头（节点地址）--> Control :50051
大陆节点 --HTTPS GET /v1/sha256/<hex>?src=<发布地址>--> 中继缓存 :8443
              缓存未命中：中继经 upstream_proxy 或直连从 origins 下载一次，校验 SHA-256 后保存
```

- **端到端。** Agent 拨中继地址，但 TLS 的 SNI 和证书校验名仍是面板的 gRPC 名（Agent 已有的 `GRPCHost` 与 `GRPCServerName` 分离）。面板验证 Agent 的客户端证书，Agent 验证面板证书。中继不持有任何一方的私钥，看到的只是 TLS 密文；它能做的最坏的事是断开或拖慢连接。
- **来源地址。** 中继在每条连接前写一个 PROXY v2 头，内容是节点的地址。Control 只对 `grpc.relay_sources` 里的来源读这个头，其他来源一律不读（别人伪造的头会被当成 TLS 握手的开头而失败）。这样入网记录的公网地址、按地址的规则仍然针对节点，而不是中继。
- **先等节点开口。** 节点总是先发（TLS ClientHello）。中继收到节点的第一批字节（最多等 10 秒）后才拨面板，再依次写 PROXY 头、这批字节，然后双向转发。只连一下就关的连接（Agent 测中继的连接耗时）不会在面板上留下空连接。
- **抗资源耗尽。** Control 读完 PROXY 头后恢复 gRPC 自己的握手超时，发一个字节就挂起的连接会被关掉；中继转发节点首包后，面板须在 `handshake_timeout`（30 秒）内应答，每个节点地址最多 `max_connections_per_source` 条连接。缓存把来源地址规范化（主机小写、路径重新编码，带查询串或片段的直接拒绝），记住每个来源实际的 SHA-256 一小时，同一来源换别的哈希来请求直接返回 404，不再下载；每个地址的并发请求有上限，可以只对 `allowed_sources` 服务。
- **与边界节点同机。** Control 按来源 IP 信任 PROXY 头，同机的其他进程也能用这个 IP 连 Control。同机部署时，出口规则只放行 `anix-relay` 用户访问 Control 的 gRPC 端口；`grpc.relay_sources` 只写中继自己的地址，宽于 /24（IPv6 /64）会被拒绝。
- **与边缘模式的区别。** 边缘代理（`grpc.edge_proxies`，[edge-grpc](../edge-grpc/README.md)）终止 TLS，用请求头替节点担保证书，攻破代理等于能冒充节点。中继不终止 TLS，攻破中继不能冒充节点，也不能读控制内容。两者不能同时用（`grpc.relay_sources` 与 `grpc.edge_proxies` 互斥）。
- **发布文件缓存。** 只提供按 SHA-256 取文件。节点在请求里带上原始发布地址（`src`），中继只从配置的 `origins` 前缀下载，下载后校验哈希，不一致就丢弃并返回 502。哈希来自面板经端到端控制通道下发的升级请求（`UpgradeArtifact.sha256`、`UpgradeKernel.sha256`），签名由 Agent 用内置的发布公钥校验，所以缓存的完整性不依赖中继，也不依赖缓存的 TLS。缓存仍走 HTTPS（中继自己的证书，节点照常校验），以免明文暴露下载的内容。
- **无 API。** 中继只有两类监听：转发端口（TCP 透传）和缓存端口（只接受 `GET`/`HEAD /v1/sha256/<64 位小写十六进制>`）。没有列表、上传、管理、指标接口。运行状态看 systemd 与日志（JSON，写 stderr）。

## 3. 中继程序 `anix-relay`

源码：`cmd/anix-relay`、`internal/regionrelay`、`internal/proxyproto`（与 Control 共用）。配置文件（YAML，未知键拒绝启动），默认路径 `/etc/anix-relay/relay.yaml`：

```yaml
forwards:
  - name: control-grpc
    listen: ":50051"
    # Control 的地址，按顺序尝试；连上的那个会一直用到它失败。
    upstreams: ["panel.example.com:50051"]
    proxy_protocol: true        # Control 必须把本机列入 grpc.relay_sources
    # max_connections: 20000    # 超出的连接直接关闭
    # dial_timeout: 2s           # 远小于 Agent 等面板应答的 5 秒
    # handshake_timeout: 30s     # 转发节点首包后，面板须在此时间内应答
    # max_connections_per_source: 256   # IPv6 按 /64 计为一个来源
    # keep_alive: 30s           # 两侧 TCP keepalive；不设空闲超时，控制流是长连接
cache:
  listen: ":8443"
  tls_cert_file: /etc/anix-relay/cache.pem   # 对节点使用的缓存域名有效的证书，改动后一分钟内生效
  tls_key_file: /etc/anix-relay/cache.key
  dir: /var/cache/anix-relay
  origins:                                   # 只从这些 https 前缀下载，防止成为开放代理
    - https://github.com/AnixOps/anix-agent/releases/download/   # 只到具体仓库的发布文件：大小写变体与 latest 别名不在前缀内
    - https://github.com/AnixOps/anix-control/releases/download/
    - https://panel.example.com/
  # redirect_hosts: [objects.githubusercontent.com, release-assets.githubusercontent.com]  # 重定向只能去 origins 的主机和这些主机
  # upstream_proxy: http://10.8.0.1:3128     # 经骨干的 HTTP 代理下载；此时由代理解析和连接，内网限制要在代理上做
  # allow_private_origins: false             # 默认拒绝连到内网、回环、链路本地及其他保留地址（含 NAT64、6to4）
  # allowed_sources: [10.0.0.0/8]            # 只服务这些节点地址；防火墙也只对本地区节点开放缓存端口
  # max_requests_per_source: 16              # 每个地址（IPv6 按 /64）同时打开的请求数
  # max_connections: 1024                    # 缓存端口同时打开的连接总数，超出的排队等待
  # max_misses_per_source: 30                # 每个地址每小时最多触发的下载数；单个地址同时等待的来源少于 max_fetches
  # max_bytes: 21474836480                   # 20 GiB，按最近使用淘汰；下载中的文件不计入，磁盘另需 max_fetches × max_object_bytes
  # max_object_bytes: 2147483648             # 单个文件 2 GiB
  # max_fetches: 4
  # fetch_timeout: 30m                      # 单个响应最长 1 分钟加两倍 fetch_timeout
```

- `anix-relay -config <file> -check` 只校验配置。
- 转发：每条节点连接独立拨上游；上游失败时按顺序换下一个，并记住可用的那个。关闭时关掉所有连接。
- 缓存：同一个哈希同时只下载一次，其他请求等待同一次下载；下载中断或哈希不符时不留任何文件；启动时清掉上次留下的半成品。
- 中继本身不保存节点身份、密钥或配置。它自己的持久数据只有缓存目录里的发布文件（公开内容）和缓存证书。

## 4. Control 改动

- 新操作员键 `grpc.relay_sources`（`ANIX_CONTROL_GRPC_RELAY_SOURCES`）：中继在 Control 看来的地址（IP 或 CIDR）。这些来源的连接必须在 10 秒内送出合法的 PROXY v2 头（IPv4/IPv6 TCP，或 LOCAL），否则关闭；头里的源地址替换连接的对端地址。见 [configuration.md](../../reference/configuration.md)。
- 实现：`internal/grpc/relay.go` 在 TLS 握手之前包装监听器（`internal/proxyproto.Listener`）。头在连接自己的 goroutine 里读，不阻塞 accept。
- 按地址的规则：Control 的 gRPC 侧只有入网记录与「同一公网地址」的重新接入规则读地址（`remoteHost`），有了 PROXY v2 头后它们看到的就是节点地址，不需要改成按证书。Agent 的身份本来就只来自客户端证书。

## 5. Agent 改动（anix-agent）

- 拨号地址与证书名分离：已支持（`GRPCHost` 拨号，`GRPCServerName` 校验面板证书）。
- 中继列表（`GRPCRelays`）：测 TCP 连接延迟选最快的；断开后指数退避并换另一台；定期复测，只有明显更好才切换。中继先接受 TCP 再连面板，所以 Agent 以经中继收到面板的第一批字节为准判断中继可用，TCP 连上但面板没有应答也算这台中继失败。中国大陆节点不回退直连面板。列表写在本地配置里，重启不依赖面板。
- 升级下载顺序：先本地区中继缓存（按 `sha256` 和原始地址），再控制连接，再原始 https 地址。签名与哈希校验不变。

## 6. 运行与故障

- 两台中继互为备份，可与大陆边界节点同机。Agent 拿到两台的地址，任一台故障时切到另一台。
- 中继或主面板不可达：Agent 的数据面按最后一次配置继续运行（现有行为：配置快照在本地，控制流断开不清空数据面），控制流按退避重连。
- 中继到主面板走专线骨干：在 `upstreams` 里写骨干上的面板地址。
- 生产部署由 owner 经 AnixOps-ansible 执行。

## 7. 安全边界

| 中继被攻破后能做的 | 不能做的 |
|---|---|
| 断开、拖慢、丢弃连接（可用性） | 读或改控制内容（TLS 端到端） |
| 伪造节点的来源地址（只影响入网记录与按地址的规则） | 冒充节点（需要节点私钥）或冒充面板（需要面板证书私钥） |
| 拒绝提供缓存文件 | 提供被改过的发布文件（哈希与签名在节点上校验） |

## 8. 分步计划

| 步骤 | 内容 | 仓库 |
|---|---|---|
| 1 | 中继程序（透传、PROXY v2、缓存）；Control `grpc.relay_sources`；本文档 | anix-control |
| 2 | Agent 中继列表、选择与切换；升级先走中继缓存；需求文档 | anix-agent |
| 3 | 中继列表按节点组维护（`relays`、`release_caches`，迁移 0049）：组的安装命令带 `--relay`、`--release-cache`，安装脚本经中继预检、加入并写入配置；已装节点经期望配置成员 `control_route` 下发（只增不删：组里删掉中继不会从已装节点撤走，避免断联）。见 [agent-onboarding.md](../../guide/agent-onboarding.md#region-relays) | anix-control、anix-agent |
| 4 | 部署角色：中继 systemd 服务、证书、防火墙；Control 加 `grpc.relay_sources` | AnixOps-ansible |
| 5 | 无 Agent 机器的只读上报（R31）走同一中继 | 随组网模块 |
