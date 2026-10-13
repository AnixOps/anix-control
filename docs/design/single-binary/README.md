# 单程序、自带证书与内嵌 SQLite（M2）

**状态：** 2026-10-13 草案，待所有者审阅。方向已由所有者选定（[运维简化路线图](../ops-simplification/README.md) 的 S1、S2、S4），本文只把它落成可实施、可拆 PR 的设计，不重新讨论方向。

**兼容性：** 无。只有一位维护者和使用者；生产面板今天没有节点（所有者口述）。旧的安装形态、配置文件和 PostgreSQL 部署都不保留迁移路径，唯一的例外是生产库的一次性导入。

**范围：** S1、S2、S4 和配置面，以及改进项 N1、N2、N3、N7、N8、N10、N32、N33、N34、N36、N40。不在本文：一键升级和回滚（S3，M4）、备份异地副本（S11，M4）、`admin reset`（N11，M3）、flux 运行时和阶段机制的删除（N5、N6，M5）。

本文引用的行号以 `go_dev` 80537a48 为准。写 `unknown` 的地方是代码和仓库回答不了的问题，需要所有者在生产环境里查（附录 A 给出只读 SQL）。

## 决定

| # | 问题 | 决定 |
|---|---|---|
| B1 | ACME 库 | `golang.org/x/crypto/acme/autocert`，不用 certmagic（2.1） |
| B2 | 挑战类型 | TLS-ALPN-01（443）在先，HTTP-01（80）兜底 |
| B3 | 证书存放 | 数据库表，私钥用根密钥封存，随备份走，还原和迁机不重新申请 |
| B4 | 节点 gRPC 端口 | 保持 50051，与 443 用同一张证书；`grpc.port` 可改 |
| B5 | 申请失败 | 用自签证书照常启动，页面顶部横幅说明原因，不发放节点接入命令，后台自动重试 |
| B6 | 反代或 CDN | 提供 `tls.mode: external` 逃生口 |
| B7 | Agent | 不改动 |
| B8 | 运行形态 | 一个 systemd 单元、一个系统用户；程序、配置、数据各在一处（3.1） |
| B9 | PostgreSQL | 服务端删除 PostgreSQL 驱动和全部 PostgreSQL 分支；只读 PostgreSQL 的一次性导入命令保留到 M5 |
| B10 | 表结构迁移 | 有序迁移列表、版本表、程序版本；旧程序拒绝在新库上启动（N36） |
| B11 | 写入竞争 | 一个写连接加读连接池；流量明细改为按小时合并；日志表有保留期 |
| B12 | 生产库 | 导入，不重建（3.5） |
| B13 | 首个管理员 | 一次性领取链接，磁盘和配置里不留明文密码（N10） |
| B14 | 备份 | `VACUUM INTO` 快照、根密钥、配置和清单打成一个包；每天一次、每次表结构迁移前一次；每晚还原验证；本地包不加密 |
| B15 | 还原 | 目标非空时拒绝；`--replace` 先把现有数据挪走，从不删除 |
| B16 | 看门狗 | `Type=notify` 加 `WatchdogSec=60`；外部心跳地址是后台设置，默认关 |
| B17 | /metrics | 只回答本机来源和带管理员 API 令牌的请求，无配置项 |
| B18 | 容器 | 不保留 Docker 镜像、Compose 和 Helm |
| B19 | 前端 | 嵌入二进制 |
| B20 | 官方包形态 | 各包的宿主代码编译进 Control（一个程序多个入口）；签过名的精简包（不含二进制）用 go:embed 嵌入 |
| B21 | 每次启动 | 校验、登记、安装或升级、启用全部内置包；任何一个包失败都不阻止 Control 启动 |
| B22 | 插件中心 | 只剩状态、重启和包配置；删除导入、路由模式、三个下发开关 |
| B23 | Agent 侧插件载荷 | 不随 M2 内置；Agent 插件下发和拓扑执行保持今天生产上的关闭状态，到 M4 随 Agent 发布清单再接上 |
| B24 | 社区版和商业版 | 删除 `app.edition` 和构建期的商业包排除；订单、支付、返佣包进入每个公开发布 |
| B25 | 配置面 | 共 9 个键，一般安装只写 `domain` 一行（第 5 节） |
| B26 | 发布产物 | Control 只发 linux/amd64 和 linux/arm64 两个二进制（N39 的 Control 部分） |

## 1. 现状（已核实）

### 1.1 进程、监听和证书

一个进程开三个监听，启动顺序在 `cmd/server/main.go` 的 `run`（main.go:352-545）：

| 监听 | 默认地址 | 来源 | TLS |
|---|---|---|---|
| gRPC（节点） | `0.0.0.0:50051`，默认关闭 | `grpc.enabled: false`、`port: 50051`（internal/config/defaults.yaml:109-112）；`startGRPCServer` 在任何工作线程之前绑定端口（main.go:497-506, 854-883） | 配了 `grpc.tls_cert_file`/`tls_key_file` 才开，否则明文（internal/grpc/server.go:140-147）；证书用 `tls.LoadX509KeyPair` 只在启动时读一次（server.go:142） |
| API | `0.0.0.0:8080` | `server.host`/`port`（defaults.yaml:6-8；main.go:1015-1039） | 永远明文（main.go:1042-1045） |
| 网页 | `0.0.0.0:3000` | `frontend.enable`/`port`（defaults.yaml:24-27；main.go:1048-1134） | `tls.enable` 且配了证书文件时 `ListenAndServeTLS`（main.go:1226-1233） |

- 网页监听把 `/api/*`、订阅路径和五个安装路径反代到 API 监听（main.go:1075-1098, 1149-1159, 1298-1352）。安装路径是 M0（#304）刚加上的，所以 N1 的“经反代拿到 HTML”已修；但两个 HTTP 监听、一层自家反代和为它写的信任代理逻辑（main.go:1161-1223）都还在。
- `/install.sh`、`/install/*`、`/metrics`、`/livez`、`/readyz`、`/health` 注册在 API 路由上（internal/router/router.go:115-121, 132-141）。
- gRPC 的 TLS 配置向客户端要证书但不强制（`tls.RequestClientCert`，server.go:81-93）；证书由 Agent 服务和拦截器对照 Agent 信任包校验。服务端每 30 分钟断开一次连接（`MaxConnectionAge`，server.go:65）。
- `tls.domain` 只出现在一行日志里（main.go:439-443）。
- 今天公网部署的证书来自 certbot：`config/deploy/grpc_tls/setup_certbot.sh` 用 Cloudflare DNS-01 申请，续期后复制到 `INSTALL_DIR/config/tls` 并重启服务（setup_certbot.sh:1-25）。安装器检查证书能否用系统根验证，不能就拒绝（scripts/install.sh:762-791），没有证书时只打印下一步（scripts/install.sh:855-867）。网页那一侧由 nginx 或 Caddy 终止 TLS（docs/DEPLOYMENT.md:441-522）。
- 告警扫描检查 Agent、转发链路、模块证书和 CA 的到期（internal/kernelalerts/scan.go:54-59），不检查面板自己的证书（N2）。

**Agent 一侧（anix-agent dev_new 95fa91b）。** Agent 用系统根验证 Control 的 gRPC 证书，没有任何信任私有 CA 的配置：`tls.Config` 只有 `ServerName`、`MinVersion` 和 `RootCAs`（api/agent/client.go:892-900），`RootCAs` 在生产里是 nil，即系统根（client.go:103-108；node/agent_control.go:199-203）。仓库里没有 `InsecureSkipVerify`。`pki/<node>/ca.pem` 是 Agent CA 信任包，只用来校验签发给自己的客户端证书，从不用来验证 Control（api/agent/pki/pki.go:156-171）。没配 `GRPCHost` 时 Agent 连 `ApiHost` 的主机名加 50051（node/agent_control.go:262-268）。证书验证失败时 Agent 不发送任何凭据，按 1 到 30 秒退避一直重试（client.go:36-38, 401-450）。

### 1.2 数据库

**打开方式。** `internal/database/database.go:25` 的 `Init` 按 `database.driver` 选驱动：`sqlite`/`sqlite3`/空串走 `github.com/glebarez/sqlite`（纯 Go，底层是 modernc.org/sqlite v1.23.1，SQLite 3.41.2：`modernc.org/sqlite@v1.23.1/lib/sqlite_linux_amd64.go:2277`），`postgres` 走 `gorm.io/driver/postgres`（database.go:34-54）。编译不需要 CGO（Makefile:38）。内置默认值是 PostgreSQL（defaults.yaml:28-39），而 systemd 安装器下载的模板是 SQLite（config/config.yaml.example:37-38；scripts/install.sh:361-383）。

**SQLite 参数。** 连接串带 `busy_timeout(5000)`、`_txlock=immediate`，文件库再加 `journal_mode(WAL)`（database.go:145-166）。没有设置 `synchronous`、`foreign_keys`，取 SQLite 默认值。连接池参数只对 PostgreSQL 生效（database.go:82-99），所以内核对 SQLite 的连接数不设上限。每个包宿主进程用 `sdk/packagestoresdk` 自己再开同一个文件，最多 4 个连接（sdk/packagestoresdk/store.go:30, 88-90）。

**SQLite 已知限制。**

- 一次只有一个写事务。驱动把所有 `FOR UPDATE` 丢掉（`glebarez/sqlite@v1.11.0/sqlite.go:115-118`），正确性靠 `_txlock=immediate` 让写事务在 BEGIN 时就排队。
- 写者不止内核一个进程：内核和每个声明 `kernel.storage.v1` 的包宿主（13 个包）都直接写同一个文件。
- 忙等超过 5 秒报 `database is locked`；只有经 `WithRetryableTransaction` 的路径会重试，最多 8 次（internal/service/transaction_retry.go:16-34）。
- GORM 没开 `SkipDefaultTransaction`，每次 Create、Update、Delete 都是一个事务。

**PostgreSQL 专有或分方言的代码（非测试）。** 全仓没有 `SKIP LOCKED`、`RETURNING`、`jsonb`、`ILIKE`、序列函数（`cmd/sqlite2postgres` 和 `scripts/staging/stagingctl` 除外）和 `LISTEN/NOTIFY`。`ON CONFLICT` 和部分索引两种库都支持（例如 internal/model/node_group.go:51、kernel.go:246；internal/service/forward_runtime_job_schema.go:83-93）。真正依赖 PostgreSQL 的是下表：

| 位置 | PostgreSQL 做法 | SQLite 时 |
|---|---|---|
| 启动锁 cmd/server/bootstrap.go:258-285 | `pg_advisory_lock` | 不加锁 |
| 节点操作账本 internal/kernelnodeops/ledger.go:112-118 | `pg_advisory_xact_lock` | 用一条空 UPDATE 取写锁 |
| 订阅变更日志 internal/subscriber/changes.go:65-69 | `pg_advisory_xact_lock` | 靠单写者 |
| 邀请码 internal/service/invite_service.go:225-230；packages/affiliate/native/invite_codes.go:219-250 | 咨询锁 | 跳过或忙重试 |
| 签名密钥库 identity/keystore/keystore.go:151-156 | 咨询锁 | 靠单写者 |
| 一致性读 internal/nodesecrets/verify.go:187-191；packages/identity-platform/native/directory.go:288-291 | `REPEATABLE READ READ ONLY` | 普通事务 |
| 统计 SQL internal/service/stats_service.go:451-462、node_traffic_series.go:217-224；packages/machine-telemetry/native/traffic.go:61-71 | `GREATEST`、`::numeric` | `CAST` |
| 包存储 internal/packagestore/postgres.go:32-204；views.go:186-191, 411-441 | 每个包一个角色 `anix_pkg_<id>` 和模式 `pkg_<id>`，按角色授权和行过滤视图 | 共用内核文件，表名加 `pkg_<id>_` 前缀，“什么也不隔离，只用于开发和测试”（packagestore.go:8-10, 277-306） |
| 包迁移 sdk/packagestoresdk/migrations.go:168-172 | 咨询锁 | 靠单写者 |
| 远程模块运行时 internal/service/plugin_runtime.go:59-61；packagestore.go:197-200 | 支持 | 拒绝 |
| 旧转发表删除 internal/forwardlegacy/drop.go:183-211, 311-312 | `lock_timeout`、`pg_dump` 证明 | 用 Control 自己的备份证明 |
| 后台备份 internal/service/system_service.go:466-474 | 拒绝（只支持 SQLite） | `VACUUM INTO`（system_service.go:514-527） |

结论：除了远程模块运行时、包存储隔离和两张行过滤视图，内核没有只在 PostgreSQL 上才能工作的功能。

