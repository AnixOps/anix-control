# Roadmap

本文规定 AnixOps 未来的开发方向，覆盖 Control（本仓库）、Agent（anix-agent）、
自研网络内核与客户端（networkcore_anixops / anixops-net）、运维（anixops-ansible）
以及官网、博客与测速工具。闭源组件有内部的路线图，不写在这里。

更新：2026-11-07。当前稳定版本：Control / Agent v5.0.1。

## 方向原则

1. **轻量、简洁**：只做必要的检查，优先复用现有代码，不写过度防御的分支。删掉的
   兼容层和死代码不再加回来。
2. **只用自研内核**：Agent 的数据面只跑我们自己的内核（networkcore 的 anixcore），
   sing-box、xray、hysteria 已经移除，不再引入第三方代理内核。新协议、新能力先进内核，
   再由 Agent 下发、Control 管理。
3. **版本与兼容**：Control 和 Agent 保持相同的主次版本，补丁号各自独立；Agent 上报
   协议修订号，Control 按兼容范围检查（`docs/reference/agent-versioning.md`）。发版
   全部自动化，由工作流打 tag。
4. **运维即代码**：所有服务器操作都通过 anixops-ansible 的 playbook 和工作流执行，
   不在机器上临时手工改。生产由一台主机承载 Control 和主库，另有一台冷备。
5. **国内只做入口和中转**：国内机器只做入口和中转（内核级转发），代理服务和出口都放在
   境外节点。节点要尽量不暴露品牌与指纹。
6. **社区版不限量**：付费、订单、优惠券、推广佣金只放在商业版里；社区版不限制用户数和
   节点数。
7. **先有证据再标完成**：一项工作只有在代码、测试和 CI 证据都齐了之后才算完成；生产上的
   变更以真实环境的验证为准。

## 5.0.x：稳定线（下一个补丁 5.0.2）

| 方向 | 内容 | 涉及 |
| --- | --- | --- |
| Agent 稳定性 | 节点首次应用配置失败时，Agent 保持连接并上报错误，不再退出后反复重启（新装节点缺内核时会卡在这里） | Agent |
| 新节点内核 | 面板加"手动安装内核"：上传内核发布文件，只信任内置发布公钥，用于无法自动下载的新节点 | Control |
| 国内入口内核转发 | 代码已在 5.0.1；逐条线路把国内入口 / 中转切到 nftables 内核转发，每条线路由运维确认后再切 | Control / Agent / ansible |
| 防扫描收尾 | 节点上 ansible 步骤框架的日志和状态目录改为中性名称 | ansible |
| 节点密钥拆分收尾 | 已完成：生产 2026-11-08 完成 finalize；5.0.3 删除双读、回填、校验、finalize 和 unsplit 代码，未 finalize 的数据库拒绝启动 | Control |
| 订阅域名隔离 | 准备备用订阅域名，开启 `app.subscribe_domains_only`，把面板和 Agent gRPC 从被封风险高的域名迁走 | Control / 运维 |
| 访问统计 | 统计覆盖 UDP / QUIC / Mux，在新内核上核实上报并补测试 | 内核 / Agent |
| 路由校验 | 网络路由增加"同一入口端口冲突"检查 | Control |
| 离线安装包 | 每次发版后在生产主机生成 Agent 离线安装包，纳入发版流程 | ansible |

## 5.1：商业版与授权

- 公开仓库改为 AGPL-3.0 + CLA；社区版不限量。
- 商业版使用离线 license（Ed25519 签名 token）：用户数、节点数上限只在商业版二进制里
  生效，超限或过期只阻止新建，不影响已有用户和节点；永久授权加更新窗口。
- 后端的付费功能已经拆到私有仓库（覆盖构建 + 签名发布）；剩下的工作是把前端的商业
  页面（订单、套餐购买、优惠券、支付、推广）通过路由和菜单扩展点移出公开仓库。

## 5.x：中期方向

- **流量分类与 QoS**：已经有动态流量分类和"忙时让路、闲时满速"的下载优先级调度；
  下一步把 UDP 与多路复用纳入调度，并按真实节点数据调整默认值。
- **目标站探测**：自动选站与定期复查已经完成；下一步做按 ASN 发现邻近站点，以及让
  出口使用同机真实站点。
- **内核**：继续在 networkcore 补齐服务端协议和客户端能力（规则 / 策略组运行时、运行中
  切换节点、更多订阅格式、MITM 各道 gate），客户端直接引用固定的内核 commit。
- **出口策略**：出口默认拒绝回国流量，国内目标在入口直接出去；出口上一旦出现国内目标就
  计数并告警（说明入口或客户端的分流漏配了）。
- **插件化**：继续把 `/api/v2` 里的业务域迁进包（`docs/architecture/package-extraction.md`
  M3–M4），等 M4 的 Agent 发布清单出来，再打开 Agent 插件分发和拓扑执行。
- **CI 与发版**：控制 PR CI 的时长（尤其是内核仓库）；开发版通道（`.dev.N`）作为日常
  功能测试的入口。

## 远期：基础设施

- **Control 高可用**：现在是"单主 + 冷备（每小时备份、保留 14 天）"；之后视负载和需要
  再上热备库或多实例 active/active（设计已有，暂缓）。
- **备份**：数据库备份加一份异地副本，并定期做恢复演练。
- **环境分层**：在生产之外加一套 staging；发版流程固定为"签名制品 → staging → 人工
  批准 → 生产"，健康检查失败自动回滚。
- **监控**：外部的可用性监控，以及节点、线路的统一看板。

## 不做

- 不恢复 UniProxy、旧面板 API、节点 API key 这类已经删掉的兼容层。
- 不再引入第三方代理内核，也不再引入 gost 等外部转发程序。
- 不给社区版加用户数或节点数限制。

## 详细状态去哪看

| 问题 | 来源 |
|------|------|
| 插件化的设计与里程碑（M0–M4） | [`docs/architecture/package-extraction.md`](docs/architecture/package-extraction.md) |
| 当前的发版工作，以及怎么发版 | [`CHANGELOG.md`](CHANGELOG.md) 的 `## Unreleased`、[`changelog.d/`](changelog.d/README.md)、[`docs/RELEASING.md`](docs/RELEASING.md) |
| 已发布的阶段，以及明确没完成的部分 | [`docs/architecture/release-line-status.md`](docs/architecture/release-line-status.md) |
| 某个功能是已实现、部分实现、计划中还是推迟 | [`docs/features.md`](docs/features.md) |
| 具体还有哪些待办 | [`TODO.md`](TODO.md) |
| 改了什么、什么时候改的 | [`CHANGELOG.md`](CHANGELOG.md) |

每一项都适用的规则：

- 只有代码、测试和 CI 证据都齐了，一项才能标为完成。
- 产品版本 tag 跟随 CHANGELOG 和 [`docs/RELEASING.md`](docs/RELEASING.md)，与 Go
  模块的导入后缀（`/v4`）无关。
