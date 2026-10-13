# 运维极致简化路线图（5.0 之后的整体方向）

**状态：** 2026-10-12 由所有者逐项选定（S1 到 S11）。本文记录决定、核实过的问题和实施顺序；每个里程碑拆成独立的 PR。

**目标。** 装一个面板只需要一个域名和一条命令，接一台节点只需要一条命令。此后上架套餐、开通用户、确认收款、处理告警、升级面板和节点，每件日常事务都在一个页面内完成，最后只点一次确认。除接新机器外，每周登录服务器 0 次、手改配置文件 0 次。

这份路线图接在 [`../node-onboarding/README.md`](../node-onboarding/README.md)（5.0 接入设计，D1 到 D8）之后，不重新讨论其中已定的内容。无需向后兼容：只有一位维护者和使用者。

## 怎么得出的

10 个视角（首次安装、升级与备份、配置面、后台结构、节点运维、售卖、监控与安全、命令行与 API、维护者流程、节点机操作）读 4.2.1 的代码，数出每件事的步骤和输入，列出 196 条操作负担；每个视角各有一位质疑者逐条复核；5 位设计者分别按「零配置默认值、一屏引导流程、自动驾驶、做减法、手机与消息优先」提出方案，再合成 11 个大方向的选择。未列入选择的 42 项改进是在任何方向下都成立的，见附录。

## 已选定的方向

| # | 问题 | 决定 | 里程碑 |
|---|---|---|---|
| S1 | 面板入口与证书 | Control 自带证书（ACME 申请和续期）；网页、API、安装脚本走 443，节点 gRPC 同一张证书走单独端口；不再需要 nginx、certbot。限制：域名不能套 CDN 代理，80 和 443 归 Control。 | M2 |
| S2 | 数据库与安装方式 | 只保留单程序加内嵌 SQLite（systemd）。Compose、Helm、sqlite2postgres 删除。生产库现为 PostgreSQL 15：需要一次性搬数据或重建，先查用户和订单数量。 | M2 |
| S3 | 升级方式 | 一键升级：每天检查新版本，点一次（或一条命令）完成备份、升级、自检和节点分批升级；自检失败自动回滚且不丢数据。 | M4 |
| S4 | 官方插件包 | 官方包内置，每次启动自动安装、升级、启用（含订单和支付包）；插件中心导入、路由模式和三个下发开关取消。 | M2 |
| S5 | 售卖与收款 | 手动收款一键确认；取消社区版和商业版的区分；删除支付网关页、返佣提现、用户余额；订单和支付包进入每个公开发布。 | M3 |
| S6 | 告警与处理入口 | 单一推送渠道（Telegram 一键绑定，或 SMTP 加收件邮箱），出现和恢复各推一条，链接直达处理按钮。 | M3 |
| S7 | 后台结构 | 任务首页（开站清单加需要处理列表），侧栏约 6 个入口；删除访问组、多管理员和权限模式、路由模式、负载均衡、通知模板、直接改配置键的编辑器。 | M3 |
| S8 | 用户能用哪些节点 | 只留订阅分组一层：节点组挂订阅分组，套餐包含订阅分组；其余四个来源删除。 | M3 |
| S9 | 发布流程 | 一条命令发布两个仓库；Agent 版本与 Control 解耦（Control 内置固定的 Agent 发布清单：标签加校验和）；取消 RC 和 release 分支，只发正式版。 | M4 |
| S10 | 组模板变更 | 模板改动自动生效：节点协议关联组模板，密钥和端口按节点保存；改模板先预览受影响节点，确认一次下发全组。 | M4 |
| S11 | 备份异地副本 | 备份自动上传到对象存储，保留若干份，首页显示最近一次上传结果，失败推送告警。 | M4 |

## 核实过的问题（4.2.1 已发布代码）