**“远程身份模块只支持 PostgreSQL”的确切含义。** 包有两种运行方式。本地方式（默认）是 Control 的子进程，经 Unix socket 和 socketpair 通信（internal/pluginhost/process.go:49-150），在 SQLite 上可用。远程方式是把包作为单独的容器（`Dockerfile.module`，`docker-compose.modules.yml:39-74` 里的 `identity` 服务）跑，经 `:7443` 的 mTLS 监听登记（cmd/server/module_runtime.go:31-112）。远程方式要求 `module_runtime.enabled`，并且在两处拒绝 SQLite：设置运行时（plugin_runtime.go:59-61，“the remote runtime requires PostgreSQL”）和租用包存储（packagestore.go:197-200，远程宿主无法共用内核的 SQLite 文件）。所以受限的只是“远程”，不是身份模块本身。

**高频写入。**

| 写入者 | 频率 | 每次写什么 |
|---|---|---|
| Agent 心跳 | 默认每 20 秒（internal/grpc/agent_control_server.go:28-31），forward.v1 会话 60 秒 | `touchNode` 更新 `v2_node`/`v2_forward_node`（agent_control_server.go:752-758）；每个插件一条遥测和观测状态 upsert（internal/service/plugin_telemetry.go:175-190, plugin_observed_state.go:201-215）；传输记录每节点每分钟至多一次（internal/agenttransport/recorder.go:20, 120, 170） |
| 流量上报（Agent 流和 UniProxy） | 每个上报窗口一次，旧推送 60 秒（internal/service/server_service.go:638） | 一个事务（server_service.go:184-198, 218-244, 291-308）：每个有流量的用户一行 `v2_server_log`，更新 `v2_node`，两条 `v2_stat_server`，逐个用户更新 `v2_user` 的 u/d（internal/subscriber/traffic.go:36-82） |
| 在线记录 | 每用户每分钟至多一次，500 条一批（internal/subscriber/activity.go:15-19, 82-104） | upsert `v4_kernel_user_activity` |
| 节点日志、包上报、转发上报 | 每批或每次上报一个事务（internal/service/node_log_service.go:103-114；package_report.go:229-244；internal/kernelforward/reports.go:92-125） | 插入或 upsert |
| 审计 | 每个管理写操作一条，异步（internal/middleware/audit_log.go:200-201, 256-274） | 插入 `v2_audit_log` |
| 吊销表刷新 | 每个进程每 5 秒（internal/authn/revocation.go:237-248, 284-297） | 两条 DELETE，没有匹配行也各是一个写事务 |

控制面里没有流量的内存缓冲或合并，“spool”只指 Agent 的重发队列（internal/agentreports/agentreports.go:43-47）。`v2_server_log` 没有自动清理（docs/reference/traffic-stats-operations.md:139-140），`v2_node_log`、`v2_audit_log` 也没有；全仓没有 `VACUUM`、`wal_checkpoint` 或 `PRAGMA optimize` 任务。

**CI 里的 PostgreSQL。** 测试用 `ANIX_TEST_POSTGRES_DSN`、`POSTGRES_TEST_DSN` 或 `ANIXOPS_AGENT_E2E_POSTGRES_DSN` 打开，没有就跳过；大多数包总在 SQLite 上跑，设了 DSN 时再在 PostgreSQL 上跑一遍（例如 internal/nodesecrets/db_test.go:28）。只在 PostgreSQL 上跑的是包存储、咨询锁和统计 SQL 的测试。用到 PostgreSQL 的作业（.github/workflows/ci.yml）：

| 作业 | 行 | 为什么存在 |
|---|---|---|
| PostgreSQL Stats Regression | 1233-1265 | 统计 SQL 和咨询锁 |
| Package Storage PostgreSQL (1/4)–(4/4) | 1267-1349 | 包的角色和模式；按包 id 命名，所以同一库里只能串行（ci.yml:1267-1270；AGENTS.md） |
| SQLite to PostgreSQL Dry Run | 1351-1416 | `cmd/sqlite2postgres` |
| PostgreSQL Restore Rehearsal | 1418-1465 | `scripts/postgres_restore_rehearsal.sh` |
| Docker Build Smoke、Kubernetes Smoke、Staging Rehearsal Smoke | 679-809、882-1007、467-517 | 容器和 Compose/Helm 部署，各带一个 PostgreSQL |
| Cross-Repo E2E | 1947-2107 | 可选 PostgreSQL；不设 DSN 时用 SQLite（internal/tests/agente2e/suite_test.go:127-135） |

Smoke Tests 和 E2E Tests 跑在 SQLite 上（internal/tests/smoke/smoke_test.go:47），没有任何作业在容器里运行 Control 来测 Agent：两个跨仓库 E2E 都是 `go build` 出二进制或在进程内启动 Control（ci.yml:1944-1945, 2045-2048）。

### 1.3 表结构和版本

- 没有版本化迁移。启动时对 65 个模型（cmd/server/bootstrap.go:30-112）在开发和测试环境做完整 `AutoMigrate`；生产环境空库建全表，非空库只建缺的表，“已有的表不改”（bootstrap.go:214-253）。另有十几个每次启动都跑的 `Ensure*` 步骤，其中几处手写加列和换索引（bootstrap.go:129-194；internal/service/kernel_service.go:136-149）。
- 结果是同一版本的生产库和新装库表结构可以不同（生产库只会缺列，不会报错）。
- 库里不记录程序版本，也没有拒绝旧程序的检查。唯一存了 Control 版本的是旧转发归档的 JSON（internal/forwardlegacy/archive.go:34, 98）。
- 生产基线（2026-09-29 的 `pg_dump`，PostgreSQL 15.18）：约 150 MB，18 个用户、14 个节点、23 个 WireGuard 对端，没有订单、支付和转发规则（docs/architecture/release-line-status.md:88-101）。所有者 2026-10-12 说面板上已没有节点；今天的准确数字是 `unknown`，见附录 A。

### 1.4 插件包

- **格式。** `packages/shared/build_package.py` 给每个包产出 `.anxp`（确定性 tar，超过 64 MiB 才压缩）、规范化 manifest、manifest 的 Ed25519 签名、公钥和 SBOM（build_package.py:296-324, 1149-1159）。签名用 `openssl pkeyutl -rawin` 签 manifest 原始字节（build_package.py:841-864），不是 cosign。
- **内容。** `.anxp` 里有两个架构的 Control 宿主二进制 `bin/control-host-linux-{amd64,arm64}`、路由表、迁移索引、可选 WebUI；Agent 侧的包还有 `agent/linux-<arch>/plugin`，gost-mesh 还有 gost 运行时（build_package.py:577-665）。正式发布要求两个架构都在（build_package.py:1430-1431）。
- **数量和大小。** 18 个包（build_package.py:54-73）。每个包都带 Go 宿主进程；没有 `control/` 目录的包用通用宿主 `packages/shared/controlhost`。本次实测（linux/amd64，`-s -w`）：Control 本体 58.7 MB；通用宿主 13.1 MB；forward 13.7 MB；用到包存储的宿主 30.1–31.0 MB（knowledge 30.1、ticket 30.1、plan 30.3、order 30.4、subscription 30.4、payment 30.8、identity-platform 31.0）；machine-telemetry 的 Agent 插件 16.3 MB（未 strip）。一个 `.anxp` 约 58–60 MB，全套每个架构约 0.5 GB（docs/architecture/container-deployment.md:16；docs/UPGRADE.md:262 称压缩包“几百 MB”）。
- **存放。** 包字节存在数据库 `v3_kernel_plugin_artifact.Data`（internal/model/kernel.go:116-128），启动宿主时再落到磁盘（internal/service/kernel_service.go:1694-1797）。
- **校验。** 发布签名对照 `plugins.official_public_key`（defaults.yaml:66；kernel_service.go:549-574, 597-606）；启动时激活这把公钥、作废其他根（cmd/server/main.go:845-852；bootstrap.go:157-159）。宿主启动前重新校验每个文件的摘要（internal/pluginhost/artifact.go:64-148）。
- **安装。** 启动时只自动导入 identity-platform 一个包，来源目录是 `plugins.identity_bootstrap_package_dir`（internal/service/plugin_bootstrap.go:33-105；bootstrap.go:160-162）；它从不升级已有安装（plugin_bootstrap.go:38-41, 301-404），失败则 Control 拒绝启动（main.go:458-460）。其余 17 个包要在插件中心导入（web/src/views/admin/Plugins.vue:566-592；`POST /api/v3/plugin-releases` 和 `/:id/artifact`，router.go:677-680）。宿主重启时按 `plugin_id` 排序排队启用，不按依赖（internal/plugincontrol/operation_worker.go:127-171）；目前 18 个包的依赖都为空。
- **社区版和商业版。** `config/editions.json:4` 把 affiliate、order、payment 列为商业包；发布作业不带 `--edition`，于是公开发布不含它们（build_package.py:87-98；ci.yml:1713-1738）；商业包只由手动的 `.github/workflows/commercial-packages.yml` 构建。运行时 `app.edition` 默认 `community`，把商业路由答成 404（defaults.yaml:62-64；router.go:47-49）。identity-platform 不在商业名单里。
- **路由模式。** `package_routes.default_mode`（默认 `rehearsed`，defaults.yaml:133-139；config.go:220-256；main.go:404）决定没有存储模式的路由走 native 还是 legacy；每条路由还可以存 `legacy`、`shadow`、`native`（internal/service/package_route_modes.go:19-112），由 `/api/v4/kernel/route-modes`（router.go:790-795）、`anix-control routes`（cmd/server/routes_command.go:73）和路由模式页（web/src/views/admin/RouteModes.vue）修改。
- **三个下发开关。** 都在 `plugins` 下（config.go:388-404）：

| 键 | 内置默认 | 生产模板 | 管什么 |
|---|---|---|---|
| `plugins.control_execution_enabled` | true（defaults.yaml:67） | false（config/config.prod.yaml:117），安装器强制改 true（scripts/install.sh:492, 530-531） | 包宿主进程和生命周期（main.go:570, 623-663）；关闭时 `/api/v2` 答 502（internal/compat/v2/gateway.go:73-75） |
| `plugins.dispatch_enabled` | false（defaults.yaml:76） | false（config.prod.yaml:131） | 向 Agent 下发插件操作（main.go:575-581, 728-739） |
| `plugins.topology_execution_enabled` | false（defaults.yaml:78） | false（config.prod.yaml:135） | 部署编排执行器，要求上一个开关也开（main.go:583-586, 740-751） |

### 1.5 安装器、配置、备份和监控

- **配置。** 有配置文件时整个替换内置默认值，不合并（internal/config/config.go:754-766）；未知 YAML 键静默忽略，未知环境变量只记日志（config.go:764-774）；没有废弃键的处理。共 20 节 151 个叶子键（docs/reference/environment-variables.md:30-178 有 149 行可由环境变量设置）。无人读取的键：`cache.*`（5 个，启动时写死内存缓存，main.go:482）、`app.name`、`app.version`、`app.traffic_log_enable`、`log.output`/`file_path`/`max_size`/`max_backups`/`max_age`、`forward_runtime.clean_agent.public_url` 和 `token_expire_seconds`；`tls.domain` 只进日志。`forward_runtime.iptables_ansible` 不是键，只是模板里会被忽略的一段（config/config.yaml.example:197-208）。`app.api_token`、`grpc.api_token`、`package_routes.default_mode` 仍有读者（internal/middleware/middleware.go:240；internal/grpc/server.go:226-227；main.go:404）。
- **安装器。** `scripts/install.sh`（1405 行）有 install、update、rollback、preflight、migrate、enable-agents（install.sh:1318-1401），没有交互提问，不问域名；默认目录 `/opt/anixops/control`、用户 `anixops`（install.sh:20-21, 276-280）。首个管理员的明文密码写进 `config.yaml` 的 `admin.password` 和 `INSTALL_DIR/.bootstrap-admin-password`（install.sh:426-427, 441-445），之后没有任何步骤删除它。升级失败只还原程序、前端和配置，不还原数据库（install.sh:1186-1201）。旧布局识别和 `bin/v2board` 链接在 install.sh:154-171, 1141-1142。
- **systemd 单元。** `Type=simple`、`Restart=on-failure`，没有 `WatchdogSec`、`ProtectSystem`、`StateDirectory`（install.sh:1152-1184）。全仓没有 sd_notify，也没有外部心跳；告警在 Control 进程内的单例工作线程里跑（cmd/server/singleton_workers.go:115-119），进程挂了就没人报警。
- **首个管理员。** `service.InitAdmin` 在没有管理员时用 `admin.password` 建一个，没配密码就拒绝启动（internal/service/init_admin.go:20, 24-95）。
- **根密钥。** `module_runtime.ca_kek`（config.go:466）封存模块 CA 私钥、转发链路 CA 私钥、DNS 凭据和节点组加入令牌（internal/modulepki/authority.go:492-506；internal/agentpki/link.go:52-54；internal/kernelforward/dns.go:155-185；internal/nodegroup/token.go:20-35）。Go 代码从不自动生成它，只由安装器、Compose 脚本或 Helm 生成（install.sh:707-734）；启动时不检查它能否解开已有的 CA（authority.go:133-152, 515-517）。`identity.kek` 只交给 identity-platform 宿主（main.go:920-928），没有它就没有签名密钥（packages/identity-platform/control/identity.go:92-93）；安装器不生成它。`jwt.secret` 由安装器生成（install.sh:372）。
- **备份。** 后台备份用 `VACUUM INTO`，只支持 SQLite（system_service.go:466-474, 514-527）；备份设置页的定时、保留和 S3 没有任何代码执行（internal/model/system.go:68-86；`CleanupOldBackups` 只有测试调用）。还原是在运行中的进程里关掉数据库再覆盖文件（system_service.go:319-329）。
- **/metrics。** 没有任何认证（router.go:120-121），每次抓取都数用户、节点和订单（internal/handler/metrics.go:67-74）。

