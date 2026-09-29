# Control Center 合并记录

## 结论（2026-09-29 更新）

`Anixops-control-center` 和 `Anixops-control-center-worker` 两个仓库已经以快照方式合并进本仓库，旧仓库只读归档。之后只在本仓库的 `go_dev` 分支上开发，它是唯一的长期分支。

- `control-center/`：Control Center 应用。快照取自旧仓库 `master` 合并 `production` 后的内容，包括 Go CLI/TUI/服务、Vue Web、Flutter 客户端、Helm/部署/监控资产和测试报告脚本。它是独立的 Go 模块 `github.com/AnixOps/anix-control/control-center`。
- `control-center/workers/`：Cloudflare Workers API，快照取自 `Anixops-control-center-worker` 的 `master`，并删减到 Center 客户端实际使用的核心路由（见下文）。它取代了 Center 仓库中旧的 `workers/` 目录。
- 旧仓库的 git 历史、tag 和 Releases 保留在归档仓库中，本仓库不导入历史和 tag。

早先版本的本文建议「不做整仓物理合并」，理由是两边的模块、发布单元和认证边界不同。现在的做法是按目录隔离，保留这些边界，同时把源码、CI 和分支统一到一个仓库：

| 边界 | 合并后的处理 |
| --- | --- |
| Go module | `control-center/go.mod` 是独立模块；根目录的 `go test ./...`、`go vet`、golangci-lint、govulncheck 都不会扫到它；根 CI 的 gosec 和 swag 显式排除 `control-center/` |
| 发布单元 | Control 镜像的 Docker 构建上下文排除 `control-center/`（`.dockerignore`）；Center 使用 `control-center-v*` tag，发布时不标记为仓库的 latest，不影响 `scripts/install.sh` |
| CI | `.github/workflows/control-center.yml`（Go、Web、Flutter）和 `control-center-workers.yml` 通过路径过滤只在相关目录变化时运行，不是 `go_dev` 规则集的必需检查；根 `ci.yml` 对所有 PR 都运行（必需检查见 `.github/BRANCH_PROTECTION.md`），但它的 gosec、swag 和 Docker 构建上下文都排除 `control-center/` |
| 认证 | 不变。插件页使用 Control `/api/v2` JWT 和 `/api/v3` 合同；其余 Center 页面仍使用 Workers `/api/v1` 会话 |

## 导入时排除的内容

- `release-artifacts/` 中已提交的二进制发布包（约 98MB）。其中的迁移说明和 v2.5.0 公告移到 `control-center/docs/releases/`。
- `memory/`（AI 助手笔记）、Flutter 生成的 `mobile/ios/Flutter/flutter_export_environment.sh`（含本机路径）、旧的 `workers/` 目录。
- 两个旧仓库的 `.github/`：子目录中的 workflow 不会被 GitHub 执行，已移植到根目录的 `control-center*.yml`。
- Worker 仓库的孤儿分支 `main`（只有 LICENSE），以及已过时的机器人 PR（worker 改名）。

## Workers API 删减范围

保留：health/readiness/liveness/metrics、auth、MFA、users（含 me、tokens、sessions、lockout）、nodes、node-groups、playbooks、tasks、schedules、notifications、dashboard、audit-logs、ssh、plugins、agents、logs、backups、batch、SSE/WebSocket，以及 tail worker。

删除：incidents、governance、webhooks、kubernetes、lb、mesh、scaling、ai、vectors、web3、ipfs，以及 `/internal` 开发者模式和生成的 endpoint-visualizer 报告。

`migrations/` 不做改动。已经应用到生产 D1 的迁移（包括 incidents 表）保持原样。

对应地，Center 的 Web 删除了 AI 助手、Web3（含 Web3 登录）页面，以及未挂路由的 ELK/LogSearch/MetricsExplorer/Monitoring/Resilience/ServiceDiscovery/Tracing 模拟页面和它们的模拟数据测试；Flutter 删除了 AI、Web3 功能和对应的 API 客户端。

## 部署注意事项

- Cloudflare Workers Builds 需要把 Git 连接从 `Anixops-control-center-worker` 改为本仓库：根目录 `control-center/workers`，生产分支 `go_dev`，监听路径 `control-center/workers/**`。改完之前，线上仍运行旧仓库最后一次部署的未删减版本；改完后的第一次部署会让被删除的接口从 `api.anixops.com` 下线。
- Center 旧仓库的历史中曾提交过 Android 签名 keystore（后来已删除）。归档后历史仍然公开；如果这把 keystore 签过已发布的包，需要轮换上传密钥。

## 插件生命周期迁移（历史阶段，保留作参考）

1. 冻结边界：`anix-control` 负责插件目录、签名发布物、安装状态、生命周期操作和配置 revision；Control `/api/v2` JWT 是插件管理页唯一的 Control 会话来源。
2. 迁入插件管理体验：Center 的目标卡片、健康状态、失败原因、最近操作、取消操作等已经落到 `web/src/views/admin/Plugins.vue`。
3. 验证和发布：Vue 单测、API 合同测试、Chromium E2E 和真实 Control 进程 gate 已经通过；真实 Agent staging 的生命周期变更、回滚和 operator approval 证据仍待补齐。
4. 客户端收敛（可选）：Flutter、TUI/CLI 和 Workers 作为 `control-center/` 下的 API 客户端继续存在；如果后续要把 Center 的插件页标记为 deprecated，仍以真实 Control/Agent staging 通过为前提。

## 合并完成标准

- `anix-control` 的一个插件管理入口覆盖目录、版本、Control/Agent 安装状态、生命周期、配置和操作历史。
- 不再存在两套 Control JWT 存储或两套 `/api/v3` API 客户端实现。
- Control 生命周期 action 带显式幂等键；Agent 目标的 installation intent 由服务端 lifecycle generation 和派生 identity 去重，并能通过 operation ID 查询最终状态。
- 未验证发布物不能安装，配置更新遵守 revision 冲突保护。
- 源码、CI 和分支统一在本仓库 `go_dev`；旧 Center 与 Worker 仓库已归档。
