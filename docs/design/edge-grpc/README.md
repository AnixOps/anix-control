# gRPC 边缘代理（edge）设计

状态：已实现（分支 `feat/edge-grpc`，未提交）。E1 至 E5 的代码与测试见下方“实现”一节。

## 1. 目标

面板部署在 A 机器，B 机器运行 nginx 并持有全部公网证书。Agent 的 gRPC 经 B 的 443 端口（域名 `grpc.<域名>`，公网证书由 B 出示）进入，B 再把请求转到 A 的私网地址。A 上不放任何公网证书。

## 2. 已确定的决定

- 部署拓扑：以双机为例（A 跑 Control，B 跑 nginx）。单机是另一套方式，不经过边缘，见 `docs/guide/deploy-single-host.md`。
- B 到 A 的链路是私网（同机房内网、WireGuard 或 VPN），明文 h2c 转发。
- 允许提交转发客户端证书的地址由新增的 `grpc.edge_proxies` 决定，不复用 `server.trusted_proxies`，权限最小。
- 公网证书只在 B 上。A 不做 ACME，不读取 gRPC 的 TLS 文件。

## 3. 数据流

```
Agent --TLS 443（公网证书，B 出示）--> B nginx --h2c 50051（私网）--> A Control
                                       | 覆盖 X-Anix-Client-Cert（客户端证书）
                                       | 覆盖 X-Forwarded-For（$remote_addr）
```

- B 用 `ssl_verify_client optional_no_ca` 请求客户端证书。nginx 校验证书持有者的签名（TLS 握手的 CertificateVerify），但不校验证书链；链由 Control 用内部根校验（`agentpki.VerifyPeer`）。这样 B 不需要 CA 文件。
- Agent 证书由内部根直接签发（`internal/modulepki/authority.go:389`，签发者是自签的根，见 :477），叶子证书即完整链。`$ssl_client_escaped_cert` 只含叶子，足够。这一点在实现时用真实证书再确认一次。
- Agent 首次入职（`AgentEnrollment.Join`）没有客户端证书：nginx 传空头，Control 按“没有证书”处理，入职路径不变。

## 4. 不变量（安全边界）

1. Agent 身份只来自两种来源之一：直连 TLS 的对端证书（原有行为），或边缘头 `X-Anix-Client-Cert`。边缘头只在 TCP 对端属于 `grpc.edge_proxies` 时才被读取。
2. 边缘模式的 gRPC 监听是明文的，因此对 TCP 对端做准入：不属于 `grpc.edge_proxies` 的连接在 accept 时直接关闭，不进入 gRPC 层。
3. nginx 每次都用 `grpc_set_header` 覆盖边缘头，客户端自带的同名头被覆盖。空值与“没有证书”等价。
4. 客户端地址：边缘模式下，`remoteHost`（`internal/grpc/agent_auth.go:286`）取 `X-Forwarded-For` 的第一个不可信地址，只在边缘来源时采信。入职记录的公网地址、“同一公网地址”的重新接入规则依赖它，否则记录的都会是 B。 边缘没有发送地址时，入职被拒绝（`FailedPrecondition`）：没有地址就无法记录，也无法按地址限流。
5. 证书校验仍在 Control。nginx 只转发，不做身份判断。

## 5. 配置

| 键 | 环境变量 | 含义 |
|---|---|---|
| `grpc.edge_proxies` | `ANIX_CONTROL_GRPC_EDGE_PROXIES` | IP 或 CIDR 列表。非空即边缘模式 |

边缘模式下：

- gRPC 监听为明文 h2c，监听地址取 A 的私网地址，`grpc.port` 为 50051。
- 同时设置 gRPC 的 TLS 文件（`tls.cert_file`/`tls.key_file` 或 `grpc.tls_cert_file`/`grpc.tls_key_file`）时拒绝启动，两种传输只能选一种。
- `agent_control.mtls` 的门禁（`internal/config/config.go:821`）把边缘视为满足 TLS 要求。
- 不向 `grpc.public_name` 申请 ACME 证书。
- 就绪检查（`internal/agentinstall/readiness.go`）的 gRPC 证书项改为“由边缘终止，本机无法检查”，并提示检查 B 上的证书。
- `agent_install.grpc_target` 设为 `grpc.<域名>:443`。节点拨号的端口与监听端口可以不同。

这是第 10 个操作员键（只在边缘模式读取）。需要同步 `internal/config` 的操作员键清单、`docs/reference/configuration.md`，以及依赖键数的测试（`internal/config/config_test.go:80`）。

## 6. 改动拆分（PR）