## 2. S1：自带证书

### 2.1 ACME 库

| | `x/crypto/acme/autocert` | certmagic |
|---|---|---|
| 依赖 | `golang.org/x/crypto` v0.57.0 已是直接依赖（go.mod:21），模块缓存里自带 `acme/autocert`；新增模块 0 个 | 不在 Control 的 go.mod 里。v0.23.0 自己依赖 acmez/v3、zerossl、libdns、miekg/dns、zap、zap/exp、blake3、cpuid（其 go.mod） |
| 许可 | BSD-3-Clause | Apache-2.0 |
| 功能 | TLS-ALPN-01、HTTP-01、自动续期、可插拔缓存接口 | 另有 DNS-01、ARI、分布式锁、按需签发 |
| CI 和体积 | 不变 | go.sum 和二进制都会增加 |

**决定：autocert。** Control 只有一个实例、一个域名，用不上分布式锁和按需签发；DNS-01 需要域名服务商的令牌，与“只问一个域名”相悖。certmagic 唯一值得要的是 ARI（由 CA 指示续期时间），用固定的续期提前量代替即可。Agent 仓库因为 sing-box 的 ACME 构建标签间接带着 certmagic（anix-agent go.mod:96），与 Control 无关。

autocert 的几个行为要在实现里绕开或利用：

- 首次握手会同步申请证书，最长阻塞 5 分钟（autocert.go:274）。所以 Control 在启动后用后台 goroutine 主动申请，握手回调不等待（2.4）。
- 不能给 IP 地址申请（autocert.go:255 要求名字里有点），没有 SNI 的握手直接报错（autocert.go:250-253）。这正好符合“必须有域名”。
- 默认在剩余三分之一寿命时续期，最多提前 30 天，加不超过 1 小时的抖动（renewal.go:142-156）。证书寿命变短时这个规则仍然成立，不需要配置。
- `Email` 是可选的（autocert.go:152-157）。面板自己的到期告警（2.3）代替 CA 的邮件提醒，所以安装时不问邮箱。

### 2.2 监听和端口

| 监听 | `tls.mode: acme`（默认） | `tls.mode: external` |
|---|---|---|
| HTTPS | `:443`，网页、API、订阅、`/install.sh`、`/install/*`、健康检查都在这一个 gin 引擎上（N1） | 无 |
| HTTP | `:80`，只回答 HTTP-01 挑战，其余请求 301 到 https（autocert 默认处理，autocert.go:388-394, 418） | `listen`（默认 `127.0.0.1:8080`）明文，前面是运维自己的反代 |
| gRPC | `:50051`，与 443 同一张证书 | `:50051`，证书来自 `tls.cert_file`/`tls.key_file` |

- 删除 3000 端口的网页监听和它的反代代码（main.go:1048-1134, 1136-1233, 1298-1352）以及 `frontend.*`。前端静态文件嵌入二进制（B19），SPA 回退只对非 `/api`、非安装路径生效。
- `:80` 和 `:443` 以双栈方式监听（`net.Listen("tcp", ":443")`），IPv4 和 IPv6 都在。
- 非 root 绑定 80、443 用 systemd 的 `AmbientCapabilities=CAP_NET_BIND_SERVICE`（3.1）。
- gRPC 端口保持 50051（B4）：它是 Control 现在的默认值（internal/grpc/server.go:61；internal/agentinstall/offline.go:70-77），也是 Agent 没配 `GRPCHost` 时的回退（node/agent_control.go:262-268）。改端口不带来任何好处，却要同时改两个仓库和防火墙。可由 `grpc.port` 修改，`/install/agent.env` 发给节点的 `grpc_target` 就是 `<domain>:<grpc.port>`。
- 在 acme 模式下 Control 自己就是边缘，`server.trusted_proxies` 固定为空（不信任任何转发头）；external 模式固定为本机回环（今天的默认值，internal/requestorigin/requestorigin.go:28）。两者都不再是配置项。
- 不发 HSTS：自签兜底时 HSTS 会把浏览器锁在外面。

### 2.3 申请、续期和热加载

新包 `internal/panelcert` 持有证书来源，对外只暴露 `GetCertificate(*tls.ClientHelloInfo)` 和当前状态（`acme`、`self_signed`、`external`，证书的 NotAfter，最后一次错误）。

- **申请。** `autocert.Manager{Prompt: AcceptTOS, HostPolicy: HostWhitelist(domain), Cache: dbCache, RenewBefore: 0}`，并调用一次 `HTTPHandler(nil)` 打开 HTTP-01。启动后立刻在后台对 `domain` 调一次 `GetCertificate`（带 ECDSA 能力的 ClientHello，拿到 P-256 证书），不阻塞监听。
- **挑战顺序。** autocert 固定先试 TLS-ALPN-01，开了 HTTP-01 再试它（autocert.go:773-775）。这正是要的顺序：TLS-ALPN-01 只需要本来就归 Control 的 443，在同一个 TLS 监听里应答（`acme-tls/1` 握手，autocert.go:228-235, 320-321），不经过任何路由；某些网络会拦截或改写 443 上的 TLS 时，HTTP-01 在 80 上兜底。
- **存放（B3）。** 实现 `autocert.Cache` 的 Get/Put/Delete，存到新表 `v5_acme_cache(key TEXT PRIMARY KEY, data BLOB, updated_at)`；含私钥的条目（账户密钥、证书私钥）用根密钥以 AES-256-GCM 封存，附加数据为 `anixops acme cache v1:<key>`。不用 autocert 自带的 `DirCache`（目录 0700、文件 0600，cache.go:38, 65-67），理由是数据库随备份走：还原或迁机不会重新申请，不消耗 CA 的重复证书配额。
- **热加载（N2）。** 两个 TLS 监听都用 `GetCertificate`，每次握手取当前证书，续期后新连接立刻用上新证书，不重启。
  - HTTPS：`tls.Config{GetCertificate: panelcert.GetCertificate, NextProtos: ["h2", "http/1.1", "acme-tls/1"]}`。
  - gRPC：`tls.Config{GetCertificate: panelcert.GetCertificate, ClientAuth: tls.RequestClientCert}`，`credentials.NewTLS` 会补上 `h2`。也就是把 server.go:87-93 的 `Certificates: []tls.Certificate{cert}` 换成 `GetCertificate`，`ClientAuth` 的规则不变：仍然向 Agent 要客户端证书但不在握手里强制，校验仍在 Agent 服务和拦截器里做。
  - 已建立的 Agent 流继续用旧证书，最多 30 分钟后因 `MaxConnectionAge` 重连拿到新证书（server.go:65）。
- **到期告警（N2）。** `kernelalerts` 增加两种：`panel_certificate_expiring`（窗口沿用 `leafWindow`：寿命的六分之一与 `alerts.leaf_expiry_days` 取小，scan.go 现有规则）和 `panel_certificate_fallback`（正在用自签证书，严重级别为 critical）。扫描读取 `panelcert` 的状态，不读文件。
- **测试用的 CA。** 内部开关 `ANIX_CONTROL_ACME_DIRECTORY_URL`（只读环境变量，不进配置文件和操作员文档）指向别的 ACME 目录。CI 的集成测试用固定版本、校验和锁定的 Pebble 二进制（下载方式同 ci.yml:1643-1665 的 gost），所有者在真实 VPS 上先用 Let's Encrypt staging 目录验收，再切到正式目录。

**配额。** Let's Encrypt 公布的限额（以其当前文档为准）里与此相关的是：同一组名字每周 5 张重复证书、每个名字每小时 5 次验证失败、每个注册域每周 50 张证书。设计上的应对：证书和账户随数据库备份保存（B3），重装和迁机先还原再启动；失败重试按 1、5、15 分钟，然后每小时，再后每 6 小时，同一小时内最多 3 次；手动“重新申请”（后台按钮或 `anix-control tls retry`）也受同一限额保护。

### 2.4 失败兜底

ACME 申请失败（DNS 未生效、80 和 443 不通、域名套了 CDN、CA 不可用）时：

1. Control 照常启动。`panelcert` 首次启动时生成一张自签证书（ECDSA P-256，主题和 SAN 为 `domain`，并加上本机的公网 IP，有效期 30 天，到期前自动换新），同样存在 `v5_acme_cache` 里。没有可用的 ACME 证书时 `GetCertificate` 立即返回这张，从不阻塞握手；ALPN 为 `acme-tls/1` 的握手始终交给 autocert。
2. 网页顶部显示红色横幅：“证书申请失败，正在使用临时证书，节点暂时无法接入”。原因（DNS 解析结果、预检结论、CA 的错误）只对管理员显示；公开接口 `GET /api/v5/panel/status` 只返回状态值，供横幅和安装器读取。
3. 节点接入命令不发放。M1 的接入就绪检查（N23）把“面板证书是公共 CA 签发”列为一项，不通过就显示修复办法。
4. 已接入的节点照样被拒（Agent 只认系统根），按 1 到 30 秒退避重试（anix-agent api/agent/client.go:36-38, 401-450）；证书一旦签下来，下一次握手就成功，不需要在节点上做任何事。
5. 后台按 2.3 的节奏重试；每次失败更新横幅里的原因。

新装的面板因此永远能打开：浏览器会提示证书不受信任，但领取链接（3.7）和设置都能用。域名尚未解析时，安装器额外打印 `https://<公网 IP>/claim#…` 作为后备。

### 2.5 `tls.mode: external`

成本低，所以提供：一个模式值、三个只在此模式生效的键（`listen`、`tls.cert_file`/`tls.key_file`、`grpc.public_name`）和一个按文件热加载的证书来源。

- 适用：80 和 443 被这台机器上的其他网站占用；或域名必须套 CDN 代理。
- HTTP 明文监听 `listen`（默认 `127.0.0.1:8080`），运维自己的反代终止 TLS 并把整个域名转发过来。Control 信任回环来源的转发头。
- gRPC 不能经 CDN 或普通 HTTP 反代转发：TLS 被代理终止后客户端证书就丢了，Agent 的 mTLS 无法成立。所以 gRPC 仍由 Control 直接监听，证书来自 `tls.cert_file`/`tls.key_file`（必须是公共 CA 签发、覆盖节点连接的名字）。`panelcert` 每 60 秒检查两个文件的修改时间和大小，变了就重新加载，加载失败保留旧证书并告警（N2）。
- 域名套了 CDN 时，节点必须连一个不经代理的名字：`grpc.public_name` 设成它（默认等于 `domain`）。
- 此模式下 Control 不碰 ACME，横幅和告警照常工作（证书来自文件）。
- 开发实例（`make run`）也用此模式：`listen: 127.0.0.1:19080`，gRPC 只绑回环时允许不配证书、明文，与今天 server.go:140-147 的行为一致。

### 2.6 安装和预检

安装器只问一个问题：域名。

```sh
curl -fsSL https://raw.githubusercontent.com/AnixOps/anix-control/<tag>/install.sh | sudo bash -s -- --version <tag> --domain panel.example.com
```

不带 `--domain` 时交互式询问一次。`--version` 仍然必填，安装器从不读取 latest（AGENTS.md 的发布规则不变）。

**端口预检（安装器，拒绝）。** `ss -Hltnp` 查 80 和 443 的占用者。被其他程序占用时停止安装并说明：停掉它，或改用 `--external`（写入 `tls.mode: external`）。防火墙无法从机器内部判断，交给下面的自探测。

**DNS 预检（Control，只提示，不阻止申请）。** `anix-control doctor` 和每次申请前：

1. 解析 `domain` 的 A 和 AAAA 记录。没有记录：原因是“DNS 未解析”。
2. 对每个解析出的地址，带 `Host: domain` 请求 `http://<地址>/.well-known/anix-control/probe`，Control 在 80 上用一个随机数应答。拿不到自己的随机数说明该地址不是本机：要么指向别处，要么被 CDN 代理（应答头里常有 CDN 的 `Server`），结论写进原因。
3. 有 AAAA 记录时要求它也通过探测：ACME CA 会经 IPv6 访问（Let's Encrypt 优先用 IPv6），指向别处的 AAAA 是最常见的失败原因之一。
4. 有的云主机不支持从本机经公网地址绕回（hairpin），探测会超时；这时只给警告，照常申请。

安装结束时安装器读 `/api/v5/panel/status`，打印证书状态和领取链接（3.7）。

### 2.7 Agent 一侧

