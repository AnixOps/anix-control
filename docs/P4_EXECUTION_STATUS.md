# 首批机器监控与运维闭环验收记录

更新：2026-09-22。旧文件名仅为链接兼容，不将 Control、Agent、NetworkCore 的路线图合并为统一 P4 等级。

| 验收面 | 源码实现 | 验证证据 | 真实环境 |
|---|---|---|---|
| 已认证 WS 逐事件持久化确认 | 已接入实际 Agent WS 会话 | 专项 Go 测试、Agent→Control 跨仓库 E2E 本地通过 | 待演练 |
| 事件唯一约束、独立事务消费 | 已实现，坏事件退避 | 并发接收、重复消费、数据库失败、两个子进程竞争测试通过 | 待多实例预发布 |
| 独立运维工单与时间规则 | 已实现，客户工单不参与 | 持续故障、认领、升级、恢复、复发、关闭、排除测试通过 | 待值班演练 |
| 邮件 + Telegram | 独立 outbox、租约、重试、验证挑战 | 测试发送器通过；没有向真实接收人发送 | 两个渠道待验证 |
| 签名插件与人工操作审批 | 沿用签名链，加不可变变更请求/审计 | 专项签名/生命周期、权限与变更测试通过 | 待真实插件灰度 |
| 运维工作台 | `/maintenance`，角色/设置/工单/操作 | 组件/导航回归、前端构建及浏览器 E2E 通过 | 待实际维护人员演练 |
| Agent 可靠队列和重启预算 | Agent 仓库实现 | 以 Agent 仓库同提交 CI 为准 | 待设备/断网/进程退出演练 |
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

- Control 源码 `3f413a01a460ad7cf749ddf36529b68f876d634b`：[CI/CD Pipeline #35657014802](https://github.com/AnixOps/anix-control/actions/runs/35657014802) 通过。覆盖后端与竞态测试、安全扫描、浏览器 E2E、真实 Agent 进程联调和 Docker 构建。
- Agent 源码 `8b05d6b40e5d1403f3cb4b5099ae37e0fd71328c`：[CI #35652823456](https://github.com/AnixOps/anix-agent/actions/runs/35652823456) 通过，也是 Control 验收固定使用的 Agent 版本。
- NetworkCore 当前记录提交 `f0fb0da376210a65b8207f7de7095b1e4320b58e`：[CI #35650630501](https://github.com/AnixOps/networkcore_anixops/actions/runs/35650630501) 通过；诊断契约源码 `fb6934a6e0db0c30b455f31edbfcc6491c078c3b` 对应的 [CI #35650045804](https://github.com/AnixOps/networkcore_anixops/actions/runs/35650045804) 通过。该证据仅表示契约、校验与适配边界验收，不表示运行时接入完成。

通知验收使用注入的测试适配器，未向真实接收人发送。真实邮件与 Telegram、Control 第三方外部监测、跨地域和设备演练、灰度观察及维护人员交接仍是生产上线门槛。