| PR | 内容 |
|---|---|
| E1 | 配置：`grpc.edge_proxies`、环境变量、校验、与 TLS 文件互斥、文档与键清单 |
| E2 | 监听：边缘模式下的明文 h2c 监听，accept 时按 `edge_proxies` 准入 |
| E3 | 身份与地址：从边缘头取证书链（`peerCertificateChain` 的边缘分支）；`remoteHost` 从 XFF 取真实地址 |
| E4 | 门禁与就绪：mTLS 门禁、ACME 跳过、就绪检查的 gRPC 证书项 |
| E5 | 文档与示例：B 上的 nginx 配置（443 双 server 块、`grpc_read_timeout` 足够长）、部署指南、变更日志片段 |

## 7. 测试

- 单元：边缘头只在边缘来源时生效；伪造头被忽略；空头等同没有证书（入职路径）；坏证书被拒绝；XFF 决定 `remoteHost`；非边缘来源在 accept 时被关闭。
- 集成：用 `internal/tests/fakeagent` 在测试里以 gRPC 元数据模拟边缘头，完成入职和控制流。
- 实机：在本机用 Docker 跑 nginx 容器（B）和 Control 容器（A），用真实 Agent 走完入职、断线重连、证书续期（只换 B 上的证书）。

## 8. 风险（必须由 owner 确认）

- **B 被攻破等于能冒充 Agent。** 直连 TLS 时，身份由 Agent 的私钥证明，攻击者拿不到私钥就无法冒充。边缘模式下，身份由 B 转发的头决定。攻破 B 的人可以转发任何曾经在 B 上出现过的 Agent 证书（证书本身是公开的），从而冒充这些 Agent。这是方案 T 的代价，无法通过 Control 的代码消除，只能靠加固 B、限制 A 的防火墙只允许 B 访问、以及及时轮换。
- 私网链路若不是私网（例如跨公网），Agent 的流量和节点凭据会明文暴露。
- 边缘模式下 Control 无法自己检查公网证书是否有效；证书过期由 B 的监控负责。

## 9. 未决与已核实

- 已核实：Agent 证书由内部根直接签发，叶子证书即可通过 `VerifyPeer`。端到端测试（`internal/tests/edgegrpc`）用真实签发的 Agent 证书，只放叶子证书进头，控制流建立成功。
- 已决定：边缘模式下 A 上只有一个 gRPC 入口（明文，只接受边缘），不再保留直连 50051 的 TLS。
- 未覆盖：经边缘的入职（`AgentEnrollment.Join`）的端到端测试。入职记录的地址（`remoteHost`）已有单元测试，见 `internal/grpc/edge_test.go`。

## 10. 实现

- E1 配置：`internal/config`（`grpc.edge_proxies`、校验、`PanelSettings.GRPCEdge`，证书来源 `edge`）。
- E2 监听：`internal/grpc/server.go`（边缘模式下的明文监听，`edgeListener` 只接受边缘地址）。
- E3 身份与地址：`internal/grpc/edge.go`（边缘凭据与头的解析）、`agent_auth.go`（`peerCertificateChain`、`remoteHost` 的边缘分支）。
- E4 门禁与就绪：`internal/agentinstall/readiness.go`（gRPC 证书项在边缘模式下由边缘检查）、`cmd/server`。
- E5 文档与示例：`docs/guide/deploy-two-hosts.md`、`docs/guide/deploy-single-host.md`、`config/deploy/examples/nginx/edge-two-hosts.conf`、`changelog.d/edge-grpc.md`。

## 11. 验证结果

- 单元：`internal/grpc/edge_test.go`（准入、头的解析、只在边缘来源采信、`GetPeerAddr`）、`internal/config/edge_test.go`（布局与校验）、`cmd/server/doctor_edge_test.go`（边缘模式下的 DNS 与端口检查）。
- 端到端（进程内）：`internal/tests/edgegrpc/edge_test.go`。真实签发的 Agent 证书经边缘监听开通控制流；没有证书被拒绝；直连 TLS 上的伪造头被忽略。
- 端到端（真实 nginx）：`internal/tests/edgegrpc/nginx_test.go`，用 `config/deploy/examples/nginx/edge-two-hosts.conf` 原样在 `nginx:1.27-alpine` 容器中运行。需要 Docker，用 `ANIX_EDGE_NGINX_E2E=1` 启用，默认跳过。本地验证通过：带证书的控制流经 nginx 开通；没有证书被拒绝；伪造的头被 nginx 替换，因此被拒绝（`Unauthenticated`）。
- 对照实验：去掉示例中的 `grpc_set_header x-anix-client-cert` 后，伪造头的测试失败（身份被采信），证明这一行是安全必需项，而不只是约定。
- 实验中发现的问题：测试里的 SNI 与示例的 `server_name` 不一致时，请求会落到网页的 server 块，症状是超时或 502，容易误判为双向流的问题。nginx 的双向流在正确的 SNI 下工作，不需要额外的缓冲指令。