不需要改动。Agent 已经按系统根验证 Control 的证书（1.1），ACME 签发的证书正是它要的；gRPC 端口和名字不变。证书续期时 Agent 在下一次重连握手时自然拿到新证书。自签兜底期间 Agent 拒绝连接、不泄露凭据，这是期望的行为。

## 3. S2：单程序与 SQLite

### 3.1 运行形态

| 项 | 值 |
|---|---|
| 程序 | `/usr/local/bin/anix-control`（root:root 0755），一个文件，内含前端、内置包和 Agent 安装脚本及其签名 |
| 上一版程序 | `/usr/local/lib/anix-control/previous/anix-control`，安装器升级时保留，供回滚 |
| 配置 | `/etc/anix-control/config.yaml`（root:anix-control 0640），只写覆盖值，通常只有 `domain` 一行 |
| 数据 | `/var/lib/anix-control/`（anix-control 0700） |
| 用户 | 系统用户 `anix-control`，无 shell |
| 单元 | `anix-control.service` |
| 日志 | journald |

数据目录：

| 路径 | 内容 | 权限 |
|---|---|---|
| `control.db`、`-wal`、`-shm` | 唯一的数据库（含包的表、ACME 缓存） | 0600 |
| `root.key` | 根密钥，32 字节（N8） | 0600 |
| `backups/` | 备份包（3.8） | 0700，文件 0600 |
| `tmp/` | 备份验证、导入的中间文件 | 0700 |
| `control.lock` | 运行中的进程持有的 flock，还原和导入据此判断服务是否在跑 | 0600 |

包宿主的 socket 放在 `RuntimeDirectory`（`/run/anix-control`），重启即清空。

systemd 单元要点：

```ini
[Service]
Type=notify
NotifyAccess=main
ExecStart=/usr/local/bin/anix-control serve
User=anix-control
Group=anix-control
StateDirectory=anix-control
StateDirectoryMode=0700
ConfigurationDirectory=anix-control
RuntimeDirectory=anix-control
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
WatchdogSec=60
Restart=always
RestartSec=5
TimeoutStopSec=90
LimitNOFILE=65535
```

`TimeoutStopSec=90` 沿用今天容器的有序关闭时长（docs/architecture/container-deployment.md:18）。包宿主子进程启动时清空 ambient 能力，绑定 80 和 443 的能力只留在主进程。业务日边界（流量重置）用进程所在主机的时区；旧面板的 PostgreSQL 会话时区默认 `Asia/Shanghai`（defaults.yaml:35），迁机前要把新主机设成同一时区（3.5 的步骤里有）。

### 3.2 只留 SQLite 的代码改动

| 删除 | 位置 |
|---|---|
| PostgreSQL 驱动和 DSN | internal/database/database.go:14, 49-54, 82-99；postgres_dsn.go；go.mod 的 `gorm.io/driver/postgres`（服务端不再链接） |
| `database.*` 配置 | config.go:647-665；数据库固定为 `<data_dir>/control.db` |
| 全部 `db.Name() == "postgres"` 分支 | 1.2 表中各处，只留 SQLite 一侧 |
| 包存储的 PostgreSQL 实现 | internal/packagestore/postgres.go，views.go 的行过滤视图；`packagestore.go` 只留共享文件加前缀，注释里“只用于开发和测试”改为正式说明 |
| SDK 的 PostgreSQL | sdk/packagestoresdk 的驱动分支和 sdk/go.mod 的 pgx、gorm postgres |
| 远程模块运行时 | `module_runtime.*`（ca_kek 除外）、`:7443` 监听（cmd/server/module_runtime.go）、`SetPluginRuntime` 的远程分支、`/api/v4/kernel/modules/runtimes`（router.go:772-773）、`Dockerfile.module`、`docker-compose.modules.yml`。内置 CA（modulepki）保留，它签发 Agent 证书 |
| sqlite2postgres | cmd/sqlite2postgres、scripts/migrate_latest_sqlite_to_postgres.sh、docs/reference/sqlite-to-postgres-migration.md |
| PostgreSQL 测试 | 只在 PostgreSQL 上跑的测试文件（`*_postgres_test.go`、`TestPostgres*`）和 `forEachDatabase` 的 PostgreSQL 一支 |

`github.com/jackc/pgx/v5` 只留给一次性导入命令（3.5），M5 随它一起删除。包存储的隔离从此只靠约定（前缀和内核 API 视图），与今天 SQLite 部署相同；所有包都是官方内置的（第 4 节），不再有不受信任的包代码需要隔离。

### 3.3 迁移和版本（N36）

- **迁移列表。** `internal/database/migrations/` 下按编号排列的迁移，嵌入二进制：`NNNN_<name>.sql`，需要转换数据时用同号的 Go 函数。每个迁移一个事务。
- **基线。** `0001_baseline.sql` 等于今天一次全新安装建出的表、索引和视图（65 个模型加 `KernelModels` 加各 `Ensure*` 的结果，包括 flux 表；它们由 M5 的清理迁移删除）。由脚本在空库上跑一遍今天的 `bootstrapDatabase` 后从 `sqlite_master` 导出，入库后不再手改。
- **版本表。** `v5_schema_migration(version INTEGER PRIMARY KEY, name TEXT, checksum TEXT, applied_at, program_version TEXT)`；`v5_control_meta(key TEXT PRIMARY KEY, value TEXT, updated_at)` 记录 `program_version`（每次成功启动写入）和 `program_version_min`（能打开此库的最低程序版本，由迁移声明）。
- **启动顺序。** 打开库 → 读版本表 → 库里有本程序不认识的迁移编号，或 `program_version_min` 高于本程序：拒绝启动，日志写明“此库由 vX 写入，请安装 vX 或更新版本，或还原备份” → 有待执行的迁移：先做一次备份（3.8，类型 `pre-migration`），再依次执行 → 校验已执行迁移的 checksum 未变 → 写 `program_version`。
- `AutoMigrate`、生产和开发两套建表逻辑、每次启动的 `Ensure*` 加列步骤全部删除（bootstrap.go:214-253, 129-194）。模型仍用于查询。
- **CI 检查。** 一个测试在空库上跑全部迁移，再对模型做一次 `AutoMigrate` 的 dry-run，二者必须一致（防止改了模型忘写迁移）；另一个测试从上一个发布的基线库（入库的测试夹具）升级上来，与全新安装的 `sqlite_master` 比较，必须一致。
- 包自己的迁移（目前只有 identity-platform 的四个 SQL，packages/identity-platform/migrations/）不变，仍由 packagestoresdk 在宿主里执行。
- **实现（P2）。** 见 [database-migrations.md](../../reference/database-migrations.md)。与上文的差别：基线按方言各一个文件（`0001_baseline.sqlite.sql`、`0001_baseline.postgres.sql`，P4 删除后者），内容是迁移前的启动在空库上实际执行的建表和建索引语句，录一次后冻结，而不是从 `sqlite_master` 导出；`kapi_*` 视图不在基线里，仍由每次启动的 `EnsureKernelAPIViews` 创建（部分视图要等节点密钥拆分完成）；最低程序版本的键名是 `min_program_version`；迁移前的自动备份属于 P10。已有库（4.x 和迁移前的 `go_dev` 建的库）第一次启动时被接管：按基线补齐缺少的表、列和索引，只增不改，校验齐全后记为已执行 `0001_baseline`。CI 检查是：全新迁移与测试里保留的旧启动路径逐项一致（防止改了模型忘写迁移），从每个迁移和旧启动的结构升级上来与全新一致，以及 4.0.0、4.1.0、4.2.0 实际建出的库都能接管。

### 3.4 SQLite 参数和写入方案

**连接参数。**

| 项 | 值 | 理由 |
|---|---|---|
| `journal_mode` | WAL | 已有 |
| `busy_timeout` | 10000 | 现为 5000；跨进程写者（包宿主）在峰值时需要更长的等待 |
| `_txlock` | immediate | 已有；写事务在 BEGIN 时排队，读后写不会中途失败 |
| `synchronous` | NORMAL | WAL 下断电可能丢最后几个事务，但不会损坏库；每次提交少一次 fsync |
| `foreign_keys` | 不开 | 现有模型和数据从未在外键约束下运行过，打开会让旧数据导入失败 |
| `auto_vacuum` | INCREMENTAL | 只对新建的库生效（导入的库也是新建的）；清理任务后执行 `PRAGMA incremental_vacuum` |
| 内核写连接 | 1 | 内核内部的写事务在 Go 里排队，不在 SQLite 里忙等 |
| 内核读连接 | 4 | WAL 下读不阻塞写 |

GORM 用两个句柄实现读写分离：写句柄 `MaxOpenConns=1`，读句柄 `MaxOpenConns=4` 且 `query_only`。用 `gorm.io/plugin/dbresolver`（新依赖，与现有的 `gorm.io/gorm` v1.25.12 同一作者）把查询路由到读句柄，事务和写语句路由到写句柄。事务里的读也走写句柄，所以“先读后写”的逻辑不受影响。包宿主保持 `MaxOpenConns=4`（store.go:30）。

**高频写入。**

| 写入者 | 改动 |
|---|---|
| `v2_server_log` | 今天每次上报每个用户一行。改为按 (用户, 节点, 节点类型, 倍率, 小时) 合并：`log_at` 取整点，`ON CONFLICT DO UPDATE SET u = u + excluded.u, d = d + excluded.d`，加唯一索引。最细的读者按小时分组（internal/service/node_traffic_series.go:163-184），粒度够用。上报 60 秒一次时行数降为约六十分之一 |
| 流量事务 | 保持一个上报一个事务；逐用户 UPDATE 改为一条 `UPDATE v2_user SET u = u + CASE id …` 批量语句，减少持锁时间 |
| 心跳 | `touchNode` 只在距上次写入超过 60 秒或状态变化时写库，其余心跳只更新内存（在线判断的 5 分钟窗口不受影响） |
| 吊销表刷新 | 两条 DELETE 从每 5 秒改为每 5 分钟；读取仍每 5 秒 |
| 保留期 | 新的单例清理任务每小时运行，分批删除（每批 5000 行，批间让出写锁）：`v2_server_log` 400 天、`v2_node_log` 30 天、`v2_audit_log` 400 天；之后 `PRAGMA incremental_vacuum` 和 `PRAGMA optimize`。数值是内置常量 |
| 检查点 | WAL 自动检查点之外，每天备份后执行一次 `PRAGMA wal_checkpoint(TRUNCATE)` |

**基准。** 同一个 PR 加 `BenchmarkTrafficReportSQLite`：N 个节点、每次上报 K 个用户、心跳 20 秒，在 CI 机器上测每秒提交数和 p99 写等待，结果写进 PR 描述和附录 B。

### 3.5 生产库迁移

**选择导入，不重建。** 重建意味着所有用户的 UUID 和订阅令牌（v2_user.uuid、token，internal/model/user.go:21-22）变化，每个客户的订阅链接失效；密码哈希、余额、工单、订单历史、流量历史、审计、优惠券、邀请码、知识库全部丢失。导入保留全部这些，代价是一个一次性命令。

**命令。** `anix-control import-postgres`，代码在 `cmd/server/import_postgres_command.go` 和 `internal/pgimport/`，随 M2 的发布一起发出，M5 删除。它与目标程序是同一个二进制，所以建出的表结构必定与该版本一致。

```sh
sudo anix-control import-postgres \
  --dsn 'postgres://reader@127.0.0.1:5432/anix_control?sslmode=disable' \
  --root-key-file /opt/anixops/control/config/secrets/module_ca_kek \
  --old-config /opt/anixops/control/config/config.yaml \
  --dry-run --report /var/lib/anix-control/tmp/import-report.json
```

以 root 运行（要读旧安装里只有 root 可读的 `module_ca_kek`），写出的文件属主设为 `anix-control`。

**怎么读。** 一个 `BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY` 事务，会话设 `default_transaction_read_only = on`，全程一个快照，从不写 PostgreSQL；只需要一个只读角色。读 `information_schema` 列出 `public` 和 `pkg_*` 模式的表、各表行数、序列的 `last_value`。

**怎么写。**

