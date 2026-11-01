package router

import (
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/health"
	"github.com/AnixOps/anix-control/v5/internal/middleware"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func registeredPackageRoute(gateway gin.HandlerFunc, packageID, routeID string, legacy gin.HandlerFunc) gin.HandlerFunc {
	packagebridge.DefaultRouteRegistry().MustRegister(packageID, routeID, legacy)
	return gateway
}

func registeredPackageWebSocketRoute(gateway gin.HandlerFunc, packageID, routeID string, legacy gin.HandlerFunc, preflight func(*gin.Context) bool) gin.HandlerFunc {
	packagebridge.DefaultRouteRegistry().MustRegisterWebSocket(packageID, routeID, legacy)
	if preflight == nil {
		return gateway
	}
	return func(c *gin.Context) {
		if preflight(c) {
			gateway(c)
		}
	}
}

// Setup 设置路由
func Setup(r *gin.Engine, cfg *config.Config) {
	subscribePath := config.PrepareSubscribePath(cfg)

	// 全局中间件
	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())

	// 速率限制器 (测试模式跳过)
	exemptPaths := []string{"/health", "/livez", "/readyz", "/metrics", "/swagger/"}
	adminLimiter := middleware.NewRateLimiter(10, 20,
		middleware.WithExemptPaths(exemptPaths),
		middleware.WithTTL(5*time.Minute),
	)
	userLimiter := middleware.NewRateLimiter(30, 60,
		middleware.WithExemptPaths(exemptPaths),
		middleware.WithTTL(5*time.Minute),
	)
	publicLimiter := middleware.NewRateLimiter(5, 10,
		middleware.WithExemptPaths(exemptPaths),
		middleware.WithTTL(5*time.Minute),
	)
	// 启动定期清理过期桶
	adminLimiter.StartCleanup(1 * time.Minute)
	userLimiter.StartCleanup(1 * time.Minute)
	publicLimiter.StartCleanup(1 * time.Minute)

	// Admin API tokens ("Authorization: Bearer anixadm_...", docs/reference/
	// admin-api-tokens.md) authenticate the administrator APIs in front of
	// JWTAuth: /api/v2/admin, /api/v2/admin/agent, /api/v3 and /api/v4. User
	// routes and the compatibility group below do not have it, so a token
	// is refused there like any other string that is not a JWT.
	adminAPIToken := middleware.AdminAPIToken()

	// Swagger API 文档
	// 自定义 handler 来正确处理 doc.json
	r.GET("/swagger/*any", func(c *gin.Context) {
		path := c.Param("any")
		if path == "/doc.json" || path == "doc.json" {
			// 直接返回静态 swagger.json 文件
			c.File("./docs/swagger.json")
			return
		}
		// 其他 swagger 文件使用 ginSwagger
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	})

	// 健康检查
	// @Summary 健康检查
	// @Description 检查服务是否正常运行
	// @Tags 系统
	// @Produce json
	// @Success 200 {object} map[string]string
	// @Router /health [get]
	// /livez and /readyz are the container probes; /health keeps its response
	// shape and fails while the server drains.
	r.GET("/livez", health.Default.Live)
	r.GET("/readyz", health.Default.Ready)
	r.GET("/health", health.Default.Health)

	// Prometheus 指标
	metricsHandler := handler.NewMetricsHandler()
	r.GET("/metrics", metricsHandler.GetMetrics)

	// 订阅接口 (公开，使用用户 token 认证)
	// 路径可通过配置 app.subscribe_path 自定义，默认为 "s"
	subscribeHandler := handler.NewSubscribeHandler(cfg)
	r.GET("/"+subscribePath+"/:token", subscribeHandler.GetSubscription)

	// One-command node onboarding (forward-sdk.md, section 9): the signed
	// install script, its signature, the release metadata it reads and the
	// Agent release when Control holds it. Public downloads, rate limited.
	agentInstall := handler.NewAgentInstallHandler()
	installPublic := r.Group("")
	installPublic.Use(publicLimiter.Middleware())
	installPublic.GET("/install.sh", agentInstall.Script)
	installPublic.GET("/install.sh.sig", agentInstall.Signature)
	installPublic.GET("/install/agent.env", agentInstall.Metadata)
	installPublic.GET("/install/agent/:tag/:asset", agentInstall.Artifact)
	installPublic.GET("/install/kernel/anx-node/:version/:file", agentInstall.Kernel)
	// The caller's address as Control sees it (server.trusted_proxies
	// applied): the script hands it to the join.
	installPublic.GET("/install/ip", agentInstall.ClientIP)

	// API v2
	v2 := r.Group("/api/v2")
	v2RouteCache := handler.NewV2RouteCache()
	v2PackageGateway := handler.NewV2CompatibilityGateway(cfg, v2RouteCache)
	v2WebSocketGateway := handler.NewV2WebSocketGateway(cfg, v2RouteCache)
	{
		// 认证接口 (无需登录) - 公开限流
		authPublic := v2.Group("")
		authPublic.Use(publicLimiter.Middleware())
		{
			authPublic.POST("/login", v2PackageGateway.Serve)
			authPublic.POST("/register", v2PackageGateway.Serve)
		}

		// 支付接口

		// 支付回调 (无需认证)

		// 需要登录的接口
		auth := v2.Group("")
		auth.Use(userLimiter.Middleware())
		auth.Use(middleware.JWTAuth())
		{
			// 用户接口
			userHandler := handler.NewUserHandler()
			auth.GET("/user/profile", v2PackageGateway.Serve)
			auth.GET("/user/dashboard", v2PackageGateway.Serve)
			// 用户订阅接口
			auth.GET("/user/subscription", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.user.subscription.get", userHandler.GetSubscription))

			// 知识库接口
			knowledgeHandler := handler.NewKnowledgeHandler()
			auth.GET("/user/knowledge", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.article.list", knowledgeHandler.GetArticles))
			auth.GET("/user/knowledge/:id", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.user.knowledge.id.get", knowledgeHandler.GetArticle))

			// 工单接口
			ticketHandler := handler.NewTicketHandler()
			auth.GET("/user/ticket", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.user.ticket.get", ticketHandler.GetTickets))
			auth.POST("/user/ticket", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.user.ticket.post", ticketHandler.CreateTicket))
			auth.GET("/user/ticket/:id", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.user.ticket.id.get", ticketHandler.GetTicket))
			auth.POST("/user/ticket/:id/reply", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.user.ticket.id.reply.post", ticketHandler.ReplyTicket))
			auth.POST("/user/ticket/:id/close", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.user.ticket.id.close.post", ticketHandler.CloseTicket))

			// 套餐接口
			planHandler := handler.NewUserPlanHandler()
			auth.GET("/user/plan", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.user.plan.get", planHandler.GetPlans))

			// 优惠券接口

			// 订单接口

			// 用户支付接口
		}

		// 管理员接口
		admin := v2.Group("/admin")
		admin.Use(adminLimiter.Middleware())
		admin.Use(adminAPIToken)
		admin.Use(middleware.JWTAuth())
		admin.Use(middleware.AdminAuth())
		admin.Use(middleware.AuditLog())
		{
			adminHandler := handler.NewAdminHandler()
			monitorWSHandler := handler.NewMonitorWSHandler()

			// ========== WebSocket 实时监控 ==========
			admin.GET("/ws/monitor", registeredPackageWebSocketRoute(v2WebSocketGateway.Serve, "machine-telemetry", "telemetry.admin.ws.monitor.get", monitorWSHandler.HandleMonitorWS, nil))

			// 仪表盘
			admin.GET("/dashboard", registeredPackageRoute(v2PackageGateway.Serve, "machine-telemetry", "telemetry.admin.dashboard.get", adminHandler.GetDashboard))
			admin.GET("/traffic/hourly", registeredPackageRoute(v2PackageGateway.Serve, "machine-telemetry", "telemetry.admin.traffic.hourly.get", adminHandler.GetHourlyTraffic))
			admin.GET("/traffic/user-ranking", registeredPackageRoute(v2PackageGateway.Serve, "machine-telemetry", "telemetry.admin.traffic.user_ranking.get", adminHandler.GetUserTrafficRanking))
			admin.GET("/system/info", registeredPackageRoute(v2PackageGateway.Serve, "machine-telemetry", "telemetry.admin.system.info.get", adminHandler.GetSystemInfo))

			// 邀请码管理 (注册邀请码，社区版保留)
			admin.GET("/invite/codes", v2PackageGateway.Serve)
			admin.POST("/invite/codes", v2PackageGateway.Serve)
			admin.DELETE("/invite/codes/:id", v2PackageGateway.Serve)

			// 用户管理
			admin.POST("/users", v2PackageGateway.Serve)
			admin.GET("/users", v2PackageGateway.Serve)
			admin.GET("/users/stats", v2PackageGateway.Serve)
			admin.GET("/users/:id", v2PackageGateway.Serve)
			admin.PUT("/users/:id", v2PackageGateway.Serve)
			admin.DELETE("/users/:id", v2PackageGateway.Serve)
			admin.POST("/users/:id/ban", v2PackageGateway.Serve)
			admin.POST("/users/:id/unban", v2PackageGateway.Serve)
			admin.POST("/users/:id/reset-traffic", v2PackageGateway.Serve)
			admin.POST("/users/:id/reset-subscribe", v2PackageGateway.Serve)

			// 订单管理

			// 节点管理 (新版)
			nodeHandler := handler.NewNodeHandler()
			admin.GET("/nodes", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.get", nodeHandler.GetNodes))
			admin.GET("/nodes/stats", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.stats.get", nodeHandler.GetNodeStats))
			admin.GET("/nodes/:id/logs", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.logs.get", nodeHandler.GetNodeLogs))
			admin.POST("/nodes", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.post", nodeHandler.CreateNode))
			admin.GET("/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.get", nodeHandler.GetNode))
			admin.PUT("/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.put", nodeHandler.UpdateNode))
			admin.DELETE("/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.delete", nodeHandler.DeleteNode))
			admin.POST("/nodes/:id/sync", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.sync.post", nodeHandler.SyncProtocol))
			admin.GET("/nodes/:id/agent-control", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.agent_control.get", nodeHandler.GetAgentControlStatus))
			admin.POST("/nodes/:id/agent-control/operations", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.agent_control.operations.post", nodeHandler.DispatchAgentControlOperation))

			// 节点高级配置 (RawConfig - 直接JSON编辑)
			admin.GET("/nodes/:id/raw-config", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.raw_config.get", nodeHandler.GetNodeRawConfig))
			admin.PUT("/nodes/:id/raw-config", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.raw_config.put", nodeHandler.UpdateNodeRawConfig))
			admin.POST("/nodes/validate-config", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.validate_config.post", nodeHandler.ValidateRawConfig))

			// 节点协议管理
			admin.GET("/nodes/:id/protocols", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.protocols.get", nodeHandler.GetProtocols))
			admin.POST("/nodes/:id/protocols", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.protocols.post", nodeHandler.CreateProtocol))
			admin.PUT("/nodes/:id/protocols/:protocol_id", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.protocols.protocol_id.put", nodeHandler.UpdateProtocol))
			admin.DELETE("/nodes/:id/protocols/:protocol_id", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.nodes.id.protocols.protocol_id.delete", nodeHandler.DeleteProtocol))
			admin.GET("/protocol-templates", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.protocol_templates.get", nodeHandler.GetProtocolTemplates))
			admin.POST("/wireguard/keypair", registeredPackageRoute(v2PackageGateway.Serve, "wireguard", "wireguard.admin.wireguard.keypair.post", nodeHandler.GenerateWireGuardKeypair))

			// 订阅分组和模板管理
			subAdmin := handler.NewSubscriptionAdminHandler()

			// 订阅分组
			admin.GET("/subscription/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.get", subAdmin.GetGroups))
			admin.POST("/subscription/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.post", subAdmin.CreateGroup))
			admin.GET("/subscription/groups/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.get", subAdmin.GetGroup))
			admin.PUT("/subscription/groups/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.put", subAdmin.UpdateGroup))
			admin.DELETE("/subscription/groups/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.delete", subAdmin.DeleteGroup))

			// 订阅模板
			admin.GET("/subscription/groups/:id/templates", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.templates.get", subAdmin.GetTemplates))
			admin.GET("/subscription/groups/:id/protocols", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.protocols.get", subAdmin.GetGroupProtocols))
			admin.POST("/subscription/groups/:id/protocols", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.protocols.post", subAdmin.UpdateGroupProtocols))
			admin.POST("/subscription/groups/:id/templates", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.groups.id.templates.post", subAdmin.CreateTemplate))
			admin.GET("/subscription/templates/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.templates.id.get", subAdmin.GetTemplate))
			admin.PUT("/subscription/templates/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.templates.id.put", subAdmin.UpdateTemplate))
			admin.DELETE("/subscription/templates/:id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.templates.id.delete", subAdmin.DeleteTemplate))

			// 用户订阅分组关联
			admin.GET("/subscription/users/:user_id/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.users.user_id.groups.get", subAdmin.GetUserGroups))
			admin.POST("/subscription/users/:user_id/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.users.user_id.groups.post", subAdmin.AssignGroupToUser))
			admin.DELETE("/subscription/users/:user_id/groups/:group_id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.users.user_id.groups.group_id.delete", subAdmin.RemoveGroupFromUser))

			// 套餐订阅分组关联
			admin.GET("/subscription/plans/:plan_id/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.plans.plan_id.groups.get", subAdmin.GetPlanGroups))
			admin.POST("/subscription/plans/:plan_id/groups", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.plans.plan_id.groups.post", subAdmin.AssignGroupToPlan))
			admin.DELETE("/subscription/plans/:plan_id/groups/:group_id", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.plans.plan_id.groups.group_id.delete", subAdmin.RemoveGroupFromPlan))

			// 订阅工具接口
			admin.GET("/subscription/formats", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.formats.get", subAdmin.GetSubscriptionFormats))
			admin.GET("/subscription/protocols", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.protocols.get", subAdmin.GetProtocolTypes))
			admin.GET("/subscription/protocols/available", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.protocols.available.get", subAdmin.GetAvailableProtocols))
			admin.POST("/subscription/preview", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.preview.post", subAdmin.PreviewSubscription))
			admin.GET("/subscription/stats", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.subscription.stats.get", subAdmin.GetGroupStats))

			// 套餐管理
			admin.GET("/plans", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.get", adminHandler.GetPlans))
			admin.POST("/plans", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.post", adminHandler.CreatePlan))
			admin.GET("/plans/:id", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.id.get", adminHandler.GetPlan))
			admin.PUT("/plans/:id", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.id.put", adminHandler.UpdatePlan))
			admin.DELETE("/plans/:id", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.id.delete", adminHandler.DeletePlan))
			admin.POST("/plans/:id/assign", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.admin.plans.id.assign.post", adminHandler.AssignPlanToUser))

			// 工单管理 (V2 Stub)
			ticketHandler := handler.NewAdminTicketHandler()
			admin.GET("/ticket", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.admin.ticket.get", ticketHandler.GetTickets))
			admin.POST("/ticket/reply", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.admin.ticket.reply.post", ticketHandler.ReplyTicket))
			admin.POST("/ticket/:id/close", registeredPackageRoute(v2PackageGateway.Serve, "ticket", "ticket.admin.ticket.id.close.post", ticketHandler.CloseTicket))

			// 优惠券管理 (V2 Stub)

			// 知识库管理 (V2 Stub)
			knowledgeHandler := handler.NewAdminKnowledgeHandler()
			admin.GET("/knowledge", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.get", knowledgeHandler.GetArticles))
			admin.POST("/knowledge", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.post", knowledgeHandler.CreateArticle))
			admin.PUT("/knowledge/:id", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.id.put", knowledgeHandler.UpdateArticle))
			admin.DELETE("/knowledge/:id", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.id.delete", knowledgeHandler.DeleteArticle))

			// ========== 支付网关管理 ==========

			// 网关管理

			// 支付统计和记录

			// ========== Telegram Bot 管理 ==========
			telegramHandler := handler.NewTelegramHandler()

			admin.GET("/telegram/bot", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.bot.get", telegramHandler.GetBot))
			admin.PUT("/telegram/bot", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.bot.put", telegramHandler.UpdateBot))
			admin.POST("/telegram/webhook", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.webhook.post", telegramHandler.SetWebhook))
			admin.DELETE("/telegram/webhook", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.webhook.delete", telegramHandler.DeleteWebhook))
			admin.POST("/telegram/notify", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.notify.post", telegramHandler.SendNotification))
			admin.POST("/telegram/broadcast", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.broadcast.post", telegramHandler.Broadcast))
			admin.GET("/telegram/users", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.users.get", telegramHandler.GetUserBindings))
			admin.PUT("/telegram/users/:id/notify", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.telegram.users.id.notify.put", telegramHandler.UpdateUserNotify))

			// ========== MFA 管理 ==========
			admin.GET("/mfa/config", v2PackageGateway.Serve)
			admin.PUT("/mfa/config", v2PackageGateway.Serve)

			// ========== 邀请码管理（注册控制，所有版本） ==========

			// ========== 通知管理 ==========
			notificationHandler := handler.NewNotificationHandler()
			admin.GET("/notification/templates", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.templates.get", notificationHandler.ListTemplates))
			admin.POST("/notification/templates", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.templates.post", notificationHandler.CreateTemplate))
			admin.PUT("/notification/templates/:id", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.templates.id.put", notificationHandler.UpdateTemplate))
			admin.DELETE("/notification/templates/:id", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.templates.id.delete", notificationHandler.DeleteTemplate))
			admin.GET("/notification/logs", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.logs.get", notificationHandler.ListLogs))
			admin.POST("/notification/test", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.test.post", notificationHandler.SendTestNotification))
			admin.GET("/notification/email/config", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.email.config.get", notificationHandler.GetEmailConfig))
			admin.PUT("/notification/email/config", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.admin.notification.email.config.put", notificationHandler.UpdateEmailConfig))

			// ========== 邀请返利管理 ==========

			// ========== 系统配置管理 ==========
			systemHandler := handler.NewSystemHandler()
			admin.GET("/system/configs", registeredPackageRoute(v2PackageGateway.Serve, "platform", "platform.admin.system.configs.get", systemHandler.GetConfigs))
			admin.GET("/system/configs/:key", registeredPackageRoute(v2PackageGateway.Serve, "platform", "platform.admin.system.configs.key.get", systemHandler.GetConfig))
			admin.GET("/system/subscription-settings", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.system.subscription_settings.get", systemHandler.GetSubscriptionSettings))
			admin.PUT("/system/configs/:key", registeredPackageRoute(v2PackageGateway.Serve, "platform", "platform.admin.system.configs.key.put", systemHandler.SetConfig))
			admin.DELETE("/system/configs/:key", registeredPackageRoute(v2PackageGateway.Serve, "platform", "platform.admin.system.configs.key.delete", systemHandler.DeleteConfig))
			admin.GET("/system/audit-logs", registeredPackageRoute(v2PackageGateway.Serve, "platform", "platform.admin.system.audit_logs.get", systemHandler.GetAuditLogs))

			// The backup routes (/system/backup*, /system/backups*) are gone:
			// their schedule, retention and S3 settings ran nothing and the
			// online restore overwrote the open database. Backups are
			// /api/v5/backups; a restore runs on the server
			// (`anix-control backup restore`).

			// ========== 负载均衡管理 ==========
		}

		// 用户接口 (需认证)
		authUser := v2.Group("")
		authUser.Use(userLimiter.Middleware())
		authUser.Use(middleware.JWTAuth())
		{
			adminCompat := authUser.Group("")
			adminCompat.Use(middleware.AdminAuth())
			{
				adminCompat.POST("/user/reset", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.user.reset.post", handler.NewAdminHandler().ResetCompatFlow))
			}

			// 用户支付

			// 用户 Telegram
			telegramHandler := handler.NewTelegramHandler()
			authUser.GET("/user/telegram/status", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.telegram.status.get", telegramHandler.GetTelegramStatus))
			authUser.POST("/user/telegram/unbind", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.telegram.unbind.post", telegramHandler.UnbindTelegram))
			authUser.POST("/user/telegram/notify", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.telegram.notify.post", telegramHandler.UpdateNotifySettings))

			// 用户 MFA
			authUser.GET("/user/mfa/status", v2PackageGateway.Serve)
			authUser.POST("/user/mfa/totp/setup", v2PackageGateway.Serve)
			authUser.POST("/user/mfa/totp/enable", v2PackageGateway.Serve)
			authUser.POST("/user/mfa/disable", v2PackageGateway.Serve)
			authUser.POST("/user/mfa/verify", v2PackageGateway.Serve)
			authUser.POST("/user/mfa/backup-codes/regenerate", v2PackageGateway.Serve)

			// 用户自助重置订阅链接（需重新验证身份，所有版本）
			authUser.POST("/user/subscription/reset", middleware.AuditLog(), v2PackageGateway.Serve)

			// 用户通知
			notificationHandler := handler.NewNotificationHandler()
			authUser.GET("/user/notifications", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.get", notificationHandler.GetUserNotifications))
			authUser.GET("/user/notifications/unread-count", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.unread_count.get", notificationHandler.GetUnreadCount))
			authUser.POST("/user/notifications/:id/read", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.id.read.post", notificationHandler.MarkAsRead))
			authUser.POST("/user/notifications/read-all", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.read_all.post", notificationHandler.MarkAllAsRead))

			// 用户邀请
		}

		// Telegram Webhook (公开)
		telegramHandler := handler.NewTelegramHandler()
		tgWebhook := v2.Group("")
		tgWebhook.Use(publicLimiter.Middleware())
		{
			tgWebhook.POST("/telegram/webhook", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.telegram.webhook.post", telegramHandler.TelegramWebhook))
		}

		agentHandler := handler.NewAgentHandler()

		// Agent 管理接口 (管理员)
		agentAdmin := v2.Group("/admin/agent")
		agentAdmin.Use(adminLimiter.Middleware())
		agentAdmin.Use(adminAPIToken)
		agentAdmin.Use(middleware.JWTAuth())
		agentAdmin.Use(middleware.AdminAuth())
		{
			agentAdmin.GET("/list", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.list.get", agentHandler.ListAgents))
			agentAdmin.POST("/tasks", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.tasks.post", agentHandler.CreateTask))
			agentAdmin.GET("/tasks", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.tasks.get", agentHandler.ListDiagnosticTasks))
			agentAdmin.GET("/tasks/:task_id", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.tasks.task_id.get", agentHandler.GetTaskResult))
			agentAdmin.POST("/execute", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.execute.post", agentHandler.ExecuteCommand))
			agentAdmin.GET("/monitor", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.admin.agent.monitor.get", agentHandler.GetMonitor))
		}
	}

	// API v3 is the control-kernel surface. Legacy business APIs remain under
	// /api/v2 while services are migrated behind plugin compatibility adapters.
	v3 := r.Group("/api/v3")
	v3.Use(adminLimiter.Middleware())
	v3.Use(adminAPIToken)
	v3.Use(middleware.JWTAuth())
	v3.Use(middleware.AdminAuth())
	v3.Use(middleware.AuditLog())
	{
		kernel := handler.NewKernelHandler()
		// The built-in packages: Control installs, upgrades and enables them
		// at every start, so there is no import, install, update, rollback,
		// enable or disable route; an installation's only action is restart
		// (single-binary design, section 4.3).
		v3.GET("/extensions", kernel.ListExtensions)
		v3.GET("/extensions/:plugin_id/:version/webui/:sha256/:filename", kernel.ServePluginWebUIAsset)
		v3.GET("/plugins", kernel.ListPlugins)
		v3.Any("/plugins/:plugin_id/*route", kernel.PluginRouteGateway)
		v3.GET("/plugin-releases", kernel.ListPluginReleases)
		v3.GET("/plugin-releases/:id/artifact", kernel.GetPluginReleaseArtifact)
		v3.GET("/plugin-installations", kernel.ListPluginInstallations)
		v3.POST("/plugin-installations/:id/actions", kernel.PluginInstallationAction)
		v3.GET("/plugin-installations/:id/config", kernel.GetPluginInstallationConfiguration)
		v3.PUT("/plugin-installations/:id/config", kernel.UpdatePluginInstallationConfiguration)

		v3.GET("/service-scopes", kernel.ListServiceScopes)
		v3.GET("/access-groups", kernel.ListAccessGroups)
		v3.POST("/access-groups", kernel.CreateAccessGroup)
		v3.GET("/access-groups/resolve", kernel.ResolveAccess)
		v3.GET("/access-groups/:id", kernel.GetAccessGroupDetail)
		v3.PUT("/access-groups/:id", kernel.UpdateAccessGroup)
		v3.DELETE("/access-groups/:id", kernel.DeleteAccessGroup)
		v3.POST("/access-groups/:id/users", kernel.AddGroupUser)
		v3.DELETE("/access-groups/:id/users/:user_id", kernel.RemoveGroupUser)
		v3.POST("/access-groups/:id/plans", kernel.AddGroupPlan)
		v3.DELETE("/access-groups/:id/plans/:plan_id", kernel.RemoveGroupPlan)
		v3.GET("/resource-grants", kernel.ListResourceGrants)
		v3.POST("/resource-grants", kernel.CreateResourceGrant)
		v3.DELETE("/resource-grants/:id", kernel.DeleteResourceGrant)
		v3.GET("/quota-policies", kernel.ListQuotaPolicies)
		v3.PUT("/quota-policies", kernel.UpsertQuotaPolicy)
		v3.DELETE("/quota-policies/:id", kernel.DeleteQuotaPolicy)

		v3.GET("/nodes/:id/plugins", kernel.ListAssignments)
		v3.GET("/nodes/:id/assignments", kernel.ListAssignments)
		v3.PUT("/nodes/:id/assignments", kernel.UpsertAssignment)
		v3.DELETE("/nodes/:id/assignments/:assignment_id", kernel.DeleteAssignment)

		v3.GET("/operations", kernel.ListOperations)
		v3.POST("/operations", kernel.CreateOperation)
		v3.POST("/operations/:id/cancel", kernel.CancelOperation)
	}

	// API v4 exposes the package route gateway and module and agent PKI
	// administration.
	// Its admission model is identical to v3.
	// Public: the identity token keys Control accepts (JWKS).
	identityPublic := r.Group("/api/v4/identity")
	identityPublic.Use(publicLimiter.Middleware())
	identityPublic.GET("/jwks.json", handler.IdentityJWKS)
	// Public: the registration settings the web app reads first.
	publicConfig := r.Group("/api/v4/public")
	publicConfig.Use(publicLimiter.Middleware())
	publicConfig.GET("/config", handler.PublicConfigHandler(cfg))
	identityUser := r.Group("/api/v4/identity")
	identityUser.Use(userLimiter.Middleware(), middleware.JWTAuth())
	identityUser.POST("/logout", handler.IdentityLogout)
	// The signed-in user's own traffic by destination
	// (docs/reference/destination-stats.md).
	destinationStats := handler.NewDestinationStatsHandler()
	userV4 := r.Group("/api/v4/user")
	userV4.Use(userLimiter.Middleware(), middleware.JWTAuth())
	userV4.GET("/traffic/destinations", destinationStats.UserSummary)
	// The signed-in member's Telegram binding: status, a one-time bind link
	// to the bot, and unbind.
	telegramBindings := handler.NewTelegramBindingHandler()
	userV4.GET("/telegram", telegramBindings.UserStatus)
	userV4.POST("/telegram/bind-link", telegramBindings.UserBindLink)
	userV4.DELETE("/telegram", telegramBindings.UserUnbind)

	v4 := r.Group("/api/v4")
	v4.Use(adminLimiter.Middleware())
	v4.Use(adminAPIToken)
	v4.Use(middleware.JWTAuth())
	v4.Use(middleware.AdminAuth())
	v4.Use(middleware.AuditLog())
	{
		kernel := handler.NewKernelHandler()
		v4.Any("/plugins/:plugin_id/*route", kernel.PluginRouteGateway)
		// The forward package's v4 administrator API (forward-sdk.md F5a),
		// served as its control route /api/v4/plugins/forward/*.
		v4.Any("/forward/*route", kernel.ForwardGateway)
		modules := handler.NewModuleHandler()
		v4.POST("/kernel/modules/enrollment-tokens", modules.CreateEnrollment)
		v4.GET("/kernel/modules/enrollment-tokens", modules.ListEnrollments)
		v4.DELETE("/kernel/modules/enrollment-tokens/:id", modules.RevokeEnrollment)
		v4.POST("/kernel/modules/ca/rotate", modules.RotateCA)
		v4.GET("/kernel/modules/trust-bundle", modules.TrustBundle)
		v4.GET("/kernel/identity", handler.IdentityStatus)
		v4.GET("/kernel/features", handler.ListFeatures)
		v4.POST("/kernel/identity/import", handler.StartIdentityImport)
		v4.POST("/kernel/identity/cutover", handler.StartIdentityCutover)
		v4.POST("/kernel/identity/rollback", handler.StartIdentityRollback)
		v4.POST("/kernel/identity/finalize", handler.FinalizeIdentity)
		v4.GET("/kernel/modules/runtimes", modules.ListRuntimes)
		v4.PUT("/kernel/modules/runtimes/:plugin_id", modules.SetRuntime)
		// The console's administrator API that the kernel owns, under
		// /api/v4/admin and outside the /api/v2 package catalog: when each user
		// was last online, the members of a subscription group, and the bulk
		// actions of the tables. A bulk action repeats the single-item /api/v2
		// route for each id through the same package gateway, so each item
		// runs wherever its route's mode puts it.
		v4.GET("/admin/users/activity", handler.NewAdminUserActivityHandler().List)
		// The console's node list: GET /api/v2/admin/nodes with each node's
		// approval and node group (its /api/v2 shape is a contract).
		v4.GET("/admin/nodes", handler.NewAdminNodesHandler().List)
		groupMembers := handler.NewAdminGroupMembersHandler()
		v4.GET("/admin/subscription-groups/:id/members", groupMembers.List)
		// A group's node protocols as subscribers see them: their order and
		// display names (model.SubscriptionGroupProtocol).
		v4.GET("/admin/subscription-groups/:id/protocol-layout", groupMembers.ProtocolLayout)
		v4.PUT("/admin/subscription-groups/:id/protocol-layout", groupMembers.ReplaceProtocolLayout)
		// Edits a group makes to the nodes a protocol produces, kept apart from
		// the protocol so they can be reset.
		v4.GET("/admin/subscription-groups/:id/protocols/:protocol_id/nodes", groupMembers.ProtocolNodes)
		v4.PUT("/admin/subscription-groups/:id/protocols/:protocol_id/nodes", groupMembers.ReplaceProtocolNodes)
		v4.DELETE("/admin/subscription-groups/:id/node-overrides", groupMembers.ResetNodeOverrides)
		// Each user's traffic by destination and category, and its settings.
		v4.GET("/admin/traffic/destinations", destinationStats.AdminSummary)
		v4.GET("/admin/traffic/destinations/settings", destinationStats.GetSettings)
		v4.PUT("/admin/traffic/destinations/settings", destinationStats.PutSettings)
		// The rules the categories are judged by: the built-in ones, the
		// administrator's own and the remote lists.
		trafficCategories := handler.NewTrafficCategoriesHandler()
		v4.GET("/admin/traffic/categories", trafficCategories.List)
		v4.GET("/admin/traffic/categories/classify", trafficCategories.Classify)
		v4.POST("/admin/traffic/categories/rules", trafficCategories.CreateRule)
		v4.PUT("/admin/traffic/categories/rules/:id", trafficCategories.UpdateRule)
		v4.DELETE("/admin/traffic/categories/rules/:id", trafficCategories.DeleteRule)
		v4.POST("/admin/traffic/categories/sources", trafficCategories.CreateSource)
		v4.PUT("/admin/traffic/categories/sources/:id", trafficCategories.UpdateSource)
		v4.DELETE("/admin/traffic/categories/sources/:id", trafficCategories.DeleteSource)
		v4.POST("/admin/traffic/categories/sources/:id/refresh", trafficCategories.RefreshSource)
		// The traffic priorities and per-user class caps the proxy nodes get
		// in their configuration (internal/trafficqos).
		v4.GET("/admin/traffic/qos", trafficCategories.GetQoS)
		v4.PUT("/admin/traffic/qos", trafficCategories.PutQoS)
		bulk := handler.NewAdminBulkHandler(handler.GatewayBulkRunner(v2PackageGateway.Serve))
		v4.POST("/admin/users/bulk", bulk.Users)
		v4.POST("/admin/invite-codes/bulk", bulk.InviteCodes)
		v4.GET("/kernel/node-operations", handler.NewKernelNodeOperationsHandler().List)
		// A proxy node's traffic series for the admin charts.
		v4.GET("/kernel/nodes/:id/traffic", handler.NewNodeTrafficHandler().Get)
		// Certificate-expiry and phase alerts (internal/kernelalerts).
		v4.GET("/kernel/alerts", handler.NewKernelAlertsHandler().List)
		// Personal access tokens for automation: managed from a signed-in
		// session only (the middleware refuses API tokens on this prefix).
		apiTokens := handler.NewAdminAPITokensHandler()
		v4.POST("/kernel/api-tokens", apiTokens.Create)
		v4.GET("/kernel/api-tokens", apiTokens.List)
		v4.DELETE("/kernel/api-tokens/:id", apiTokens.Revoke)
		// The administrator's Telegram test: one fixed message through the
		// configured bot, answered as a result class (never the token).
		notifications := handler.NewKernelNotificationsHandler()
		v4.POST("/kernel/notifications/telegram/test", notifications.TelegramTest)
		// An administrator binds a member to a Telegram account by hand.
		v4.POST("/admin/telegram/bindings", telegramBindings.AdminBind)
		v4.DELETE("/admin/telegram/bindings/:user_id", telegramBindings.AdminUnbind)
		agents := handler.NewAgentPKIHandler()
		v4.POST("/kernel/agents/enrollment-tokens", agents.CreateEnrollmentToken)
		v4.POST("/kernel/agents/install-tokens", agentInstall.CreateInstallToken)
		// What an install command needs from this Control, checked without
		// a network call (any administrator).
		v4.GET("/kernel/agents/install-readiness", agentInstall.Readiness)
		v4.POST("/kernel/agents/rotate-credentials", agents.RotateCredentials)
		v4.GET("/kernel/agents/transports", handler.NewAgentTransportsHandler().List)
		// Node groups (5.0 node onboarding): a group's join token makes any
		// machine that runs its install command a node of the group. The
		// command (it shows the token) and the token reset are for super
		// administrators.
		nodeGroups := handler.NewNodeGroupsHandler(agentInstall)
		v4.GET("/kernel/node-groups", nodeGroups.List)
		v4.POST("/kernel/node-groups", nodeGroups.Create)
		// The first group of a panel needs no input: the join dialog asks
		// for this one (the default group, created once).
		v4.POST("/kernel/node-groups/default", nodeGroups.EnsureDefault)
		v4.GET("/kernel/node-groups/:id", nodeGroups.Get)
		v4.PUT("/kernel/node-groups/:id", nodeGroups.Update)
		v4.DELETE("/kernel/node-groups/:id", nodeGroups.Delete)
		v4.GET("/kernel/node-groups/:id/command", nodeGroups.Command)
		v4.GET("/kernel/node-groups/:id/nodes", nodeGroups.Nodes)
		v4.POST("/kernel/node-groups/:id/reset-token", nodeGroups.ResetToken)
		// Egress routes: domains a proxy node's users open that leave
		// through another proxy node (internal/egressroute).
		egressRoutes := handler.NewEgressRoutesHandler()
		v4.GET("/kernel/egress-routes", egressRoutes.List)
		v4.POST("/kernel/egress-routes", egressRoutes.Create)
		v4.PUT("/kernel/egress-routes/:id", egressRoutes.Update)
		v4.DELETE("/kernel/egress-routes/:id", egressRoutes.Delete)
		// :id is the node, proxy-<id> or forward-<id> (the segment shares
		// its wildcard name with /kernel/nodes/:id/traffic).
		v4.POST("/kernel/nodes/:id/approve", nodeGroups.ApproveNode)
		v4.POST("/kernel/nodes/:id/reject", nodeGroups.RejectNode)
		// Node diagnostics (5.0 node page, 诊断): whitelisted actions of the
		// Agent's agent.diagnostic operation run on the node's Agent Control
		// stream, answered in the request; a restart needs a super
		// administrator, every call is in the operation log.
		nodeDiagnostics := handler.NewNodeDiagnosticsHandler()
		v4.POST("/kernel/nodes/:id/diagnostics", nodeDiagnostics.Run)
		v4.GET("/kernel/nodes/:id/diagnostics", nodeDiagnostics.List)
		// Node entries: the addresses subscribers reach a proxy node at
		// beyond its host (several lines, native IPv6, provider NAT ports;
		// internal/nodeentry).
		// One machine is one node: a forward node becomes a regular node
		// without protocols (docs/design/node-unification).
		nodeEntries := handler.NewNodeEntriesHandler()
		v4.GET("/kernel/nodes/:id/entries", nodeEntries.List)
		v4.PUT("/kernel/nodes/:id/entries", nodeEntries.Replace)
		// The device network model (docs/design/network-model): lines,
		// ingresses, egresses, ANX-Line transports and paths as one
		// document, planned into each device's NetworkPlan
		// (internal/networkmodel).
		networkModel := handler.NewNetworkModelHandler()
		v4.GET("/kernel/network", networkModel.Get)
		v4.PUT("/kernel/network", networkModel.Replace)
		v4.POST("/kernel/network/transports/:id/rotate-key", networkModel.RotateLinkKey)
		v4.GET("/kernel/network/devices/:id/plan", networkModel.DevicePlan)
		v4.GET("/kernel/nodes/:id/network", networkModel.NodeNetwork)
		v4.PUT("/kernel/nodes/:id/network", networkModel.SetNodeNetwork)
		// Panel HTTP(S) reverse proxies: sites by host, routes by path to
		// upstreams, run on entry nodes as NetworkPlan reverse proxies
		// (internal/reverseproxy).
		reverseProxies := handler.NewReverseProxiesHandler()
		v4.GET("/kernel/reverse-proxies", reverseProxies.List)
		v4.POST("/kernel/reverse-proxies", reverseProxies.Create)
		v4.PUT("/kernel/reverse-proxies/:id", reverseProxies.Update)
		v4.DELETE("/kernel/reverse-proxies/:id", reverseProxies.Delete)
		// Panabit iWAN lines: third-party iWAN ingresses AnixOps Net logs
		// into with a shared account to reach proxy nodes through them
		// (node entries of kind iwan; internal/iwanline).
		iwanLines := handler.NewIWANLinesHandler()
		v4.GET("/kernel/iwan-lines", iwanLines.List)
		v4.POST("/kernel/iwan-lines", iwanLines.Create)
		v4.PUT("/kernel/iwan-lines/:id", iwanLines.Update)
		v4.DELETE("/kernel/iwan-lines/:id", iwanLines.Delete)
		// Forward routes in subscriptions (加入订阅, Phase 10): a route whose
		// target is a proxy node's protocol is offered again at its entries
		// (internal/routesubscription).
		routeSubscriptions := handler.NewRouteSubscriptionHandler()
		v4.GET("/kernel/forward-routes/:id/subscription", routeSubscriptions.Get)
		v4.PUT("/kernel/forward-routes/:id/subscription", routeSubscriptions.Set)
		// Reality target probing (目标站探测): a proxy node's Agent probes
		// candidate REALITY targets from its network (reality.probe);
		// Control ranks them and recommends one; apply writes one into a
		// Reality protocol of the node; auto has the singleton worker
		// recheck the node and replace a target that keeps failing.
		realityTargets := handler.NewRealityTargetsHandler()
		v4.GET("/kernel/nodes/:id/reality-targets", realityTargets.List)
		v4.POST("/kernel/nodes/:id/reality-targets/probe", realityTargets.Probe)
		v4.POST("/kernel/nodes/:id/reality-targets/apply", realityTargets.Apply)
		v4.PUT("/kernel/nodes/:id/reality-targets/auto", realityTargets.SetAuto)
		// Staged Agent upgrades (forward-sdk.md section 9, O4; H19).
		upgrades := handler.NewAgentUpgradesHandler()
		v4.GET("/kernel/agents/upgrade-plan", upgrades.Plan)
		v4.POST("/kernel/agents/upgrades", upgrades.Start)
		v4.GET("/kernel/agents/upgrades", upgrades.List)
		v4.GET("/kernel/agents/upgrades/:id", upgrades.Get)
		v4.POST("/kernel/agents/upgrades/:id/pause", upgrades.Pause)
		v4.POST("/kernel/agents/upgrades/:id/resume", upgrades.Resume)
		v4.POST("/kernel/agents/upgrades/:id/abort", upgrades.Abort)
	}

	// The first administrator's claim link (adminclaim, N10): public, at the
	// public limiter's rate, which also applies in gin's test mode. The body
	// (the token and a password) is never logged: the access log records no
	// body, the audit log never captures it (middleware
	// bodyNeverCapturedPaths), and the handler writes the one audit row.
	claimLimiter := middleware.NewRateLimiter(5, 10,
		middleware.WithTTL(5*time.Minute),
		middleware.WithTestModeBypass(false),
	)
	claimLimiter.StartCleanup(1 * time.Minute)
	claimHandler := handler.NewClaimHandler()
	claim := r.Group("/api/v5/claim")
	claim.Use(claimLimiter.Middleware())
	{
		claim.GET("/status", claimHandler.Status)
		claim.POST("", claimHandler.Claim)
	}

	// API v5: the panel's own state, with v4's admission model. The
	// certificates Control serves (the console's banner reads them) and a
	// retry of the failing ones; the backups (list, back up now, and the
	// download, which needs a super administrator and is audited). There
	// is no restore route: a restore runs on the server with Control
	// stopped.
	v5 := r.Group("/api/v5")
	v5.Use(adminLimiter.Middleware())
	v5.Use(adminAPIToken)
	v5.Use(middleware.JWTAuth())
	v5.Use(middleware.AdminAuth())
	v5.Use(middleware.AuditLog())
	{
		panelStatus := handler.NewPanelStatusHandler()
		v5.GET("/panel/status", panelStatus.Status)
		v5.POST("/panel/certificates/retry", panelStatus.RetryCertificates)
		v5.GET("/settings/catalog", handler.NewSettingsCatalogHandler().List)
		backups := handler.NewBackupsHandler()
		v5.GET("/backups", backups.List)
		v5.POST("/backups", backups.Create)
		v5.GET("/backups/:id/download", backups.Download)
	}

	// Routes of the packages outside the community set (router_commercial.go).
	SetupContributed(r, cfg)
}
