# 5.1 商业版与授权：审计记录

本文是 5.1（社区版改 AGPL-3.0、商业版离线授权）的审计记录：每项决定由谁在何时做出，每一步改了什么，
证据在哪里，每条对外承诺靠什么保证，以及尚未解决的问题。每一步合并后在这里追加，不改写已有记录。
规则本身见 [`LICENSING.md`](../../LICENSING.md)，进度清单见 [`ROADMAP.md`](../../ROADMAP.md) 的 5.1 节。

时间均为 UTC。

## 1. 决策记录

| 日期 | 决定 | 决定人 | 记录方式 |
| --- | --- | --- | --- |
| 2026-10-26 | 支付、订单、优惠券、推广佣金归商业版；公开仓库改 AGPL-3.0 + CLA；社区版不限量；永久授权 + 更新期；随 5.1 推出 | 项目所有者 | 项目讨论记录 |
| 2026-10-26 | 实现等 5.0.0 正式版之后再开始 | 项目所有者 | 项目讨论记录 |
| 2026-11-01 | 后端商业功能移出公开仓库（#457），商业部分放在私有仓库覆盖构建 | 项目所有者授权，开发执行 | PR #457 |
| 2026-11-12 | license 格式与校验放在私有共享库（ANIX1 离线 token，Ed25519） | 项目所有者授权，开发执行 | 私有仓库 PR |
| 2026-11-22 02:39 | 采纳 5 条实施建议：AGPL 只用于本仓库；授权型 CLA + “贡献永远开源”反向承诺；没有 license 时行为等同社区版、更新期外的版本在升级前拦下；公开承诺“已开源功能不挪到商业版”；规则先于实现公开 | 项目所有者 | 项目讨论中的决策卡片 |
| 2026-11-22 02:44 | 第 1 步两个 PR 审阅通过，可以合并 | 项目所有者 | 项目讨论记录 |
| 2026-11-22 12:55 | 暂不设立公司，签约主体问题搁置；第 2 至 4 步不需要主体，第 5 步（开始销售）前或第一位外部贡献者签署 CLA 前必须解决 | 项目所有者 | 项目讨论记录 |

## 2. 实施步骤与证据