1. 目标数据目录必须为空（与还原相同的规则；dry-run 写到 `--work-dir`，默认一个新的临时目录，不要求数据目录为空）。在工作目录里新建 `control.db.importing`，执行全部迁移（3.3），再按 identity-platform 包的迁移索引建出 `pkg_identity_platform_*` 表。
2. 表映射：`public.<t>` → `<t>`；`pkg_<id>.<t>` → `pkg_<id>_<t>`。
3. 逐表按主键顺序复制，每 1000 行一个 SQLite 事务。按列名对应，按目标列的类型转换：布尔 → 0/1，`timestamptz` → 驱动写 `time.Time` 时的格式（UTC），`bytea` → BLOB，其余原样。
4. 不复制的内容：`v4_kernel_package_storage`（PostgreSQL 角色台账，SQLite 上会重建）；`v3_kernel_plugin_artifact` 的包字节（第 4 节起包内置在程序里；发布和安装记录照常复制）；`pkg_identity_platform.signing_key`（由新程序用派生的 KEK 重新生成，代价是所有人重新登录一次，JWT 换密钥同样如此，见第 5 节）。
5. 源库里有、目标结构里没有的表或列：表为空或列全为 NULL 时跳过并写进报告；有数据时拒绝，除非逐个用 `--allow-drop <表>` 或 `--allow-drop <表.列>` 明确放弃。目标有、源库没有的列取默认值（生产库“只建缺的表、不改已有表”，缺列是预期内的）。
6. 序列：目标表是 `INTEGER PRIMARY KEY AUTOINCREMENT`（glebarez/sqlite@v1.11.0/sqlite.go:206），把 `sqlite_sequence.seq` 设为 PostgreSQL 序列 `last_value` 与 `max(id)` 的较大者，删除过的编号不会复用。
7. 节点密钥拆分表（`v4_kernel_node_credential`、`v4_kernel_protocol_secret`、`v4_kernel_node_secret_split`）和身份切换状态（`v4_kernel_identity_authority`）原样复制。封存值不变，因为根密钥文件就是旧的 `module_ca_kek`；导入前用它解开一次旧库里的 CA 私钥，解不开就拒绝（N8 的启动检查提前做）。
8. 从旧 `config.yaml` 推出覆盖值：`domain` 取 `agent_install.public_url` 的主机，没有时取 `tls.domain`；非默认的 `app.subscribe_path` 和 `grpc.port` 原样带过去；其余全部丢弃并列在报告里。写入 `/etc/anix-control/config.yaml`：文件已由安装器写好时只补上缺的键，`domain` 不一致则拒绝。

**对账。** 每张表比较行数，再比较校验和：按主键顺序把每行编码成规范文本（整数十进制、布尔 0/1、时间 RFC 3339 UTC、字节十六进制、NULL 用固定标记），对整表算 SHA-256。PostgreSQL 一侧对读出的值算，SQLite 一侧在提交后重新读出再算，两边必须相同。另外抽查：全部用户的 `token`、`uuid`、密码哈希逐行相等。最后 `PRAGMA integrity_check` 必须为 `ok`。任何一项不符就删掉中间文件、以非零状态退出。全部通过后改名为 `control.db`，写入 `root.key` 和配置（第 8 步）。

**dry-run。** 做完以上全部步骤，包括对账，然后删除中间文件，只留报告（每张表的行数、校验和、耗时、跳过的表和列、序列调整）。可以对着正在运行的生产库做：只读快照事务，不影响写入，只增加读负载。

**停机时间估计。** 停机 = 停掉旧 Control 之后到新 Control 开始服务：导入耗时（以 dry-run 报告的总耗时为准）加启动和签证书（秒级）。2026-09-29 的库约 150 MB，其中包字节不导入，预计在分钟级；准确数字只能由 dry-run 给出，所以把 dry-run 列为所有者的必做步骤。

**切换步骤（所有者执行，不可由代理代做）。**

1. 运行附录 A 的只读 SQL，记录数字。
2. 在生产机或一台临时机上用 `--dry-run` 跑一次，读报告，确认没有被拒绝的表。
3. 设定新主机时区与旧面板相同；确认 80、443 可用（停掉 nginx 等）。
4. 停止旧服务：`sudo systemctl stop anix-control`（旧单元）。PostgreSQL 保持运行、只读使用。
5. 用新安装器安装程序但不启动（`install.sh --version <tag> --domain <域名> --skip-start`），然后执行不带 `--dry-run` 的导入。
6. 启动：`sudo systemctl start anix-control`；确认证书签发、管理员能登录、抽几个用户的订阅链接与切换前逐字节相同。
7. 旧安装目录和 PostgreSQL 保留至少 14 天，不删除、不再写入。

### 3.6 新装流程

1. 把域名的 A（和可选的 AAAA）记录指向这台机器，不开 CDN 代理。
2. 在机器上执行一条命令（2.6）。安装器：检查 80、443 → 建用户和目录 → 下载并校验程序 → 写 `config.yaml`（只有 `domain`）→ 装单元 → 启动并等 `READY=1` → 读证书状态 → 执行 `anix-control admin claim-link`，打印领取链接。
3. 打开链接，设置邮箱和密码，进入后台。

此后不需要再登录服务器，除非接新机器。

### 3.7 首个管理员领取链接（N10）

| 项 | 规则 |
|---|---|
| 何时生成 | 库里没有管理员时，由 `anix-control admin claim-link` 生成（安装器在最后调用一次）。服务启动时不自动生成，只在日志写“尚无管理员，运行 sudo anix-control admin claim-link 取得领取链接” |
| 令牌 | `anixclaim_` 加 32 字节随机数的 base64url（256 位） |
| 存放 | 只存 SHA-256 哈希和到期时间，在 `v5_control_meta` 的 `admin_claim` 键 |
| 有效期 | 24 小时；每次执行命令都生成新令牌，旧的立即作废 |
| 一次性 | 领取成功在同一事务里建管理员、删除令牌；之后领取接口对任何令牌都答 404 |
| 链接形式 | `https://<domain>/claim#anixclaim_…`。令牌放在片段里，浏览器不会把它发给服务器，所以不进访问日志和 Referer；页面脚本读出后用 POST 体提交 |
| 提交 | `POST /api/v5/claim {token, email, password}`，公共限流；常量时间比较哈希；密码规则与修改密码相同；身份已切到 identity 时经 identity 建账号，否则走今天的建管理员路径 |
| 日志和审计 | 令牌从不写日志；请求体不被记录；审计只记 `admin_claimed`、邮箱和来源地址 |
| 显示在哪里 | 只打印到执行命令的终端（标准输出），不进 journald、不进文件 |
| 还原和迁机 | 库里已有管理员，不生成领取链接 |

随之删除：`admin.email`、`admin.password`（config.go:597-601）、`.bootstrap-admin-password` 文件、安装器的 `--admin-email`/`--admin-password`，以及 `InitAdmin` 在没有密码时拒绝启动的规则（init_admin.go:20, 62-64）。

**实现（P9）。** 代码在 `internal/adminclaim`、`cmd/server/admin_command.go`、`internal/handler/claim.go` 和 `web/src/views/Claim.vue`。与上表的差别和补充：

- 管理员指 `v2_user.is_admin = 1` 的账户（被封禁的也算）；身份切到 identity 后它是 identity 的投影，判断不变。
- `GET /api/v5/claim/status` 只回答 `{claimable}`，含义是“库里还没有管理员”，不说明是否生成过链接或是否过期：链接过期时页面仍显示表单，提交后才告诉持有令牌的人已过期，没有令牌的人只会得到“无效”。
- 回答码：已有管理员 404；令牌不是当前待领取的 403 `claim_invalid`；过期 410 `claim_expired`；邮箱格式 400；密码规则 422（`weak_password`、`password_too_long`）；邮箱已被普通账户占用 409；identity 不可达 503。成功 201，回答 `{user_id, email, next: "/login"}`，页面随后用同一邮箱和密码走普通登录，不另发会话。
- 密码规则：仓库没有修改密码接口，采用建账户的规则（至少 6 个字符；bcrypt 最多读 72 字节）。
- 身份已切到 identity（`identity` 或 `finalized`）时，内核事务里建订阅者和账户关联（`finalize` 前镜像密码哈希）、消耗令牌、写审计，提交后用 identity 的 `ImportAccounts` 单账户导入把账户交给 identity；identity 拒绝时撤销这些行并恢复令牌。`importing` 状态仍走内核路径，下一次增量导入带过去。
- 命令输出：没有 `domain` 时打印路径和本机监听地址；Control 自己终止 TLS 且网页证书未被公共信任（或还没有证书状态）时，加一行本机第一个公网地址。`-ttl` 取 1 分钟到 24 小时。命令的日志（包括 SQL 日志）走标准错误，标准输出只有链接。
- 请求体不进任何日志：访问日志从不记录请求体，审计中间件对 `/api/v5/claim` 不捕获请求体，审计行由领取逻辑自己写。限流与公共接口相同（每地址 5 次/秒，突发 10），测试模式下也生效。
- 领取页面没有语言切换（登录页首次加载的预算），文案是单独的消息组，随页面加载。
- 安装器（P11 前的最小改动）：不再写 `admin` 和 `.bootstrap-admin-password`，拒绝 `--admin-email`/`--admin-password`，安装或更新成功后以服务用户身份执行一次 `admin claim-link`；已有管理员时不打印。

### 3.8 自动备份（N32）

**备份包。** `backups/anix-control-<UTC 时间>-<版本>-<类型>.tar.gz`，内容：

| 文件 | 来源 |
|---|---|
| `manifest.json` | 格式 `anixops.backup/v1`，程序版本、最高迁移号、创建时间、类型、域名，以及其余每个文件的 SHA-256 和关键表行数（`v2_user`、`v2_order`、`v2_node`） |
| `control.db` | `VACUUM INTO` 生成的一致快照 |
| `root.key` | 根密钥 |
| `config.yaml` | 覆盖值配置 |

用 `VACUUM INTO`（SQLite 3.27 起支持，当前 3.41.2）而不是在线备份 API：驱动没有暴露在线备份接口（modernc.org/sqlite v1.23.1 和 glebarez/go-sqlite v1.21.2 的 Go 层都没有），`VACUUM INTO` 是普通 SQL，在一个读事务里产出一致且压缩过的副本，现有代码已经在用（system_service.go:514-527）。ACME 证书和账户在库里，自动包含。

**写入过程。** 先写到 `backups/.tmp-<id>/`，逐个 fsync，打包，校验，再原子改名；磁盘剩余空间小于库大小的 3 倍时不写并告警。

**时间表。**

| 类型 | 时机 |
|---|---|
| `daily` | 每天本地时间 03:30 加随机 0–30 分钟，单例工作线程执行 |
| `pre-migration` | 启动时有待执行的表结构迁移，执行前（3.3） |
| `pre-upgrade` | 升级前；M4 的一键升级调用同一个函数，M2 由安装器的 `update` 调用 `anix-control backup create --kind pre-upgrade` |
| `manual` | 后台“立即备份”或 `anix-control backup create` |

**保留。** 最近 7 个 `daily`，最近 3 个 `pre-migration`/`pre-upgrade`，最近 3 个 `manual`，以及最近一个验证通过的包。都是内置常量，删除备份设置页里不起作用的定时、保留和 S3 字段（N4 的一部分）。

**每晚验证。** 每天的备份写完后：解到 `tmp/verify-<id>/`（0700）→ 核对清单里的摘要 → 只读打开，`PRAGMA integrity_check` 为 `ok` → 迁移号与清单一致 → 用包内的 `root.key` 解开包内库里的 CA 私钥（证明密钥和库是一对）→ 关键表行数与清单一致 → 删除临时目录。结果写入新表 `v5_backup(id, file, kind, size, sha256, program_version, created_at, verified_at, verify_error)`。首页显示“上次验证通过的备份：时间”；失败产生告警 `backup_verify_failed`。

**加密：本地包不加密。** 备份包和数据库、根密钥在同一块盘上，能读到备份目录的人也能读到 `control.db` 和 `root.key`，用根密钥加密备份既不增加保护，又会造成循环依赖（解开备份需要的密钥就在备份里）。保护靠权限：目录 0700、文件 0600、属主 `anix-control`。异地副本（S11，M4）离开这台机器，必须加密，届时用一把单独的恢复密钥，在后台显示一次由所有者保存，不用根密钥。

**下载。** 后台“下载最新备份”只对超级管理员开放，下载前重新输入密码，写审计。

### 3.9 还原和迁机（N33）

`anix-control backup restore <备份包> [--replace]`：

1. 拿不到 `control.lock`（服务在运行）就拒绝，提示先 `systemctl stop anix-control`。
2. 核对清单和每个文件的摘要。
3. 包里的迁移号高于本程序认识的最高号：拒绝，提示需要的最低版本。
4. 数据目录里已有 `control.db` 或 `root.key`：拒绝；带 `--replace` 时先把它们移到 `replaced-<UTC 时间>/`，从不删除。
5. 写入文件，设属主和权限，不启动服务。下次启动时若包比程序旧，照常执行待做的迁移（之前会先自动备份一次）。

`install.sh --restore <备份包>`（新机器）：建用户和目录 → 安装 `--version` 指定的程序 → 执行 `backup restore` → 用包里的配置（可用 `--domain` 改域名）→ 启动。不生成新的根密钥，不生成领取链接。目标机器上已有数据库时拒绝。

### 3.10 看门狗和外部心跳（N34）

- 单元为 `Type=notify`、`WatchdogSec=60`（3.1）。Control 在监听全部绑定、启动流程完成后发 `READY=1`；之后每 20 秒检查一次：数据库 `SELECT 1` 在 5 秒内返回、写连接能在 10 秒内拿到、事件循环未卡死，全部通过才发 `WATCHDOG=1`。连续 60 秒没有心跳时 systemd 重启进程（`Restart=always`）。表结构迁移期间发 `EXTEND_TIMEOUT_USEC`，关闭时发 `STOPPING=1`。
- 实现约 40 行：向 `$NOTIFY_SOCKET` 写 unixgram 数据报，不引入 go-systemd（它不在 go.mod 里）。
- 外部心跳：后台 › 系统里的一个可选地址（HTTPS），默认空。设置后单例工作线程每 60 秒在上述检查和告警扫描都正常时 GET 一次；外部的“停止心跳即报警”服务负责通知。这是后台设置而不是配置键，因为它需要随时在网页上改。

