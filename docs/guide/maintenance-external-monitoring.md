# Control 外部可用性监测

核对日期：2026-09-22。当前只做公开产品能力核验，未创建外部账户、配置真实 URL 或发送消息，不能标记 Control 宕机双渠道告警已完成。

[UptimeRobot 官方价格与功能表](https://uptimerobot.com/pricing/) 当前显示：Free 为 50 monitors、5 分钟间隔，支持邮件；功能对比中 Telegram 所在行 Free 不支持，Solo 支持。Free 文案面向 hobby/non-profit，因此不能把免费方案直接当生产双渠道交付。Solo 10 monitors、60 秒间隔，页面月付显示 €10/月（年付折合 €9/月），提供邮件和 Telegram；最终币种、税费、额度以开通页面为准。

本次结论：免费优先条件下，UptimeRobot Free 不满足邮件 + Telegram 的独立通知要求；Solo 是待负责人选择/验证的付费候选，没有执行购买。HetrixTools 公开价格页本次访问返回 403，无法核实其当前套餐与通知额度，不作为已验证替代。

部署负责人上线前执行：

1. 在独立于 Control 所在主机的第三方平台配置真实 HTTPS `/health`，确认非 2xx 判定故障。
2. 配置负责人邮件与 Telegram 两个独立接收渠道，记录平台套餐、监测数量、间隔、月度消息额度/限流、告警阈值。
3. 在预发布维护窗口隔离 Control，记录首个失败、邮件和 Telegram 接收时间；确认恢复通知也到达。
4. 在受控部署文档保存监测 ID、测试证据、账单责任人和维护人员交接信息。不要提交真实 token、chat ID、邮箱或内部地址。

第三方只检测 Control 自身失联，Agent 多节点故障聚合仍由 Control 运维工单处理。Control 不可用时不能依赖 Control 自身后台发送宕机通知。