1. 按文档搭好的面板接不了节点。/install.sh 和 /install/* 只注册在 8080 的 API 监听上；3000 的网页监听对其他路径一律返回首页 HTML；而文档里的 nginx、Caddy 和 Helm 配置都指向 3000。所以复制的 `curl … | sudo bash` 实际执行的是 HTML（internal/router/router.go:129-138; cmd/server/main.go:1069-1095; docs/DEPLOYMENT.md:496-508,518-522）。
2. 后台新建的套餐永远不会出现在用户的购买页。show 默认是 0，后台表单里没有这个字段，用户只看得到 show=1 的套餐；新装的面板上，在售的只有种子数据里那个 12 元演示套餐（internal/model/plan.go:14; web/src/views/admin/Plans.vue:452-459; internal/handler/user_plan.go:20-21; internal/service/init_subscription.go:273-281）。
3. 你回复工单时看不到客户写了什么。管理端接口只返回标题和数字用户 ID，页面上也写着管理端看不到之前的对话；有新工单时也不通知任何人（internal/handler/admin_ticket.go:24-45; web/src/locales/modules/zh-CN/adminSupportPages.js:345-347; internal/model/notification.go:67）。
4. 除了证书提醒，系统不会主动告诉你任何事。节点离线、订单、工单的通知函数都没有调用者；Telegram 绑定流程走不通；告警没送达也会被标成已通知。首页节点卡统计的是没人写入的旧表，永远显示 0/0（TODO.md:113-114,125-128; internal/kernelalerts/monitor.go:105-111; internal/service/stats_service.go:307-338）。
5. 客户没法在线付款。“去支付”按钮只弹出“请联系管理员”；支付网关页能配置，但没有结账入口。订单和支付包也不在发布里，不另行导入时相关接口直接返回 404（web/src/views/user/Orders.vue:62-63,138-140; CHANGELOG.md:3506-3513; internal/compat/v2/gateway.go:457-468）。
6. 4.2.1 面板默认让新节点下载 Agent v4.2.1，但 Agent 仓库只有 v4.2.0 标签（已核实），安装脚本下载不到就直接失败。临时办法：配置 `agent_install.agent_version: v4.2.0`。根因是两个仓库版本号必须相同的规则（见 S9）（`internal/agentinstall/offline.go:48-52`; `internal/agentinstall/install.sh:645`）。
7. 安装脚本写出的节点配置里没有证书设置。xray 和 sing 上的 TLS 协议会悄悄以不加密方式对外提供，hysteria2 则直接起不来，而节点仍显示健康（internal/agentinstall/install.sh:904-930; snap-agent/core/xray/inbound.go:100-107; snap-agent/core/hy2/config.go:59-64）。
8. 生产库是 PostgreSQL 15，但后台的数据库备份只支持 SQLite，备份页的定时、保留和 S3 设置没有任何代码执行；安装器升级失败时也只恢复程序和配置，不恢复数据库（docs/architecture/release-line-status.md:86-90; internal/service/system_service.go:396-413,466-476; scripts/install.sh:1186-1201）。

## 里程碑

### M0 立即修复

已发布代码里核实的问题，不依赖任何方向选择。

- 规模：一个小 PR（几个提交）
- 改进项：N41

### M1 节点接入可用

在一台真实 VPS 上，从面板复制一条命令到节点出现在订阅里，全程不碰配置文件。5.0 设计的阶段 6 和 7。

- 规模：阶段 6a 接入弹窗和节点组页；6b 诊断页和升级按钮；阶段 7 证书由 Control 下发
- 改进项：N1、N22、N23、N24、N25、N26、N28、N29、N31

### M2 单程序与自带证书

S1、S2、S4 和配置面收缩。装面板只需要一个域名和一条命令。

- 规模：最大的一块，含生产库迁移
- 方向：S1、S2、S4
- 改进项：N2、N3、N7、N8、N10、N32、N33、N34、N36、N40

### M3 日常运营

S5、S6、S7、S8 和售卖、工单、订单的修补。

- 规模：前端为主
- 方向：S5、S6、S7、S8
- 改进项：N4、N11、N12、N13、N14、N15、N16、N17、N18、N19、N20、N21、N27、N35

### M4 升级与发布

S3、S9、S10、S11 和维护者工具。

- 规模：依赖 M2 的单程序形态
- 方向：S3、S9、S10、S11
- 改进项：N30、N37、N38、N39

### M5 删除与 5.0.0

5.0 设计的阶段 8 到 11：删除清单、Agent 瘦身、转发线路进订阅、`/v5` 模块路径、清理迁移。

- 规模：最后做，避免和功能 PR 冲突
- 改进项：N5、N6、N9、N42

顺序：M0 先行；M1 完成后才能在真实机器上验收接入；M2 的单程序形态是 M4 一键升级和备份还原的前提；M5 最后做。S3 的实现依赖 S2（单文件备份）、S4（包随程序升级）和 S9（Agent 发布清单）。

## 风险

- **生产库迁移（S2）。** 生产库是 PostgreSQL 15，改成内嵌 SQLite 需要一次性搬数据，或在新库上重建。动手前先查用户和订单数量，并在迁移工具里做行数和校验和对账。
- **自带证书（S1）。** 需要 80 和 443 空闲且域名不套 CDN 代理；ACME 申请失败时面板必须仍能用自签证书启动并明确提示，否则新机器会装出一个打不开的面板。
- **内置官方包（S4）。** 任何包的修复都要发一个 Control 版本；内置订单和支付包会让商业代码进入每个公开发布。
- **没有 RC（S9）。** 问题只能在正式版上发现，依赖一键升级的自检和回滚兜底，所以 S3 的回滚必须先于 S9 的取消 RC 落地。
- **一键升级的数据丢失窗口（S3）。** 自检失败的回滚不丢数据；事后才发现问题而恢复升级前备份，之后的订单会丢，需要在升级界面写明。
- **手动收款（S5）。** 客户点“我已付款”但没有确认的订单要一直保留并在待处理列表里提醒，不能超时取消。

## 附录：不用选择的改进（N1 到 N42）

1. **N1** 把两个 HTTP 监听合成一个，让 /install.sh 和 /install/* 放在任何反代后面都能访问。现在这两个路径只在 8080 上，经反代访问得到的是网页 HTML（internal/router/router.go:129-138; cmd/server/main.go:1069-1095）。
2. **N2** gRPC 证书改成文件一变就热加载，并新增“面板自身证书将到期”告警（internal/grpc/server.go:135-142; internal/kernelalerts/scan.go:50-57）。
3. **N3** 配置文件改为只覆盖内置默认值，不再整体替换默认值；删掉三份互相矛盾的模板。升级时自动把现有配置改写成只含覆盖值的新文件（删掉已废弃的键，旧文件留 .bak）；已废弃的键只警告，从未存在过的键才拒绝启动（internal/config/config.go:753-773; scripts/install.sh:381-436）。
4. **N4** 删掉没有任何代码读取的设置：cache.*、app.version、app.traffic_log_enable、tls.domain、forward_runtime.iptables_ansible；同时删掉备份页的定时/保留/S3、负载均衡页、通知模板页，以及只记成功、实际不发送的 webhook 渠道（config-8, admin-ui-10, upgrade-9）。
5. **N5** 删掉已退役的 flux 转发运行时：26 个 forward_runtime 键、相关工作线程、NodeX/Ansible 执行器和转发运行时页面。新库不再建 flux 表，旧表由 5.0 一次性迁移直接删除，不再走手输确认短语的删表流程（config-4, cmd/server/bootstrap.go:70-82）。
6. **N6** 删掉兼容期的阶段机制：node-secrets 分阶段命令、forward legacy 删表流程、四种“阶段卡住”告警；所有包路由固定走 native，删除 legacy 处理器和 package_routes.default_mode（现在默认是 rehearsed，两套处理器并存：internal/config/defaults.yaml:132-138）。identity 切换状态要先确认生产库处于哪一步，再删相关代码（cli-api-9, observe-12, upgrade-6）。
7. **N7** 删掉安装器里的 preflight、migrate、rollback 命令，以及 v2board 旧布局识别和 bin/v2board 链接；回滚改由下面的备份还原命令完成（install-14）。
8. **N8** 改用一个根密钥，就是现有的 module_ca_kek（新装时首次启动自动生成）：CA、转发链路 CA 和 DNS 凭据继续直接用它封存；JWT 由它派生；已有 identity.kek 时保留原值（或启动时用旧值解开后重新封存）；加入令牌密钥沿用 5.0 已定的 HKDF 标签 anixops node-group join token v1。启动时 CA 解不开就拒绝启动并说明原因（upgrade-3, internal/modulepki/config.go:24, internal/modulepki/authority.go:133-152, docs/design/node-onboarding/README.md:85）。
9. **N9** 删掉 app.api_token 和 grpc.api_token，连同 /flow/upload 和其余 /api/v2/internal/* 路由一起删除（它们属于已删除的 flux 运行时和 5.0 要删的 auth-keys）。app.api_token 现在连 URL 查询参数里的明文都接受（internal/middleware/middleware.go:235-266; internal/router/router.go:141,634-643）。
10. **N10** 首个管理员改为一次性领取链接：在浏览器里设置邮箱和密码，磁盘和 config.yaml 里不再留明文密码；无论保留哪种安装形态都用同一规则（install-4, install-5, config-10, scripts/install.sh:426-445）。
11. **N11** 账户页加上修改密码和退出其他会话；服务器上加 `anix-control admin reset <email>`，用来找回管理员密码和两步验证（observe-7, cmd/server/module_command.go:38-45）。
12. **N12** 两步验证页删掉从未实现的短信/邮件方式，以及在内核登录路径上不起作用的锁定设置（observe-8, internal/service/mfa_service.go:233-243）。
13. **N13** 审计合成一张表：所有管理写操作、登录和登录失败都记在里面，审计页读这一张表（observe-6, internal/handler/system_audit.go:81）。
14. **N14** 告警只有被真实渠道接收才算送达；送不出去就重试，并在后台显示“告警无法送达”。若 d6 选只看网页，进入需要处理列表即算送达（observe-3, internal/kernelalerts/monitor.go:105-111）。
15. **N15** 首页节点卡改读 v2_node（现在读的是没人写的旧表，永远显示 0/0）。在线/离线只在服务端算一次，离线筛选和首页的“还有 N 个离线”链接就不会再是空列表（internal/service/stats_service.go:307-338; packages/proxy-node/native/nodeadmin.go:123-126）。
16. **N16** 管理端工单详情返回完整对话并显示用户邮箱；有新工单、管理员回复时都发通知（admin-ui-2, internal/handler/admin_ticket.go:24-45）。
17. **N17** 新建的套餐默认就在售，删掉隐藏的 show=0 默认值；流量默认不再是 0；价格按“元”输入；时长用 30天/90天/1年 这样的选项，不再输入 Unix 秒和数字用户 ID；在用户页就能直接授予套餐（business-1, admin-ui-1, admin-ui-4, admin-ui-7）。
18. **N18** 新库不再预置 vip/test 分组、示例节点模板和 12 元演示套餐，删掉的套餐也不会在启动时自己复活；5.0 的清理迁移同时删除生产库里未被改动过的示例行（business-15, internal/service/init_subscription.go:16-286）。
19. **N19** 订单收尾自动化：标记已付和开通放进同一个事务；0 元订单自动完成；未付订单超时只取消没点过“我已付款”的那些，并退回占用的优惠券次数；点过的订单一直保留，并在需要处理列表里提醒（business-10, internal/service/order_service.go:344,397-399）。
20. **N20** 已过期的用户也能登录续费；管理员可以给用户设新密码（business-8, internal/service/auth_service.go:117-119）。
21. **N21** 用户流量的月度重置改由独立工作线程执行，不再依赖 flux 表是否存在（business-14, cmd/server/singleton_workers.go:73-81）。
22. **N22** 第一次接入节点不用填任何东西：空面板第一次打开“接入节点”时，自动建一个“默认”节点组（VLESS Reality，随机端口）。Reality 密钥、short_id 和 SS2022 密钥由服务端生成，也不再偷偷创建 Default VMess。5.0 其余设计不变（admin-ui-9, fleet-9, internal/service/node_service.go:139-160）。
23. **N23** 给出接入命令前先做就绪检查：https、gRPC 证书、CA，以及安装脚本是否能从外网取到。检查不过就显示修复办法，不给一条注定失败的命令；同时提供 `anix-control doctor`（config-2, cli-api-6, internal/handler/agent_install.go:111-125）。
24. **N24** 订阅里只放在线、运行健康、且最近一次配置成功应用的节点，恢复后自动重新放回去（internal/service/subscription_service.go:595-605; internal/grpc/agent_control_config.go:184-191）。
25. **N25** 节点页显示配置“已生效 / 失败：原因”；同一节点端口重复时拒绝保存；每个协议有“重新生成密钥”按钮，改完自动下发（fleet-10, fleet-9, internal/grpc/agent_control_config.go:189-191, internal/service/wireguard_validation.go:24-34）。
26. **N26** 节点上的 TLS 协议没有证书时，拒绝应用并报出原因，不再悄悄以明文提供。这是第 7 阶段由 Control 下发证书之前的过渡做法（agent-side-1, snap-agent/core/xray/inbound.go:100-107）。
27. **N27** 节点月流量超限后自动停止发布，到重置日自动恢复；暂停或禁用节点不再吊销它的证书（fleet-3, internal/service/agent_pki_hooks.go:20-35）。
28. **N28** 节点离线超过 7 天、证书过期后免命令重新加入：Phase 4 记录节点观测到的公网地址（现在只有旧注册路径写 server_ip），Agent 本地保留加入凭据，指纹和地址都对得上就直接重发证书（internal/agentpki/agentpki.go:42-44, internal/service/node_service.go:445, docs/design/node-onboarding/README.md:191-204）。
29. **N29** 删除节点时经 stream 让 Agent 自行卸载；节点不在线时显示一条现成的卸载命令（fleet-5, internal/kernelnodeops/kinds.go:27-49, docs/design/node-onboarding/README.md:71-72）。
30. **N30** Control 自己下载 Agent 发布，并用官方密钥校验，作为默认下载源提供；删掉在 agent_install.artifact_dir 里手工放文件这一步；网页上可以直接启动 Agent 升级（fleet-1, internal/agentinstall/upgrade.go:25-35）。
31. **N31** Agent 本机配置只留凭据：是否开转发、限速网卡、插件监管密钥、证书路径，都由 Control 下发或自动探测。另外修正 WireGuard 运行目录这类跑到沙箱外的默认路径，增加 `anix-agent status`，给 `anix-agent uninstall` 加 --yes，删掉示例里已退役的公钥（agent-side-2, agent-side-5, agent-side-7, agent-side-9）。
32. **N32** 自动备份：每天和每次升级前各写一个备份包，内含数据库快照、根密钥、配置和清单；保留若干份，每晚做一次还原验证；首页显示“上次验证通过的备份”（upgrade-1, internal/service/system_service.go:514-527）。
33. **N33** 加上还原和迁机：`anix-control backup restore <备份包>`，以及安装命令 `--restore <备份包>`（目标库非空时拒绝），装新机器时直接还原，不会生成新的根密钥或管理员；后台可下载最新备份包（upgrade-4, upgrade-1, scripts/install.sh:874-888）。
34. **N34** 给 Control 加 systemd WatchdogSec，并提供一个可选的外部心跳地址：面板停止发心跳时由外部服务通知你。现在所有告警都在 Control 进程里跑，进程挂了就没人报警（scripts/install.sh:1171-1172, cmd/server/singleton_workers.go:115-119）。
35. **N35** 注册策略（开放 / 邀请 / 关闭）改为后台开关，新装默认关闭，上架第一个套餐时提示打开（internal/service/auth_registration.go:18-33, business-12, config-6）。
36. **N36** 数据库里记录最后运行它的程序版本，旧程序拒绝在新库上启动。迁移改成一份有顺序、各环境都一样的列表；CI 检查“从上一版本升级上来”和“全新安装”的表结构是否一致（cmd/server/bootstrap.go:214-254）。
37. **N37** 维护者工具：Agent 也加 prepare_release 脚本，并在 PR 阶段检查版本号；AGENT_REF 只存一处；CHANGELOG 改成每个 PR 一个片段文件；用 `make generate`/`make check` 一条命令跑完代码生成和必需检查；protobuf golden 增加正式的退役流程，5.0 删除旧 gRPC 包要靠它（maintainer-2, maintainer-3, maintainer-4, maintainer-6, maintainer-10）。
38. **N38** SDK 里改到 Agent 契约的变更归入 agent 类，在 PR 阶段就跑跨仓库 E2E；Agent 的特权集成测试也在 PR 和 dev_new 上跑，不再等到打标签才第一次跑（maintainer-11, snap-agent/.github/workflows/release.yml:233-237）。
39. **N39** Control 只发 linux/amd64 和 linux/arm64 两个二进制；删掉 deploy.sh 等旧部署脚本；Agent 仓库删掉重复的工作流，并重写含个人 Windows 路径的 AGENTS.md 和 CLAUDE.md（maintainer-15, maintainer-14）。
40. **N40** /metrics 默认要令牌或只监听本机。它现在对外公开用户数、节点数和订单数（observe-14, internal/router/router.go:63,120-121）。
41. **N41** 两个 npm audit 豁免分别在 2026-11-02 和 11-05 过期，到期后必需检查会变红。改成必需任务只审计生产依赖，开发依赖交给 nightly（maintainer-13, web/audit-allowlist.json:2-14）。
42. **N42** 5.0 之前的 UPGRADE.md 和 CHANGELOG 历史移到 docs/archive；修正文档里写错的配置键名、容器名和过期的版本号（upgrade-8, docs/DEPLOYMENT.md:487-489,611）。