### 3.11 /metrics（N40）

默认只回答两类请求：来源地址是本机回环（`127.0.0.1`、`::1`，按直连地址判断，不看转发头），或带有效的管理员 API 令牌（`Authorization: Bearer anixadm_…`，复用 `middleware.AdminAPIToken`，router.go:86-91）。其他请求答 404。没有配置项。抓取方在同机时零配置；在别处时在后台建一个令牌。

### 3.12 删除和保留

**删除的文件和目录。**

| 类别 | 内容 |
|---|---|
| 容器 | `Dockerfile`、`Dockerfile.module`、`.dockerignore`、`docker-compose.yml`、`docker-compose.prod.yml`、`docker-compose.modules.yml`、`.env.example` |
| 部署资产 | `config/deploy/compose/`、`config/deploy/helm/`、`config/deploy/grpc_tls/`、`config/deploy/examples/nginx/`、`config/deploy/deploy_panel.sh`、`config/scripts/deploy.sh`、`pre-deploy.sh`、`v2board.service`、`modules_compose_smoke.sh`、`modules_kind_smoke.sh` |
| 数据库工具 | `cmd/sqlite2postgres`、`scripts/migrate_latest_sqlite_to_postgres.sh`、`scripts/postgres_restore_rehearsal.sh`、`scripts/staging/`（Compose 加 PostgreSQL 的路由预演，路由模式取消后无用） |
| 配置模板 | `config/config.yaml.example`、`config/config.prod.yaml`、`config/config.dev.yaml.example`（N3）。开发实例的覆盖值写在 Makefile 里 |
| 安装器 | `preflight`、`migrate`、`rollback` 命令，旧布局识别和 `bin/v2board` 链接（N7；install.sh:154-171, 1022-1096, 1141-1142） |
| Makefile | `docker-build`、`docker-run`、`docker-stop`、`deploy`、`pre-deploy` |
| 测试 | `scripts/tests/test_compose_init_secrets.sh`；`cmd/server/main_test.go:511-530` 改为不再读 `Dockerfile` 和 `deploy_panel.sh` |

**删除或改写的 CI 作业（.github/workflows/ci.yml）。** 删除：Build Smoke Images（619-677）、Docker Build Smoke（679-809）、Helm Chart Checks（811-880）、Kubernetes Smoke（882-1007）、PostgreSQL Stats Regression（1233-1265）、Package Storage PostgreSQL ×4（1267-1349）、SQLite to PostgreSQL Dry Run（1351-1416）、PostgreSQL Restore Rehearsal（1418-1465）、Staging Rehearsal Smoke（467-517）、Build Docker Image（2424-2573）、Publish Edge Docker Image（2575-2667）。改写：Cross-Repo E2E 去掉 PostgreSQL 服务（1970-1983, 2060），Deployment Script Checks 去掉 Compose 和 PostgreSQL 步骤（145-152, 162-163, 231-232），backend-build 和 release-binaries 的 `needs`（2320, 2360），release 作业的镜像部分（2672, 2706-2710, 2733-2740, 2785）；`config/scripts/check_release_workflow.sh`（14-35, 352-367, 376-390）、`classify_changes.py` 的 db 和 modules 类（59-114）、`plan_test_shards.py` 的 postgres 套件（15, 62-97）同步修改。新增一个临时作业 “PostgreSQL Import”：PostgreSQL 15 服务容器，跑导入命令的测试（建库、灌夹具、导入、对账），M5 随导入命令删除。

**改写的文档。** `docs/DEPLOYMENT.md`（只剩一种安装方式）、`docs/guide/release-installation.md`、`docs/reference/quickstart.md`、`docs/reference/startup-config.md`、`docs/reference/configuration.md`、`docs/reference/runtime.md`、`docs/control-boundary.md`、`docs/reference/environment-variables.md`（重新生成）、`docs/UPGRADE.md`（新增 5.x 迁移一节）、`docs/RELEASING.md`（产物清单）、`docs/reference/repository-layout.md`、`docs/architecture/module-runtime.md`（删去远程运行时）、`AGENTS.md`（发布规则里“容器是主要部署”、版本号清单里的 Helm 和三个模板、CI 说明里的 PostgreSQL 作业）。删除 `docs/architecture/container-deployment.md`、`docs/reference/sqlite-to-postgres-migration.md` 和 `docs/guide/staging-rehearsal.md`（路由预演，路由模式取消后无用）；`docs/guide/v4-plugin-only-upgrade.md`、`v4-plugin-only-rollback.md` 讲的是手工导入包，标为历史文档，随 N42（M5）移入 `docs/archive`。

**Docker 镜像：不保留。** 理由：没有任何 CI 作业用 Control 镜像测 Agent，两个跨仓库 E2E 都是直接构建二进制（1.2）；Edge 不需要镜像，`go_dev` 的构建产物可以用同样的安装器装到测试机。保留镜像就要保留一条只读根文件系统、没有 80/443 能力、不能写 `/var/lib` 的第二种运行形态，与 S2 相悖。镜像里携带的 `/install.sh.sig`（ci.yml:2473-2484）改为在发布构建时嵌入二进制（第 4 节的构建顺序）。

**保留。** `scripts/install.sh`（重写）、根目录 `install.sh` 和 `panel_install.sh` 两个按标签拉取安装器的入口、`config/deploy/check_release_build_policy.sh`、`config/deploy/clean_local_build_artifacts.sh`、`config/deploy/ansible/`（节点侧 Ansible 后备；其中 `nodes/` 属于 5.0 阶段 8 的删除范围，不在本文）、`config/deploy/examples/prometheus/`（改为带管理员 API 令牌抓取，3.11）。

## 4. S4：官方包内置

### 4.1 怎么内置

提出的两个选项都不可行：完整的 `.anxp` 每个架构约 0.5 GB（1.4），go:embed 会让程序变成半个 GB；放在程序旁边的目录同样大，而且在 SQLite 里还会再存一份字节（`v3_kernel_plugin_artifact.Data`），每次升级再增加一份。

**决定（B20）：把各包的宿主代码编译进 Control，签名的“精简包”嵌入。**

- **一个程序多个入口。** 每个包的 `control/main.go` 拆成可导入的库（`Run(ctx) error`）加一个空壳；新包 `packages/builtin` 汇总 id 到入口函数的表；`anix-control package-host <id>` 运行对应的宿主。宿主仍是独立的子进程，进程隔离、Unix socket 协议、桥接 socketpair、环境变量都不变（internal/pluginhost/process.go:49-150），只是可执行文件从“落盘并校验过的宿主二进制”换成 `/proc/self/exe`。用 `/proc/self/exe` 而不是路径：升级时程序文件被替换，正在运行的旧进程拉起的宿主仍是旧版本。
- **精简包。** `build_package.py` 增加 `--builtin` 模式：照常生成并签名 manifest，包里不放 `bin/control-host-*` 和 Agent 载荷，只放路由表、迁移索引、配置模式、WebUI；manifest 的 `control_entrypoint` 写成 `{"builtin": true}`。体积为 KB 到几百 KB 级（带 WebUI 的 forward 最大）。
- **嵌入。** 精简包、manifest、签名和公钥放进 `internal/builtinpkg/dist/`，用 go:embed 嵌入。仓库里提交一个空的占位索引，`go build ./...` 在没有包时也能通过；`make build` 用临时密钥生成开发用的精简包（build_package.py:945-960 已支持临时密钥）。
- **信任。** 内核仍走今天的校验路径：manifest 签名（kernel_service.go:549-574）、精简包摘要（kernel_service.go:1093）。嵌入目录自带公钥，发布构建在 CI 里核对它等于官方公钥（与今天 ci.yml:1690-1698 的做法相同），`plugins.official_public_key` 不再是配置项。宿主二进制的来源可信度从此等于 Control 程序本身，所以发布增加 `SHA256SUMS.txt.sig`（同一把官方 Ed25519 密钥签名），安装器用内置的公钥校验后再校验程序；今天的 `SHA256SUMS.txt` 没有签名（ci.yml:2752-2754）。
- **体积估计。** 用到包存储的宿主之间只差 0.0–0.9 MB（30.1 到 31.0 MB），共同部分（gRPC、GORM、SQLite 驱动、SDK）Control 本体已经包含。估计内置后程序为 65–75 MB（今天 58.7 MB），再加嵌入的前端（`unknown`，PR 中实测）。全套 `.anxp` 和镜像不再发布，发布页总体积从几百 MB 降到两个程序。

`config/scripts/check_package_boundaries.sh` 只禁止包代码依赖内核的 `internal/`（脚本 2-8 行注释和检查逻辑），内核导入包代码不违反它；脚本开头“包与内核分开构建和发布”的注释改为新的事实。

### 4.2 每次启动

在表结构迁移之后、HTTP 监听之前，替换今天只处理 identity-platform 的 `BootstrapIdentityPlatformPackage`（plugin_bootstrap.go:33-105）：

1. 读嵌入的索引，逐个校验 manifest 签名和精简包摘要。校验失败说明程序文件损坏：该包标记为失败、产生告警 `builtin_package_invalid`，其他包继续。
2. 用嵌入的公钥激活信任根（`EnsurePluginTrustRoot`，作废其他根）。
3. 排序：按 manifest 的 `dependencies` 拓扑排序（目前全为空），同层按固定顺序 identity-platform、platform，其余按 id。
4. 对每个包：发布记录不存在就登记（`RegisterPluginRelease`，幂等）；存精简包（`StorePluginArtifact`，同字节幂等）；安装记录的 `desired_version` 设为内置版本、`enabled = true`。
5. 交给现有的生命周期工作线程执行（operation_worker.go）：未运行的启用，`observed_version` 不同的升级（启动新宿主、运行包迁移、健康检查）。
6. 内置的旧版本只保留发布记录供审计；旧版本的包字节在升级成功后删除，只留上一版。

**失败处理。** 任何一个包的失败都只让该包处于 `failed`，记录 `last_error`，停止它的宿主，产生告警；Control 照常服务，该包的路由答 503 `package_unavailable`。今天 identity-platform 引导失败会让 Control 拒绝启动（bootstrap.go:160-162），改为不致命。内置包没有“上一版本”的代码可回退，所以对内置包关闭生命周期工作线程的自动回滚（operation_worker.go:214-217）；回退整个程序靠备份还原（3.9），M4 的一键升级会自动做。identity-platform 失败时没人能登录，这时的入口是服务器上的 `anix-control doctor`（列出失败的包和原因）和还原命令。

**与插件内核契约的关系**（docs/architecture/plugin-kernel-contract.md）：

- 信任根（契约 20-39 行）不变，只是公钥从配置变成嵌入。
- 包字节仓库（41-47 行）对内置包只存精简包；“宿主重新校验包摘要”变成“宿主就是正在运行的程序”。
- 生命周期计划（186-195 行）、包迁移（408-436 行）、配置（197-263 行）、作用域授权和内核能力（157-174 行）不变。
- 不再有导入，所以“只接受官方 manifest”自然成立；第三方包不支持执行，与今天相同（config.go:386-387 的注释）。

契约文档在实现 PR 里同步修改这些段落。

### 4.3 插件中心剩下什么

| 保留 | 删除 |
|---|---|
| 包列表：id、版本、状态、健康、最后错误 | 导入发布（`PluginReleaseImportDialog.vue`、`Plugins.vue:566-592`；`POST /api/v3/plugin-releases` 和 `POST /api/v3/plugin-releases/:id/artifact`，router.go:678, 680） |
| 重启宿主 | 安装、升级、回滚、停用、启用动作（`PUT /api/v3/plugin-installations`，`POST …/:id/actions` 只留 `restart`，router.go:682-683） |
| 包配置（`GET/PUT /api/v3/plugin-installations/:id/config`） | 路由模式：页面 `RouteModes.vue`、`/api/v4/kernel/route-modes*`（router.go:790-795）、`anix-control routes`、`package_routes.default_mode`、影子模式和样本收集（`internal/shadowsamples`，main.go:639） |
| 扩展目录和 WebUI 资源（`/api/v3/extensions*`） | 三个开关：`plugins.control_execution_enabled`（固定为开）、`plugins.dispatch_enabled`、`plugins.topology_execution_enabled`（及三个 `*_poll_interval`） |
| 操作列表（只读） | 远程运行时（3.2）、`plugins.identity_bootstrap_package_dir` |

**路由模式取消后的行为。** 有效模式只由 `config/package-route-defaults.json` 的预演清单决定（今天 `rehearsed` 默认策略的结果）：清单里的路由在包版本达到 `min_version` 时走 native，其余走 legacy。安装配置里存的 `routes` 键由一个迁移删除，删除前把原值写进审计。N6（M5）再把全部路由改为 native 并删除 legacy 处理器。

