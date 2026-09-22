# 首批机器监控与运维闭环验收记录

更新：2026-09-22。旧文件名仅为链接兼容，不将 Control、Agent、NetworkCore 的路线图合并为统一 P4 等级。

| 验收面 | 源码实现 | 验证证据 | 真实环境 |
|---|---|---|---|
| 已认证 WS 逐事件持久化确认 | 已接入实际 Agent WS 会话 | 专项 Go 测试、Agent→Control 跨仓库 E2E 在当前 CI 通过 | 待演练 |
| 事件唯一约束、独立事务消费 | 已实现，坏事件退避 | 并发接收、重复消费、数据库失败、两个子进程竞争测试通过 | 待多实例预发布 |
| 独立运维工单与时间规则 | 已实现，客户工单不参与 | 持续故障、认领、升级、恢复、复发、关闭、排除测试通过 | 待值班演练 |
| 邮件 + Telegram | 独立 outbox、租约、重试、验证挑战 | 测试发送器通过；没有向真实接收人发送 | 两个渠道待验证 |
| 签名插件与人工操作审批 | 沿用签名链，加不可变变更请求/审计 | 专项签名/生命周期、权限与变更测试通过 | 待真实插件灰度 |
| 运维工作台 | `/maintenance`，角色/设置/工单/操作 | 组件/导航回归、前端构建及浏览器 E2E 通过 | 待实际维护人员演练 |
| Agent 可靠队列和重启预算 | Agent 仓库实现 | 固定 Agent 提交的全量测试、竞态和 protobuf CI 通过 | 待设备/断网/进程退出演练 |
| NetworkCore 诊断契约 | 校验/序列化/显式适配器 | 以 NetworkCore 同提交 CI 为准 | 运行时接线属于后续 |
| Control 独立外部监测 | 生产配置声明与验收记录校验已实现 | 单元测试仅验证 fail-closed 校验；没有真实发送 | 待部署者配置并演练，禁止提前标记宕机告警交付 |

只有通过的实际 Actions 结果可标记 CI 验收；本地验证不能替代 CI，更不能替代生产可用性。

## 验证入口

- `GOWORK=off go test ./internal/maintenance ./internal/service ./internal/handler ./internal/router ./cmd/server`
- `GOWORK=off go test -race ./internal/maintenance ./internal/service ./internal/handler -run '^TestMaintenance' -count=1`
- Agent 构建节点测试二进制后，`ANIXOPS_AGENT_E2E_BINARY=<test-binary> GOWORK=off go test ./internal/handler -run '^TestMaintenanceCrossRepositoryAgentToTicket$' -count=1`。CI 固定 Agent SHA，调用真实 Agent SyncManager 与磁盘 outbox，不读取生产配置。
- 前端 `npm test` / `npm run build` 及现有 machine-telemetry 浏览器 E2E 继续由 CI 执行。

## 上线限制与回滚

详细运行手册见 [MAINTENANCE_P0.md](MAINTENANCE_P0.md)。事件自由文本不保存，外部通知采用至少一次交付；发送完成但写回前进程崩溃可能重复通知。通知停用不停止故障事件持久化。生产正常启动只验证 schema；`-migrate-schema` 才会修改数据库。上线需要独立备份、恢复演练和批准的维护窗口。

未执行生产升级、系统网络/数据库操作、真实邮件/Telegram、外部平台账户配置、跨地域或设备测试。后续转发、WireGuard、隧道、NAT、流量、NetworkCore 客户端/MITM 与生产交接仍保留在各仓库路线图中。

## CI 证据

- Control 源码 `724dd7d471a5d09f57b68b3b27a93fec9ad606f5`：[CI/CD Pipeline #35667452540](https://github.com/AnixOps/anix-control/actions/runs/35667452540) 通过。覆盖生产配置与安装器、后端与竞态测试、安全扫描、浏览器 E2E、真实 Agent 进程联调、PostgreSQL 迁移/恢复和 Docker 构建。
- Agent 源码 `e9ea5ce3e784ee60e34a052a4f682d872e9d32e2`：[CI #35667433060](https://github.com/AnixOps/anix-agent/actions/runs/35667433060) 通过，也是 Control 验收固定使用的 Agent 版本。
- NetworkCore 当前记录提交 `f0fb0da376210a65b8207f7de7095b1e4320b58e`：[CI #35650630501](https://github.com/AnixOps/networkcore_anixops/actions/runs/35650630501) 通过；诊断契约源码 `fb6934a6e0db0c30b455f31edbfcc6491c078c3b` 对应的 [CI #35650045804](https://github.com/AnixOps/networkcore_anixops/actions/runs/35650045804) 通过。该证据仅表示契约、校验与适配边界验收，不表示运行时接入完成。

## 3.2 转发交付证据

- Agent 源码 `bedbd2651936e0891728ab8e9511f73eb14d59fb`：[CI #35669599170](https://github.com/AnixOps/anix-agent/actions/runs/35669599170) 通过。该流水线以特权网络命名空间验证 IPv4/IPv6 TCP/UDP nftables DNAT、计数器、Agent 重启与强制退出后的恢复，以及精确回滚。
- Control 源码 `4cf77284c3eebbc91e79735cfad04d13caad0eff`：[CI/CD Pipeline #35672522857](https://github.com/AnixOps/anix-control/actions/runs/35672522857) 通过。该流水线覆盖转发回归、Control 到 Agent 进程联调、竞态、全量后端测试、前端、数据库恢复和镜像构建，并验证生产验收证据文件默认拒绝占位值和不完整灰度记录。
- 真实环境证据仍未执行。部署者需从 `config/examples/forwarding-acceptance.yaml.example` 创建被 Git 忽略的 `config/forwarding-acceptance.yaml`，填写实际提交、制品摘要、节点、时间线、回滚演练与负责人批准，再运行 `anix-control -config config/config.yaml -check-forwarding-evidence config/forwarding-acceptance.yaml`。该校验成功前不得把 3.2 标记为生产可用。

通知验收使用注入的测试适配器，未向真实接收人发送。真实邮件与 Telegram、Control 第三方外部监测、跨地域和设备演练、灰度观察及维护人员交接仍是生产上线门槛。