| 步骤 | 内容 | PR | 合并提交 | CI 证据 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 1a | 许可说明、CLA、贡献指南、NOTICE、CLA 检查、ROADMAP 清单 | [#569](https://github.com/AnixOps/anix-control/pull/569) | `714b913` | [CI/CD Pipeline 44977323101](https://github.com/AnixOps/anix-control/actions/runs/44977323101) success；[CLA 44977323178](https://github.com/AnixOps/anix-control/actions/runs/44977323178) success（head `bc8608a`） | 已合并 2026-11-22 |
| 1b | 官网“授权与价格”页 `/licensing`（`/pricing` 跳转） | [AnixOps-website#21](https://github.com/AnixOps/AnixOps-website/pull/21) | `54099ea` | [Build 44977472571](https://github.com/AnixOps/AnixOps-website/actions/runs/44977472571) success（head `4a9a572`） | 已合并 2026-11-22 |
| 1c | CLA 检查加固 + 本审计记录 | [#571](https://github.com/AnixOps/anix-control/pull/571) | — | 首次运行 [CLA 44981496387](https://github.com/AnixOps/anix-control/actions/runs/44981496387) 失败：新检查把 AI 编码助手账号 `claude` 的提交判为未签署，证明规则 1 生效；随后补充 AI 账号规则与 PR 作者检查 | 进行中 |
| 1c（补记） | 上一行的合并结果 | [#571](https://github.com/AnixOps/anix-control/pull/571) | `b2d6c87` | [CI/CD Pipeline 44981674797](https://github.com/AnixOps/anix-control/actions/runs/44981674797) success；[CLA 44981674779](https://github.com/AnixOps/anix-control/actions/runs/44981674779) success（head `71b23cf`） | 已合并 2026-11-22 03:47 |
| 1d | 商业私有仓库对齐公开 main：路由配置合并脚本、测试、表名前缀 `v2_` → `v5_`，`PUBLIC_REF` 固定到 `5b2d889` | 私有仓库 PR #4 | `2eef094` | 私有仓库 CI 45023150216 success（head `fa53b5a`） | 已合并 2026-11-22 12:46 |
| 2 | 公开仓库扩展点：新建前配额检查（社区版放行）、前端路由与菜单扩展点、页脚“源代码”链接 | 见下方第 2 步说明 | — | — | 进行中 |
| 3 | 商业版接入 license、授权页、前端商业页面移出 | — | — | — | 未开始 |
| 4 | `LICENSE` 改为 AGPL-3.0 | — | — | — | 未开始 |
| 5 | 发布 5.1.0 | — | — | — | 未开始 |

### 第 2 步说明

新建入口的范围是逐个查代码得到的（2026-11-22，公开 main `5b2d889`）：

| 对象 | 入口 | 检查位置 | 被拒绝时 |
| --- | --- | --- | --- |
| 用户 | 身份平台 `CreateSubscriber`（注册和管理员新建都走这里：`packages/identity-platform/native` 的 `auth.go`、`admin.go`，拒绝原因原样显示给操作者） | `internal/kernelidentity/server.go`，事务内写入前 | gRPC `FailedPrecondition` |
| 节点 | 管理员手动添加 `POST /admin/nodes` | `internal/service/node_service.go` `CreateNode` 事务内 | HTTP 403 |
| 节点 | 节点组加入（Agent 用 join token 自动加入） | `internal/nodegroup/join.go`，新建节点前；重新加入已有节点不检查 | 与其他拒绝相同的回答，审计记录原因 `quota` |
| 节点 | 转发节点 `CreateForwardNode` | `internal/kernelforward/nodes.go` `createNodeTx` | gRPC `FailedPrecondition` |

不检查的入口：第一个管理员的认领（`internal/adminclaim`，只在没有管理员时可用）。

“源代码”链接：配置项 `source_url`（默认本仓库地址）经 `GET /api/v4/public/config` 下发，登录页和账户菜单（用户与管理员都有）显示。
只接受 `http(s)` 地址。改了代码对外提供服务的运营者把它指向自己的仓库（AGPL-3.0 第 13 条）。

前端扩展点：`web/src/extensions/index.js` 导出商业版追加的路由和菜单项，社区版四个列表都为空；商业版构建时用自己的文件覆盖。
现有的商业页面仍在公开仓库，按 edition 隐藏，第 3 步移出。

本地验证（提交前，开发容器内）：上述 Go 包 `go test` 通过；前端 `vitest run` 135 个文件 1314 个测试通过；`eslint` 无报错；
`check_commercial_boundary_test.py` 通过。合并依据以 PR 的 GitHub Actions 结果为准。

## 3. 对外承诺与保证方式

| 承诺（LICENSING.md） | 保证方式 | 验证状态 |
| --- | --- | --- |
| 社区版不限制用户数和节点数 | 社区版里没有任何上限代码；第 2 步加入的配额检查在社区版中恒为放行，需配测试 | 现状成立；测试待第 2 步 |
| 已开源的功能不挪到商业版 | `config/commercial-boundary.json` 固定商业文件范围；CI 的 Go Quality Gates 运行 `config/scripts/check_commercial_boundary_test.py`（其中一项检查整个仓库），阻止公开代码引用商业符号；范围扩大必须改这个文件，在 PR 中可见 | 机制已存在；“不扩大”靠评审，无自动阻止 |
| license 离线校验，不联网、不上报 | 待第 3 步：校验代码只做本地签名验证，需有测试证明校验路径不发起网络请求 | 未实现 |
| 超限或更新期结束只影响新建 | 私有库 `Guard.Allow(kind, current)` 只在新建前调用，不涉及已有对象（库内 `TestGuard` 覆盖上限、更新期、无 license 三种拒绝）；待第 3 步接入后加端到端测试 | 库已实现；接入未开始 |
| 没有 license 时等同社区版 | 待第 3 步：调整私有库当前“无 license 拒绝一切”的零值行为 | 未实现，已知差异 |
| 已发布的 5.0.x 永久 MIT | 已发布的标签和发布包附带 MIT `LICENSE`，无法撤回 | 成立 |

第 2 步之后的更新（不改上表，按日期追加）：

- 2026-11-22，承诺 1（不限量）：`internal/quota` 是新建用户和节点前唯一的检查点，`quota.Check` 为空时一律放行。
  `TestNoPublicCodeSetsTheCheck` 扫描整个仓库的非测试 Go 文件，任何公开代码给 `quota.Check` 赋值都会让测试失败；
  四个入口各有一个测试证明检查发生在写入之前、被拒绝时不新建也不改动已有对象。
- 2026-11-22，承诺 4（只影响新建）：检查只在新建事务里、写入新行之前调用；更新、删除、续期、节点重新加入（rejoin）都不经过它。

## 4. CLA 流程

- 文本：[`CLA.md`](../../CLA.md) 1.0，授权型（贡献者保留版权），含反向承诺。正文为自行撰写，未复制第三方协议文本。
- 签署：贡献者在 PR 中向 `.github/cla-signatures.txt` 添加一行，提交即签署，记录永久保留在 git 历史中。
- 检查：`.github/workflows/cla.yml` 调用 `.github/scripts/check_cla.py`（含自测）。规则：PR 作者和每个提交作者都已签署；
  只能为 PR 作者本人签署；已有签名不能删改。所有者与机器人账号免签；AI 编码助手账号 `claude` 的提交只在所有者本人开的 PR 中接受。
- 已知限制：git 的提交作者只是提交里写的邮箱，不经认证，可以冒用；因此 PR 作者（经过 GitHub 认证的账号）也必须已签署。
- 已知限制：`Co-authored-by` 署名不做机器检查，由合并前的评审确认；CLA 检查尚未加入 `main` 分支规则的必需检查。

## 5. 上游来源

AGPL 切换和商业授权的前提是：除上游声明保留的部分外，本仓库代码由项目所有者享有版权。来源核对如下。
本节数据来自 2026-11-22 “剥离 V2Board”工作的分析，方法可复现，切换许可证（第 4 步）前需重新运行并记录结果。

| 上游 | 许可证 | 现状 | 处理 |
| --- | --- | --- | --- |
| V2Board（经社区 fork wyx2685/v2board，分叉点 `cedd83e1`，2025-12-12，PHP） | MIT | 2025-12-24 加入 Go 后端（`d8402536`），同日删除全部 PHP（`6c22a349`）。按 `git ls-tree -r` 的 blob 哈希比对，分叉点与当前 main 完全相同的文件只有 `LICENSE`；仍源自上游的是数据结构与命名（51 张 `v2_` 前缀表、`v2_user` 的字段名） | `NOTICE` 永久保留 Tokumeikoi 的 MIT 声明；5.1 替换 `LICENSE` 时声明不丢失 |
| flux-panel（bqlpfy/flux-panel） | Apache-2.0 | v4.1 转发模块按其 API 与页面仿做（接口层面，非复制代码），v4.2 已删除，迁移 0028 删除其表；洁净室规则见 `docs/forward-clean-room/provenance.md` | **推断**：仿接口不构成复制，不需要 Apache NOTICE；残留未用模型由瘦身工作删除 |

复现方法：`git fetch --tags`（v2.x 标签带完整旧历史）；`git ls-tree -r cedd83e1` 与 `git ls-tree -r HEAD` 比对 blob 哈希；`git diff --shortstat cedd83e1 HEAD`。

## 6. 开发方式说明

- 本仓库的改动由项目所有者指示、AI 编码助手编写。这些提交的作者是 `claude` 账号（`noreply@anthropic.com`），提交信息中的 `Co-Authored-By: Claude` 表示 AI 参与；它们属于所有者本人的工作，不是独立贡献者，CLA 检查只在所有者开的 PR 中接受这类提交。
- 所有验证在 GitHub Actions 中进行，以上 CI 运行编号可直接打开核对。

## 7. 未解决问题

| 问题 | 需要谁 | 影响 |
| --- | --- | --- |
| CLA 与商业许可的签约主体（法定名称）、适用法律与争议解决地 | 项目所有者 | CLA 1.0 目前写的是“AnixOps (AnixOps Studio)”，无适用法律条款 |
| 商业版档位、价格、续费价、付款与退款规则 | 项目所有者 | 第 5 步发布前必须公布 |
| license 签发专用密钥对（与发版签名密钥分开） | 项目所有者生成，私钥只存私有仓库 Secret | 第 3 步需要公钥 |
| 是否把 CLA 检查设为 `main` 的必需检查 | 项目所有者（仓库规则设置） | 不设为必需时依赖评审 |
| 官网 PR 的 Cloudflare 预览构建在 0 秒内失败 | 项目所有者（Cloudflare 面板配置） | 与本项改动无关；以 GitHub `Build` 检查为准 |
| 商业版的 `TestPublicConfigReportsTheCommercialEdition` 等测试需要跟上新增的 `source_url` 字段 | 开发（下次商业仓库同步） | 公开仓库合并本步后，商业仓库前移 `PUBLIC_REF` 时处理 |