**Agent 侧载荷（B23）。** machine-telemetry、nftables-forward、gost-mesh、nat-egress 四个包有 Agent 插件（每个架构一个，machine-telemetry 实测 16.3 MB），gost-mesh 还带 gost 运行时。M2 内置它们的 Control 一侧（machine-telemetry 的路由供节点列表的服务状态列使用），不内置 Agent 载荷。Agent 插件下发和拓扑执行不启动，这与今天生产的配置一致（config.prod.yaml:131, 135），也与 docs/features.md:40-42 对这三个运行时“生产执行保持关闭”的说法一致。M4 的 Agent 发布清单（S9）把 Agent 插件和 gost 作为 Agent 发布的一部分，由 Control 下载校验后下发，届时这两条路径恢复，且不再有开关。部署编排页在 M2 的行为不变（今天生产上就不执行），它的去留属于 S7（M3）。

### 4.4 社区版和商业版（B24）

商业包内置却被 `app.edition: community` 隐藏，等于没有内置。S5 已定取消版本区分，所以 M2 的内置包 PR 同时删除：`app.edition`、`config/editions.json` 和路由过滤（router.go:47-49；internal/router/edition.go；internal/edition）、`build_package.py` 的 `--edition`（87-98, 1299-1304）、`.github/workflows/commercial-packages.yml`。订单、支付、返佣包进入每个公开发布。AGENTS.md 的 “Editions (H23)” 一段同步改写（转发里预留的 `/api/v4/forward/self/` 等前缀今天没有实现，不受影响）。返佣提现和支付网关页的删除属于 S5（M3）。

### 4.5 CI 和发布

- **构建顺序：** 先包后程序。`plugin-package-publish` 改为产出精简包和 `agent-install.sh.sig`，作为制品交给 `release-binaries`；后者把它们放进 `internal/builtinpkg/dist/` 后再 `go build`。
- 发布矩阵从 linux、windows、darwin 各两个架构（ci.yml:2365-2383）减为 linux/amd64 和 linux/arm64：内置的宿主只支持 Linux（build_package.py:44），程序只以 systemd 形态运行。
- 不再生成 Agent 插件和 gost 运行时（ci.yml:1621-1665），不再发布 `anix-control-packages-<版本>.tar.gz`、identity-platform 三件套和前端压缩包；`check_release_workflow.sh` 和 `docs/RELEASING.md` 的产物清单同步修改。
- Backend Tests 和 Go Quality Gates 在占位索引下构建；跨仓库 E2E 用 `make build` 生成的开发精简包。

## 5. 配置面

**最终的操作员配置键（B25）。** 一般安装的 `config.yaml` 只有一行 `domain: panel.example.com`。

| 键 | 默认 | 说明 |
|---|---|---|
| `domain` | 无，acme 模式必填 | 面板域名。网页、API、订阅、安装脚本的地址，ACME 证书的名字，节点 gRPC 的默认名字 |
| `tls.mode` | `acme` | `acme` 或 `external`（2.5） |
| `tls.cert_file` | 空 | 仅 external：gRPC 证书文件，变化后热加载 |
| `tls.key_file` | 空 | 同上 |
| `listen` | `127.0.0.1:8080` | 仅 external：HTTP 明文监听地址 |
| `grpc.port` | `50051` | 节点 gRPC 端口 |
| `grpc.public_name` | 等于 `domain` | 仅当域名套了 CDN 时，设为节点直连的另一个名字。不叫 `grpc.host`，因为旧的 `grpc.host` 是监听地址，语义不同 |
| `subscribe_path` | `s` | 订阅路径。只为保住已经发出去的订阅链接，新装不需要改 |
| `data_dir` | `/var/lib/anix-control` | 开发和测试用 |

每个键都有对应的 `ANIX_CONTROL_*` 环境变量，`docs/reference/environment-variables.md` 由现有生成器（internal/config/env.go:256-273）重新生成，从 149 行变为 9 行。

**变为内置默认值的键。** 不再从文件读取，值是 Go 常量或由其他值派生：

| 原键 | 去向 |
|---|---|
| `env`、`server.mode`、`read_timeout`、`write_timeout`、`shutdown_drain_delay`、`server.cors.*` | 常量（生产行为）；开发实例由 `make run` 带 `-debug` 标志 |
| `server.host`/`port`、`frontend.*`、`tls.enable` | 由 `tls.mode` 决定（2.2） |
| `server.trusted_proxies` | acme 为空，external 为回环 |
| `grpc.enabled`、`grpc.host` | 始终开启、监听全部地址 |
| `log.format`、`log.level` | 文本格式写 journald，级别 info；`-debug` 标志改为 debug |
| `jwt.expire`、`auth.*`、`alerts.*`、`forward_runtime.latency.*`、`forward.anixops_experimental`、`plugins.control_host_*` | 今天的默认值变常量。注册策略（`auth.registration.*`）在 M3（N35）变为后台开关 |
| `agent_install.public_url`、`grpc_target`、`signature_file` | 由 `domain`、`grpc.public_name`、`grpc.port` 派生；签名嵌入程序 |
| `agent_install.artifact_dir` | 固定为 `<data_dir>/agent-releases`，M4（N30）由 Control 自己下载 |
| `agent_control.mtls`、`legacy_sunset` | 固定为 `required`（5.0 阶段 8 删除其他模式） |
| `plugins.official_public_key` | 嵌入（4.1） |
| `jwt.secret`、`identity.kek` | 由根密钥用 HKDF-SHA256 派生，标签分别为 `anixops jwt v1`、`anixops identity kek v1`（N8） |
| `module_runtime.ca_kek` | 变为数据目录里的 `root.key` 文件（N8） |

**删除的键（废弃，出现时只警告）。** `cache.*`（5 个，无读者）、`app.name`、`app.version`、`app.traffic_log_enable`、`app.edition`、`log.output`/`file_path`/`max_size`/`max_backups`/`max_age`（无读者）、`tls.domain`（只进日志）、`database.*`（13 个）、`admin.*`（N10）、`module_runtime.*`（远程运行时）、`plugins.*` 其余各键、`package_routes.default_mode`、`forward_runtime.clean_agent.public_url` 和 `token_expire_seconds`（无读者）。

**暂时照读、不再写入的键。** 这些键今天仍有读者，所属功能在后续里程碑删除；M2 照读但警告“将在 Mx 删除”，配置改写时不保留：`app.api_token`、`grpc.api_token`（N9，M5）；`forward_runtime.*` 除 `latency.*` 外的各键（flux 运行时，N5，M5）；`agent_install.agent_version`、`agent_install.cn_mirror_url`（S9 和 N30，M4）。

**加载规则（N3）。**

- 先取内置默认值，再用文件逐键覆盖，再用环境变量覆盖；文件不再整体替换默认值（config.go:754-766 改写）。
- YAML 用严格模式解码。键在当前列表里：生效。键在废弃列表里：警告一次，忽略。都不在：拒绝启动，提示最接近的合法键。废弃列表是 4.2 全部 151 个键（environment-variables.md 当前内容）加上历史模板里出现过的键（如 `forward_runtime.iptables_ansible`），作为数据文件入库。
- 配置文件里不再有任何秘密：根密钥在数据目录，其余派生。

**升级时改写。** `anix-control config rewrite --in <旧文件> --out <新文件>` 由安装器在 `update` 时调用（服务以非 root 运行，不能改 `/etc` 下 root 拥有的文件）：

1. 读旧文件，按上面的映射得到新键：`domain` 取自 `agent_install.public_url` 的主机，其次 `tls.domain`；`grpc.port`、`app.subscribe_path` 只在不等于默认值时保留；`grpc.tls_cert_file`/`tls_key_file` 在没有改用 ACME 时映射为 `tls.cert_file`/`tls.key_file` 并写 `tls.mode: external`。
2. 只写与默认值不同的键。
3. 旧文件留作 `config.yaml.bak-<UTC 时间>`。
4. 打印被丢弃的键和原因。

生产切换时由导入命令做同样的事（3.5 第 8 步）。

## 6. PR 拆分

每个 PR 都按文档同步规则更新 `CHANGELOG.md` 和受影响的文档。“节点可见”指已接入的 Agent 能否察觉。

| # | PR | 范围和主要文件 | 必需的测试 | 表结构 | 节点可见 | 依赖 | 需要所有者 |
|---|---|---|---|---|---|---|---|
| P1 | SQLite 参数与写入方案 | `internal/database`（参数、写读两个句柄）、`internal/service/server_service.go`、`internal/subscriber/traffic.go`（批量更新）、`internal/grpc/agent_control_server.go`（心跳节流）、`internal/authn/revocation.go`、新的清理单例任务 | 小时合并的流量结果与逐条相同；保留期任务分批删除；`BenchmarkTrafficReportSQLite` | 是：`v2_server_log` 唯一索引 | 否 | 无 | 否 |
| P2 | 有序迁移与程序版本（N36） | `internal/database/migrations`、`0001_baseline.sql`、`v5_schema_migration`、`v5_control_meta`；删除 `migrateSchema` 和 `Ensure*` 加列；`cmd/server/bootstrap.go` | 迁移与模型一致；上一版本基线升级与全新安装一致；旧程序拒绝新库；迁移 checksum 被改时拒绝 | 是：两张新表 | 否 | 无 | 否 |
| P3 | PostgreSQL 导入命令 | `cmd/server/import_postgres_command.go`、`internal/pgimport`；CI 作业 “PostgreSQL Import”（postgres:15） | 夹具库导入后行数和校验和相等；缺列、多列、非空多余表被拒；序列；dry-run 不留文件；对着只读角色运行 | 否（只写新库） | 否 | P2 | 否（生产运行属于 O2、O3） |
| P4 | 删除 PostgreSQL 运行路径 | 3.2 的全部内容；`sdk/packagestoresdk`；远程运行时；PostgreSQL CI 作业；`classify_changes.py`、`plan_test_shards.py` | 现有 SQLite 测试全绿；`go -C sdk test ./...` | 否 | 否 | P3 | 否 |
| P5 | 单一 HTTP 监听与自带证书（S1、N1、N2） | `internal/panelcert`、`cmd/server/main.go` 监听部分、`internal/grpc/server.go` 的 `tlsConfig`、`v5_acme_cache`、`kernelalerts` 两种新告警、`/api/v5/panel/status`、`anix-control tls retry`、`anix-control doctor` 的证书和 DNS 检查、网页横幅 | Pebble 签发（TLS-ALPN-01 和 HTTP-01 各一次）；续期后 HTTPS 和 gRPC 新连接用新证书，gRPC 仍要客户端证书；失败时自签兜底且握手不阻塞；external 模式文件变化热加载 | 是：`v5_acme_cache` | 是：证书来源改变（同一名字、公共 CA），连接在 30 分钟内自然换新 | 无 | 否 |
| P6 | 内置官方包（S4） | `packages/*/control` 拆库、`packages/builtin`、`anix-control package-host`、`internal/pluginhost` 的内置入口、`internal/builtinpkg`、`build_package.py --builtin`、新的启动引导、删除 `app.edition` 和商业包工作流、ci.yml 发布作业顺序和矩阵、`SHA256SUMS.txt.sig`、契约文档 | 冷启动安装全部包；同版本重启无操作；版本变化时升级并跑包迁移；一个包失败时其余照常且 Control 可用；签名或摘要不符被拒；宿主经 `/proc/self/exe` 启动 | 否（沿用现有表） | 否（Agent 下发保持关闭） | 无 | 否 |
| P7 | 插件中心收缩 | 4.3 的删除项（网页、API、CLI、`internal/shadowsamples`）；删除安装配置里 `routes` 的迁移 | 删掉的接口答 404；路由模式只由预演清单决定；`check_plugin_only_routes.py` 和路由目录同步 | 是：删除存储的路由模式 | 否 | P6 | 否 |
| P8 | 根密钥与配置面（N8、N3） | `root.key` 首次生成和启动时解开 CA 的检查；JWT 和 identity KEK 派生；`internal/config` 新加载器、严格解码、废弃键列表、`config rewrite`；删除三个模板；重新生成环境变量文档 | 未知键拒绝、废弃键警告；改写只留覆盖值；根密钥解不开 CA 时拒绝启动并说明；派生值稳定 | 否 | 否 | P2 | 否 |
| P9 | 首个管理员领取链接（N10） | `anix-control admin claim-link`、`/api/v5/claim`、领取页面、删除 `admin.*` 和 `InitAdmin` 的强制密码 | 令牌一次性、过期、轮换后旧令牌失效、已有管理员时 404；令牌不进任何日志（测试捕获日志断言） | 是：`v5_control_meta` 的一行 | 否 | P8 | 否 |
| P10 | 备份、验证与还原（N32、N33） | `internal/backup`、`v5_backup`、每日和迁移前备份、每晚验证、`anix-control backup create/restore`、后台下载、首页“上次验证通过的备份”；删除备份设置页里不起作用的字段和旧的在线还原 | 备份在写入并发下一致；验证能发现损坏和密钥不配对；还原拒绝非空目标、`--replace` 不删除；从旧版本备份还原后迁移照常执行 | 是：`v5_backup` | 否 | P2、P8 | 否 |
| P11 | systemd 形态与安装器（S2、N7、N34、N40） | 重写 `scripts/install.sh`（只问域名、端口预检、`--restore`、`--external`、`--skip-start`、`update` 调用 `config rewrite` 和备份）；单元文件；sd_notify 看门狗；外部心跳设置；/metrics 访问规则；前端嵌入；删除 3.12 的文件和 CI 作业 | `scripts/tests` 覆盖新装、升级、还原、端口占用；看门狗在数据库卡住时停发心跳；/metrics 对外部来源 404 | 否 | 是：节点安装命令里的地址变为 `https://<domain>`（同一域名时不变） | P5、P6、P8、P9、P10 | 否 |
| P12 | 文档、迁移手册与发布 | 3.12 列出的文档；`docs/UPGRADE.md` 的切换手册（3.5 的步骤）；`CHANGELOG.md`；版本号 | 文档门禁、`docs_release_claims_test.py` | 否 | 否 | P11 | 打标签由所有者执行 |

**所有者步骤（不可由代理执行）。**

| # | 步骤 | 时机 |
|---|---|---|
| O1 | 在生产库运行附录 A 的只读 SQL，把结果交给实现者 | 现在，P3 之前 |
| O2 | 用发布前的构建对生产库做 `import-postgres --dry-run`，审阅报告 | P3 合并后 |
| O3 | 生产切换（3.5 的切换步骤 3–6）：停旧服务、导入、安装、启动、核对 | M2 发布后 |
| O4 | 14 天后决定是否下线旧安装目录和 PostgreSQL | O3 之后 |

**并行。** P1、P2、P5、P6 可以同时开始；P3 在 P2 之后；P4 在 P3 之后；P7 在 P6 之后；P8 在 P2 之后，P9 在 P8 之后；P10 需要 P2 和 P8；P11 汇合 P5、P6、P8、P9、P10；P12 最后。关键路径是 P2 → P8 → P10 → P11 → P12。

## 7. 风险与待定问题

| 风险或问题 | 建议 |
|---|---|
| **生产库迁移。** 生产是 PostgreSQL 15.18；今天的用户、订单数是 `unknown`（2026-09-29 为 18 个用户、0 个订单，release-line-status.md:97-99）。导入出错会让订阅链接失效 | 先跑附录 A 和 dry-run（O1、O2）；以导入代替重建（3.5）；对账失败即中止；旧库保留 14 天 |
| **生产库处于哪个阶段。** 身份切换状态、节点密钥拆分阶段、flux 表是否已删、是否存过路由模式、是否有 identity 签名密钥，都只在生产库里 | 附录 A 的第 3–7 条查询回答这些；导入照原样复制状态表，有异常状态时在 O2 时决定先在旧面板上完成或回退该阶段 |
| **80 和 443 是否可用。** 生产机上今天大概率由 nginx 或 Caddy 占着（docs/DEPLOYMENT.md:487-522 的部署方式），是否还有别的网站共用这台机器是 `unknown` | 切换时停掉 nginx；若有其他网站，用 `tls.mode: external` 并保留 nginx（2.5） |
| **域名是否套 CDN。** `unknown` | 安装器和 `doctor` 的 DNS 探测会指出；套了就必须关闭代理，或用 external 加 `grpc.public_name` |
| **ACME 配额。** 反复重装会耗尽重复证书配额 | 证书随备份保存，迁机用 `--restore`；测试一律用 staging 或 Pebble |
| **SQLite 写入竞争。** 生产今天 0 个节点；容量估算见附录 B。真正的风险不是吞吐，而是长写事务和多个包宿主进程争同一个文件 | P1 的批量更新、心跳节流、读写分离和基准；`busy_timeout` 提高到 10 秒；基准不达标时先查长事务，不改数据库 |
| **数据增长。** `v2_server_log` 今天无限增长 | P1 的小时合并和保留期；每晚备份前检查点 |
| **内置商业包。** 订单、支付、返佣代码进入每个公开发布；任何包的修复都要发一个 Control 版本 | 接受（S4、S5 已定）。源码本来就在同一仓库的 `packages/` 下 |
| **多入口程序改变插件内核契约**：宿主不再是单独签名的二进制 | 建议接受：第三方包本来就不能执行，导入也取消了，单独签名宿主只增加 0.5 GB 和一条信任链；以签名的 `SHA256SUMS.txt.sig` 代替。需要所有者确认 |
| **Agent 插件载荷和部署编排**（B23）：M2 到 M4 之间，四个 Agent 插件运行时和拓扑执行仍然不可用 | 维持今天生产的关闭状态；M4 随 Agent 发布清单恢复。需要所有者确认这三个预览运行时和部署编排是否还要保留（S7） |
| **回滚。** M4 之前没有一键回滚；旧程序会拒绝新库 | 升级前安装器自动备份并保留上一版程序；回滚 = `systemctl stop` → 换回 `previous/anix-control` → `backup restore --replace <pre-upgrade 包>` → 启动；备份之后写入的数据会丢，升级说明里写明。PostgreSQL 到 SQLite 的切换没有反向工具：在 O4 之前，回滚就是停新服务、启动旧服务（旧库在切换后未被写入） |
| **自签兜底期间的新面板。** 浏览器警告可能吓退新装的运维 | 横幅和安装器输出明确写原因和修复办法；领取链接在自签下仍可用 |
| **时区。** 业务日边界跟随主机时区，与旧面板的 PostgreSQL 会话时区不一定相同 | 切换步骤里先对齐时区；安装器打印当前时区 |
| **程序体积和前端体积** | `unknown`，P6 和 P11 实测并写进 PR |

**无法从代码确定、需要所有者回答的问题。**生产的各项数量和状态（附录 A）；生产机 80 和 443 的占用者、是否共用其他网站、域名是否套 CDN；生产配置里是否改过 `app.subscribe_path`、`grpc.port`、`agent_install.cn_mirror_url`（`sudo grep -nE 'subscribe_path|^grpc:|port:|cn_mirror_url|public_url' /opt/anixops/control/config/config.yaml`）；是否接受多入口程序（4.1）和 Agent 载荷推迟到 M4（B23）。

## 附录 A：生产库只读 SQL

在生产库上执行，会话设为只读（`default_transaction_read_only`），不修改任何数据。每条语句单独成事务，所以某张表不存在时只有那一条报错，记录“不存在”即可，后面的语句照常执行。

```sh
psql "$DSN" -v ON_ERROR_STOP=0 -X -f m2-readonly.sql > m2-readonly.out 2>&1
```

```sql
SET default_transaction_read_only = on;

-- 1. 版本和大小
SELECT version();
SELECT current_database(), pg_size_pretty(pg_database_size(current_database()));
SELECT n.nspname AS schema, c.relname AS table_name,
       pg_size_pretty(pg_total_relation_size(c.oid)) AS size
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY pg_total_relation_size(c.oid) DESC LIMIT 30;

-- 2. 每张表的准确行数
SELECT table_schema, table_name,
       (xpath('/row/c/text()',
              query_to_xml(format('SELECT count(*) AS c FROM %I.%I', table_schema, table_name),
                           false, true, '')))[1]::text::bigint AS row_count
FROM information_schema.tables
WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name;

-- 3. 用户、订单、支付、节点
SELECT count(*) AS users,
       count(*) FILTER (WHERE is_admin = 1) AS admins,
       count(*) FILTER (WHERE banned = 0 AND plan_id IS NOT NULL
                          AND (expired_at IS NULL OR expired_at > extract(epoch FROM now()))) AS active_subscribers
FROM v2_user;
SELECT status, count(*), sum(total_amount) FROM v2_order GROUP BY status ORDER BY status;
SELECT count(*) AS payment_records FROM v2_payment_record;
SELECT count(*) AS payments FROM v2_payment;
SELECT count(*) AS plans, count(*) FILTER (WHERE "show" = 1) AS on_sale FROM v2_plan;
SELECT count(*) AS tickets FROM v2_ticket;
SELECT count(*) AS proxy_nodes FROM v2_node;
SELECT count(*) AS forward_nodes FROM v2_forward_node;
SELECT count(*) AS wireguard_peers FROM v2_wireguard_peer;
SELECT count(*) AS node_groups FROM v5_node_group;

-- 4. 身份切换状态（无行表示 kernel）
SELECT state, import_id, updated_at FROM v4_kernel_identity_authority;
SELECT count(*) AS identity_signing_keys FROM pkg_identity_platform.signing_key;

-- 5. 节点密钥拆分阶段
SELECT table_name, phase, mismatches, verified_at, finalized_at FROM v4_kernel_node_secret_split;

-- 6. flux 表是否已删除
SELECT * FROM v4_forward_legacy_drop;
SELECT to_regclass('public.v2_forward') AS v2_forward_table;

-- 7. 包安装、存储的路由模式、远程运行时
SELECT plugin_id, target, desired_version, observed_version, state, enabled
FROM v3_kernel_plugin_installation ORDER BY plugin_id;
SELECT i.plugin_id, length(c.config_json) AS config_bytes
FROM v3_kernel_plugin_configuration c
JOIN v3_kernel_plugin_installation i ON i.id = c.installation_id
WHERE position('"routes"' IN c.config_json) > 0;
SELECT * FROM v4_kernel_plugin_runtime;
SELECT pg_size_pretty(sum(length(data))::bigint) AS artifact_bytes, count(*) FROM v3_kernel_plugin_artifact;

-- 8. 流量明细的规模（决定导入耗时和保留期）
SELECT count(*), min(to_timestamp(log_at)), max(to_timestamp(log_at)) FROM v2_server_log;

-- 9. 序列，导入后核对
SELECT schemaname, sequencename, last_value FROM pg_sequences ORDER BY schemaname, sequencename;

```

## 附录 B：容量估算

**假设。** 生产今天 0 个节点。心跳 20 秒一次（agent_control_server.go:28-31）；流量上报每节点每分钟一次（旧推送间隔 60 秒，server_service.go:638；Agent 流的窗口由 Agent 决定）；每个用户同一时刻只连一个节点；没有 Agent 插件（下发关闭，B23）。设 N 为节点数，A 为一分钟内有流量的用户数（一般远小于用户总数 U）。

**P1 之后的写入量。**

- 写事务：每节点每分钟约 1 次流量事务，加上心跳落库（P1 之后每 60 秒最多一次）约 1 次，再加传输记录和配置状态的少量 upsert，合计约 3N 次每分钟。
- 写入行数：流量事务对每个有流量的用户写 2 行（小时桶 upsert、用户 u/d），每分钟约 2A 行，加上每节点约 5 行。
- 固定开销：吊销表清理每 5 分钟 2 次，审计和后台操作按人工频率。

| 规模 | 写事务/秒 | 写入行/秒 | `v2_server_log` 每天新增行 |
|---|---|---|---|
| N = 10，A = 200 | 约 0.5 | 约 7 | 约 4800（按每小时 200 个不同用户估算：200 × 24） |
| N = 50，A = 1000 | 约 2.5 | 约 38 | 约 24000 |
| N = 200，A = 5000 | 约 10 | 约 180 | 约 120000 |

作为对照，P1 之前同样 N = 50、A = 1000 时，`v2_server_log` 每天新增约 144 万行（每分钟每用户一行）。

表中最大一档每秒约 10 个写事务，长事务（单个事务内逐用户更新）比事务数更值得关注，所以 P1 把逐用户 UPDATE 改为一条批量语句。每个事务持锁时间的目标是 p99 小于 50 ms。

**P1 实测（开发机的 ext4 虚拟磁盘，不是生产 VPS；`TMPDIR` 必须指向数据盘，tmpfs 没有 fsync）。** WAL、`synchronous=NORMAL` 下，单行 UPDATE 事务每秒约 24 000 至 32 000 个（P1 之前的 `synchronous=FULL` 为每秒约 320 至 350 个）。N = 200、A = 5000 时，一次完整上报（批次登记、小时桶 upsert、用户计数、节点计数、在线人数）串行每秒约 660 至 1300 次；N 个节点同时不停上报的饱和吞吐每秒约 400 至 1000 次，是表中最大一档的 40 倍以上，P1 之前在 N = 10 就有约 1% 的上报因 `database is locked` 失败，N = 50 时约 10%。一次上报持写锁的 p50 约 0.7 ms；p99 在 WAL 自动检查点把 fsync 放在提交路径上时为 12 至 47 ms，关闭自动检查点时为 1.3 ms，所以尾部全部来自检查点。命令：`TMPDIR=<数据盘目录> go test ./internal/service -run '^$' -bench 'TrafficReportSQLite|NodeLivenessWriteSQLite' -benchtime 3000x`。生产 VPS 的磁盘上用同一命令复测后再定是否把检查点移出提交路径。
