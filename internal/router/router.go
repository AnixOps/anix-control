package router

import (
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
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
	exemptPaths := []string{"/health", "/metrics", "/swagger/"}
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
	agentPackageLimiter := middleware.NewRateLimiter(0.5, 8,
		middleware.WithTTL(5*time.Minute),
		middleware.WithTestModeBypass(false),
	)
	// 启动定期清理过期桶
	adminLimiter.StartCleanup(1 * time.Minute)
	userLimiter.StartCleanup(1 * time.Minute)
	publicLimiter.StartCleanup(1 * time.Minute)
	agentPackageLimiter.StartCleanup(1 * time.Minute)

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
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Prometheus 指标
	metricsHandler := handler.NewMetricsHandler()
	r.GET("/metrics", metricsHandler.GetMetrics)

	// 订阅接口 (公开，使用用户 token 认证)
	// 路径可通过配置 app.subscribe_path 自定义，默认为 "s"
	subscribeHandler := handler.NewSubscribeHandler(cfg)
	r.GET("/"+subscribePath+"/:token", subscribeHandler.GetSubscription)
	r.GET("/api/v1/client/subscribe", subscribeHandler.GetLegacySubscription)

	forwardFlowHandler := handler.NewForwardHandler()
	r.POST("/flow/upload", middleware.AppTokenAuth(), forwardFlowHandler.UploadPanelFlowData)

	// API v2
	v2 := r.Group("/api/v2")
	v2PackageGateway := handler.NewV2CompatibilityGateway(cfg)
	v2WebSocketGateway := handler.NewV2WebSocketGateway(cfg)
	{
		// 认证接口 (无需登录) - 公开限流
		authPublic := v2.Group("")
		authPublic.Use(publicLimiter.Middleware())
		{
			authPublic.POST("/login", v2PackageGateway.Serve)
			authPublic.POST("/register", v2PackageGateway.Serve)
		}

		cleanAgentHandler := handler.NewForwardCleanAgentHandler()
		forwardAgentPublic := v2.Group("/forward-agent")
		forwardAgentPublic.Use(publicLimiter.Middleware())
		{
			forwardAgentPublic.GET("/install.sh", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward_agent.install_sh.get", cleanAgentHandler.InstallScript))
			forwardAgentPublic.POST("/register", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward_agent.register.post", cleanAgentHandler.Register))
			forwardAgentPublic.POST("/heartbeat", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward_agent.heartbeat.post", cleanAgentHandler.Heartbeat))
			forwardAgentPublic.POST("/report", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward_agent.report.post", cleanAgentHandler.Report))
		}

		// 支付接口
		paymentHandler := handler.NewPaymentHandler()
		v2.GET("/payment/methods", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.methods.get", paymentHandler.GetPaymentMethods))
		v2.GET("/payment/status/:trade_no", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.status.trade_no.get", paymentHandler.GetPaymentStatus))

		// 支付回调 (无需认证)
		v2.POST("/payment/x402/callback", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.x402.callback.post", paymentHandler.X402Callback))
		v2.POST("/payment/stripe/webhook", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.stripe.webhook.post", paymentHandler.StripeWebhook))
		v2.POST("/payment/paypal/webhook", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.paypal.webhook.post", paymentHandler.PayPalWebhook))

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
			couponHandler := handler.NewCouponHandler()
			auth.POST("/user/coupon/check", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.user.coupon.check.post", couponHandler.CheckCoupon))

			// 订单接口
			orderHandler := handler.NewOrderHandler()
			auth.GET("/user/order", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.user.order.get", orderHandler.GetOrders))
			auth.POST("/user/order/save", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.user.order.save.post", orderHandler.SaveOrder))
			auth.GET("/user/order/:id", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.user.order.id.get", orderHandler.GetOrderDetail))

			// 用户支付接口
			auth.POST("/payment/x402/create", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.x402.create.post", paymentHandler.X402CreatePayment))
			auth.GET("/payment/x402/check/:id", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.x402.check.id.get", paymentHandler.X402CheckPayment))
			auth.POST("/payment/fiat/create", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.payment.fiat.create.post", paymentHandler.FiatCreatePayment))
		}

		// 管理员接口
		admin := v2.Group("/admin")
		admin.Use(adminLimiter.Middleware())
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
			admin.GET("/orders", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.get", adminHandler.GetOrderList))
			admin.GET("/orders/stats", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.stats.get", adminHandler.GetOrderStats))
			admin.GET("/orders/:id", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.id.get", adminHandler.GetOrder))
			admin.PUT("/orders/:id/status", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.id.status.put", adminHandler.UpdateOrderStatus))
			admin.POST("/orders/:id/paid", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.id.paid.post", adminHandler.MarkOrderPaid))
			admin.POST("/orders/:id/cancel", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.orders.id.cancel.post", adminHandler.CancelOrder))

			// 节点管理 (新版)
			nodeHandler := handler.NewNodeHandler()
			admin.GET("/nodes", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.get", nodeHandler.GetNodes))
			admin.GET("/nodes/stats", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.stats.get", nodeHandler.GetNodeStats))
			admin.GET("/nodes/:id/logs", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.logs.get", nodeHandler.GetNodeLogs))
			admin.POST("/nodes", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.post", nodeHandler.CreateNode))
			admin.GET("/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.get", nodeHandler.GetNode))
			admin.GET("/nodes/:id/credentials", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.nodes.id.credentials.get", nodeHandler.GetNodeCredentials))
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

			// 授权密钥管理
			admin.GET("/auth-keys", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.auth_keys.get", nodeHandler.GetAuthKeys))
			admin.POST("/auth-keys", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.auth_keys.post", nodeHandler.GenerateAuthKey))
			admin.DELETE("/auth-keys/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.admin.auth_keys.id.delete", nodeHandler.DeleteAuthKey))

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
			couponHandler := handler.NewAdminCouponHandler()
			admin.GET("/coupon", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.coupon.get", couponHandler.GetCoupons))
			admin.POST("/coupon", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.coupon.post", couponHandler.CreateCoupon))
			admin.DELETE("/coupon/:id", registeredPackageRoute(v2PackageGateway.Serve, "order", "order.admin.coupon.id.delete", couponHandler.DeleteCoupon))

			// 知识库管理 (V2 Stub)
			knowledgeHandler := handler.NewAdminKnowledgeHandler()
			admin.GET("/knowledge", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.get", knowledgeHandler.GetArticles))
			admin.POST("/knowledge", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.post", knowledgeHandler.CreateArticle))
			admin.PUT("/knowledge/:id", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.id.put", knowledgeHandler.UpdateArticle))
			admin.DELETE("/knowledge/:id", registeredPackageRoute(v2PackageGateway.Serve, "knowledge", "knowledge.admin.knowledge.id.delete", knowledgeHandler.DeleteArticle))

			// ========== 流量转发管理 ==========
			forwardHandler := handler.NewForwardHandler()
			admin.POST("/forward/create", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.create.post", forwardHandler.CreatePanelForward))
			admin.POST("/forward/list", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.list.post", forwardHandler.ListPanelForwards))
			admin.GET("/forward/runtime/jobs", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.runtime.jobs.get", forwardHandler.ListPanelRuntimeJobs))
			admin.GET("/forward/runtime/status", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.runtime.status.get", forwardHandler.GetPanelRuntimeStatus))
			admin.GET("/forward/runtime/doctor", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.runtime.doctor.get", forwardHandler.DiagnosePanelRuntime))
			admin.GET("/forward/local/status", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.local.status.get", forwardHandler.GetLocalRuntimeStatus))
			admin.GET("/forward/local/doctor", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.local.doctor.get", forwardHandler.DiagnoseLocalRuntime))
			admin.GET("/forward/nodex/status", registeredPackageRoute(v2PackageGateway.Serve, "gost-mesh", "gost.admin.forward.nodex.status.get", forwardHandler.GetNodeXRuntimeStatus))
			admin.GET("/forward/nodex/doctor", registeredPackageRoute(v2PackageGateway.Serve, "gost-mesh", "gost.admin.forward.nodex.doctor.get", forwardHandler.DiagnoseNodeXRuntime))
			// 可观测性 (延迟趋势/拓扑/多入口)
			admin.GET("/forward/observability/targets", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.observability.targets.get", forwardHandler.ListObservabilityTargets))
			admin.GET("/forward/observability/trend", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.observability.trend.get", forwardHandler.GetObservabilityTrend))
			admin.GET("/forward/observability/topology", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.observability.topology.get", forwardHandler.GetObservabilityTopology))
			admin.GET("/forward/observability/multi-ingress", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.observability.multi_ingress.get", forwardHandler.GetObservabilityMultiIngress))
			admin.POST("/forward/update", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.update.post", forwardHandler.UpdatePanelForward))
			admin.POST("/forward/delete", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.delete.post", forwardHandler.DeletePanelForward))
			admin.POST("/forward/force-delete", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.force_delete.post", forwardHandler.ForceDeletePanelForward))
			admin.POST("/forward/pause", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.pause.post", forwardHandler.PausePanelForward))
			admin.POST("/forward/resume", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.resume.post", forwardHandler.ResumePanelForward))
			admin.POST("/forward/diagnose", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.diagnose.post", forwardHandler.DiagnosePanelForward))
			admin.POST("/forward/sync-backend", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.sync_backend.post", forwardHandler.SyncForwardsToBackend))
			admin.POST("/forward/update-order", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.update_order.post", forwardHandler.UpdatePanelForwardOrder))
			admin.GET("/forward/agents", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.agents.get", cleanAgentHandler.ListAgents))
			admin.POST("/forward/agents", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.agents.post", cleanAgentHandler.CreateAgentToken))
			admin.POST("/forward/agents/:id/revoke", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.agents.id.revoke.post", cleanAgentHandler.RevokeAgent))
			admin.POST("/tunnel/create", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.create.post", forwardHandler.CreatePanelTunnel))
			admin.POST("/tunnel/list", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.list.post", forwardHandler.ListPanelAdminTunnels))
			admin.POST("/tunnel/update", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.update.post", forwardHandler.UpdatePanelTunnel))
			admin.POST("/tunnel/delete", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.delete.post", forwardHandler.DeletePanelTunnel))
			admin.POST("/tunnel/diagnose", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.diagnose.post", forwardHandler.DiagnosePanelTunnel))
			admin.POST("/tunnel/user/tunnel", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.user.tunnel.post", forwardHandler.ListPanelTunnels))
			admin.POST("/tunnel/user/assign", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.user.assign.post", forwardHandler.AssignPanelUserTunnel))
			admin.POST("/tunnel/user/list", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.user.list.post", forwardHandler.ListPanelUserTunnels))
			admin.POST("/tunnel/user/remove", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.user.remove.post", forwardHandler.RemovePanelUserTunnel))
			admin.POST("/tunnel/user/update", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.tunnel.user.update.post", forwardHandler.UpdatePanelUserTunnel))

			// Ansible 执行机器管理
			admin.GET("/forward/ansible-machines", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.get", forwardHandler.ListAnsibleMachines))
			admin.POST("/forward/ansible-machines", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.post", forwardHandler.CreateAnsibleMachine))
			admin.GET("/forward/ansible-machines/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.get", forwardHandler.GetAnsibleMachine))
			admin.PUT("/forward/ansible-machines/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.put", forwardHandler.UpdateAnsibleMachine))
			admin.DELETE("/forward/ansible-machines/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.delete", forwardHandler.DeleteAnsibleMachine))
			admin.POST("/forward/ansible-machines/:id/check", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.check.post", forwardHandler.CheckAnsibleMachine))
			admin.POST("/forward/ansible-machines/:id/toggle", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.toggle.post", forwardHandler.ToggleAnsibleMachine))
			admin.POST("/forward/ansible-machines/:id/sync-stats", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.ansible_machines.id.sync_stats.post", forwardHandler.SyncAnsibleMachineStats))

			// 中转节点管理
			admin.GET("/forward/nodes", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.get", forwardHandler.ListNodes))
			admin.POST("/forward/nodes", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.post", forwardHandler.CreateNode))
			admin.GET("/forward/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.get", forwardHandler.GetNode))
			admin.PUT("/forward/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.put", forwardHandler.UpdateNode))
			admin.DELETE("/forward/nodes/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.delete", forwardHandler.DeleteNode))
			admin.POST("/forward/nodes/:id/check", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.check.post", forwardHandler.CheckNode))
			admin.POST("/forward/nodes/:id/toggle", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.toggle.post", forwardHandler.ToggleNode))
			admin.POST("/forward/nodes/:id/sync-stats", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.nodes.id.sync_stats.post", forwardHandler.SyncNodeStats))
			admin.POST("/forward/test-connection", registeredPackageRoute(v2PackageGateway.Serve, "gost-mesh", "gost.admin.forward.test_connection.post", forwardHandler.TestGostConnection))

			// 转发规则管理
			admin.GET("/forward/rules", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.get", forwardHandler.ListRules))
			admin.POST("/forward/rules", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.post", forwardHandler.CreateRule))
			admin.GET("/forward/rules/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.id.get", forwardHandler.GetRule))
			admin.PUT("/forward/rules/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.id.put", forwardHandler.UpdateRule))
			admin.DELETE("/forward/rules/:id", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.id.delete", forwardHandler.DeleteRule))
			admin.POST("/forward/rules/:id/toggle", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.rules.id.toggle.post", forwardHandler.ToggleRule))

			// 转发统计
			admin.GET("/forward/stats", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.admin.forward.stats.get", forwardHandler.GetForwardStats))

			// ========== 支付网关管理 ==========
			paymentGatewayHandler := handler.NewPaymentGatewayHandler()

			// 网关管理
			admin.GET("/payment/gateways", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.gateways.get", paymentGatewayHandler.ListGateways))
			admin.POST("/payment/gateways", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.gateways.post", paymentGatewayHandler.CreateGateway))
			admin.PUT("/payment/gateways/:id", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.gateways.id.put", paymentGatewayHandler.UpdateGateway))
			admin.DELETE("/payment/gateways/:id", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.gateways.id.delete", paymentGatewayHandler.DeleteGateway))
			admin.POST("/payment/gateways/:id/toggle", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.gateways.id.toggle.post", paymentGatewayHandler.ToggleGateway))

			// 支付统计和记录
			admin.GET("/payment/stats", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.stats.get", paymentGatewayHandler.GetPaymentStats))
			admin.GET("/payment/records", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.admin.payment.records.get", paymentGatewayHandler.ListPaymentRecords))

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
			admin.GET("/invite/config", v2PackageGateway.Serve)
			admin.PUT("/invite/config", v2PackageGateway.Serve)
			admin.GET("/invite/stats", v2PackageGateway.Serve)
			admin.GET("/invite/withdrawals", v2PackageGateway.Serve)
			admin.POST("/invite/withdrawals/:id/process", v2PackageGateway.Serve)

			// ========== 系统配置管理 ==========
			systemHandler := handler.NewSystemHandler()
			admin.GET("/system/configs", v2PackageGateway.Serve)
			admin.GET("/system/configs/:key", v2PackageGateway.Serve)
			admin.GET("/system/subscription-settings", registeredPackageRoute(v2PackageGateway.Serve, "subscription", "subscription.admin.system.subscription_settings.get", systemHandler.GetSubscriptionSettings))
			admin.PUT("/system/configs/:key", v2PackageGateway.Serve)
			admin.DELETE("/system/configs/:key", v2PackageGateway.Serve)
			admin.GET("/system/audit-logs", v2PackageGateway.Serve)

			// ========== 备份管理 ==========
			admin.GET("/system/backup/config", v2PackageGateway.Serve)
			admin.PUT("/system/backup/config", v2PackageGateway.Serve)
			admin.POST("/system/backup", v2PackageGateway.Serve)
			admin.GET("/system/backups", v2PackageGateway.Serve)
			admin.GET("/system/backup/stats", v2PackageGateway.Serve)
			admin.DELETE("/system/backups/:id", v2PackageGateway.Serve)
			admin.POST("/system/backups/:id/restore", v2PackageGateway.Serve)

			// ========== 负载均衡管理 ==========
			lbHandler := handler.NewLoadBalancerHandler()
			admin.GET("/loadbalancers", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.get", lbHandler.ListLoadBalancers))
			admin.POST("/loadbalancers", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.post", lbHandler.CreateLoadBalancer))
			admin.GET("/loadbalancers/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.id.get", lbHandler.GetLoadBalancer))
			admin.PUT("/loadbalancers/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.id.put", lbHandler.UpdateLoadBalancer))
			admin.DELETE("/loadbalancers/:id", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.id.delete", lbHandler.DeleteLoadBalancer))
			admin.GET("/loadbalancers/:id/stats", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.id.stats.get", lbHandler.GetLoadBalancerStats))
			admin.POST("/loadbalancers/:id/check", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.loadbalancer.id.check.post", lbHandler.RunHealthCheck))
		}

		// 用户接口 (需认证)
		authUser := v2.Group("")
		authUser.Use(userLimiter.Middleware())
		authUser.Use(middleware.JWTAuth())
		{
			adminCompat := authUser.Group("")
			adminCompat.Use(middleware.AdminAuth())
			{
				forwardCompatHandler := handler.NewForwardHandler()
				speedLimitHandler := handler.NewSpeedLimitHandler()
				adminCompat.POST("/user/reset", v2PackageGateway.Serve)
				adminCompat.POST("/speed-limit/create", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.speed_limit.create.post", speedLimitHandler.CreatePanelSpeedLimit))
				adminCompat.POST("/speed-limit/list", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.speed_limit.list.post", speedLimitHandler.ListPanelSpeedLimits))
				adminCompat.POST("/speed-limit/update", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.speed_limit.update.post", speedLimitHandler.UpdatePanelSpeedLimit))
				adminCompat.POST("/speed-limit/delete", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.speed_limit.delete.post", speedLimitHandler.DeletePanelSpeedLimit))
				adminCompat.POST("/speed-limit/tunnels", registeredPackageRoute(v2PackageGateway.Serve, "plan", "plan.speed_limit.tunnels.post", speedLimitHandler.ListPanelSpeedLimitTunnels))
				adminCompat.POST("/tunnel/user/assign", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.tunnel.user.assign.post", forwardCompatHandler.AssignPanelUserTunnel))
				adminCompat.POST("/tunnel/user/list", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.tunnel.user.list.post", forwardCompatHandler.ListPanelUserTunnels))
				adminCompat.POST("/tunnel/user/remove", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.tunnel.user.remove.post", forwardCompatHandler.RemovePanelUserTunnel))
				adminCompat.POST("/tunnel/user/update", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.tunnel.user.update.post", forwardCompatHandler.UpdatePanelUserTunnel))
			}

			// 用户转发规则
			forwardHandler := handler.NewForwardHandler()
			authUser.GET("/user/forward/rules", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.user.forward.rules.get", forwardHandler.GetUserRules))
			authUser.POST("/user/forward/rules", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.user.forward.rules.post", forwardHandler.CreateUserRule))
			authUser.POST("/forward/create", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.create.post", forwardHandler.CreatePanelForward))
			authUser.POST("/forward/list", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.list.post", forwardHandler.ListPanelForwards))
			authUser.POST("/forward/update", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.update.post", forwardHandler.UpdatePanelForward))
			authUser.POST("/forward/delete", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.delete.post", forwardHandler.DeletePanelForward))
			authUser.POST("/forward/force-delete", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.force_delete.post", forwardHandler.ForceDeletePanelForward))
			authUser.POST("/forward/pause", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.pause.post", forwardHandler.PausePanelForward))
			authUser.POST("/forward/resume", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.resume.post", forwardHandler.ResumePanelForward))
			authUser.POST("/forward/diagnose", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.diagnose.post", forwardHandler.DiagnosePanelForward))
			authUser.POST("/forward/update-order", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.update_order.post", forwardHandler.UpdatePanelForwardOrder))
			authUser.POST("/tunnel/user/tunnel", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.tunnel.user.tunnel.post", forwardHandler.ListPanelTunnels))

			// 用户支付
			paymentGatewayHandler := handler.NewPaymentGatewayHandler()
			authUser.GET("/user/payment/channels", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.user.payment.channels.get", paymentGatewayHandler.GetChannels))
			authUser.POST("/user/payment/create", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.user.payment.create.post", paymentGatewayHandler.CreatePayment))
			authUser.GET("/user/payment/status/:trade_no", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.user.payment.status.trade_no.get", paymentGatewayHandler.GetPaymentStatus))
			authUser.GET("/user/payment/records", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.user.payment.records.get", paymentGatewayHandler.GetUserRecords))

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

			// 用户通知
			notificationHandler := handler.NewNotificationHandler()
			authUser.GET("/user/notifications", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.get", notificationHandler.GetUserNotifications))
			authUser.GET("/user/notifications/unread-count", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.unread_count.get", notificationHandler.GetUnreadCount))
			authUser.POST("/user/notifications/:id/read", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.id.read.post", notificationHandler.MarkAsRead))
			authUser.POST("/user/notifications/read-all", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.user.notifications.read_all.post", notificationHandler.MarkAllAsRead))

			// 用户邀请
			authUser.GET("/user/invite", v2PackageGateway.Serve)
			authUser.POST("/user/invite/generate", v2PackageGateway.Serve)
			authUser.GET("/user/invite/commissions", v2PackageGateway.Serve)
			authUser.POST("/user/invite/withdraw", v2PackageGateway.Serve)
			authUser.GET("/user/invite/withdrawals", v2PackageGateway.Serve)
		}

		// Telegram Webhook (公开)
		telegramHandler := handler.NewTelegramHandler()
		tgWebhook := v2.Group("")
		tgWebhook.Use(publicLimiter.Middleware())
		{
			tgWebhook.POST("/telegram/webhook", registeredPackageRoute(v2PackageGateway.Serve, "notification", "notification.telegram.webhook.post", telegramHandler.TelegramWebhook))
		}

		// 支付回调 (公开)
		paymentGatewayHandler := handler.NewPaymentGatewayHandler()
		paymentCallback := v2.Group("")
		paymentCallback.Use(publicLimiter.Middleware())
		{
			paymentCallback.POST("/payment/callback/:type", registeredPackageRoute(v2PackageGateway.Serve, "payment", "payment.callback", paymentGatewayHandler.PaymentCallback))
		}

		// 节点自动注册 API (公开)
		agentHandler := handler.NewAgentHandler()

		nodePublic := v2.Group("/node")
		nodePublic.Use(publicLimiter.Middleware())
		{
			nodeHandler := handler.NewNodeHandler()
			nodePublic.POST("/register", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.node.register.post", nodeHandler.Register))
			nodePublic.GET("/ws", registeredPackageWebSocketRoute(v2WebSocketGateway.Serve, "proxy-node", "proxy.node.ws.get", agentHandler.AgentWebSocketUnified, agentHandler.PrepareWebSocketBridge))
		}

		// 节点通信 API (需要 API Key 认证 + 可选签名验证)
		nodeAPI := v2.Group("/node")
		nodeAPI.Use(userLimiter.Middleware())
		nodeAPI.Use(middleware.NodeAPIKeyAuth())
		nodeAPI.Use(middleware.SignatureAuth())    // 签名验证 (向后兼容，可选)
		nodeAPI.Use(middleware.NodeSecureLogger()) // 安全审计日志
		{
			nodeHandler := handler.NewNodeHandler()
			nodeAPI.POST("/heartbeat", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.node.heartbeat.post", nodeHandler.Heartbeat))
			nodeAPI.POST("/runtime-health", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.node.runtime_health.post", nodeHandler.RuntimeHealth))
		}

		// UniProxy API (节点通信接口)
		uniproxy := v2.Group("/server/UniProxy")
		uniproxy.Use(userLimiter.Middleware())
		uniproxy.Use(middleware.NodeAuth())
		{
			h := handler.NewUniProxyHandler()
			uniproxy.GET("/config", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.server.uniproxy.config.get", h.GetConfig))
			uniproxy.GET("/user", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.server.uniproxy.user.get", h.GetUsers))
			uniproxy.GET("/alivelist", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.server.uniproxy.alivelist.get", h.GetAliveList))
			uniproxy.POST("/push", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.server.uniproxy.push.post", h.PushTraffic))
			uniproxy.POST("/alive", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.server.uniproxy.alive.post", h.PushAlive))
		}

		// UniProxy API v1 (兼容旧版 V2bX)
		uniproxyV1 := r.Group("/api/v1/server/UniProxy")
		uniproxyV1.Use(userLimiter.Middleware())
		uniproxyV1.Use(middleware.NodeAuth())
		{
			h := handler.NewUniProxyHandler()
			uniproxyV1.GET("/config", h.GetConfig)
			uniproxyV1.GET("/user", h.GetUsers)
			uniproxyV1.GET("/alivelist", h.GetAliveList)
			uniproxyV1.POST("/push", h.PushTraffic)
			uniproxyV1.POST("/alive", h.PushAlive)
		}

		// Agent API (NAT 后节点主动连接)
		agentPublic := v2.Group("/agent")
		agentPublic.Use(userLimiter.Middleware())
		{
			agentPublic.POST("/register", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.agent.register.post", agentHandler.AgentRegister))
			agentPublic.POST("/heartbeat", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.agent.heartbeat.post", agentHandler.AgentHeartbeat))
			agentPublic.GET("/tasks", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.agent.tasks.get", agentHandler.AgentGetTasks))
			agentPublic.POST("/result", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.agent.result.post", agentHandler.AgentReportResult))
			agentPublic.POST("/monitor", registeredPackageRoute(v2PackageGateway.Serve, "protocol-runtime", "protocol.agent.monitor.post", agentHandler.AgentMonitor))
			agentPublic.GET("/ws", registeredPackageWebSocketRoute(v2WebSocketGateway.Serve, "protocol-runtime", "protocol.agent.ws.get", agentHandler.AgentWebSocketUnified, agentHandler.PrepareWebSocketBridge))
		}

		// 转发规则同步 (Agent 使用)
		v2.GET("/forward/agent/rules", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.forward.agent.rules.get", agentHandler.AgentGetForwardRules))

		// Agent 管理接口 (管理员)
		agentAdmin := v2.Group("/admin/agent")
		agentAdmin.Use(adminLimiter.Middleware())
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

		internalAPI := v2.Group("/internal")
		internalAPI.Use(adminLimiter.Middleware())
		internalAPI.Use(middleware.AppTokenAuth())
		{
			internalAPI.POST("/forward/traffic/upload", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.internal.forward.traffic.upload.post", forwardFlowHandler.UploadPanelFlowData))
			internalAPI.POST("/forward/traffic/report", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.internal.forward.traffic.report.post", forwardFlowHandler.ReportPanelForwardTraffic))
			internalAPI.POST("/forward/traffic/snapshot", registeredPackageRoute(v2PackageGateway.Serve, "forward", "forward.internal.forward.traffic.snapshot.post", forwardFlowHandler.SnapshotPanelForwardTraffic))
			// 内部 AuthKey 生成（用于 Ansible 自动化）
			nodeHandlerForInternal := handler.NewNodeHandler()
			internalAPI.POST("/auth-keys", registeredPackageRoute(v2PackageGateway.Serve, "proxy-node", "proxy.internal.auth_keys.post", nodeHandlerForInternal.InternalGenerateAuthKey))
		}
	}

	// Node-facing v3 package downloads are deliberately outside the admin v3
	// group. The URL is same-origin, while NodeAPIKeyAuth supplies the only
	// trusted node identity used for assignment authorization.
	agentPackages := r.Group("/api/v3/agent/plugin-releases")
	agentPackages.Use(middleware.NodeAPIKeyHeaderAuth())
	agentPackages.Use(agentPackageLimiter.MiddlewareForContextKey("node_id"))
	agentPackages.Use(middleware.NodeSecureLogger())
	{
		kernel := handler.NewKernelHandler()
		agentPackages.GET("/:plugin_id/:version/artifact", kernel.ServeAgentPluginArtifact)
		agentPackages.GET("/:plugin_id/:version/manifest", kernel.ServeAgentPluginManifest)
	}

	// API v3 is the control-kernel surface. Legacy business APIs remain under
	// /api/v2 while services are migrated behind plugin compatibility adapters.
	v3 := r.Group("/api/v3")
	v3.Use(adminLimiter.Middleware())
	v3.Use(middleware.JWTAuth())
	v3.Use(middleware.AdminAuth())
	v3.Use(middleware.AuditLog())
	{
		kernel := handler.NewKernelHandler()
		v3.GET("/extensions", kernel.ListExtensions)
		v3.GET("/extensions/:plugin_id/:version/webui/:sha256/:filename", kernel.ServePluginWebUIAsset)
		v3.GET("/plugins", kernel.ListPlugins)
		v3.Any("/plugins/:plugin_id/*route", kernel.PluginRouteGateway)
		v3.GET("/plugin-releases", kernel.ListPluginReleases)
		v3.POST("/plugin-releases", kernel.RegisterPluginRelease)
		v3.GET("/plugin-releases/:id/artifact", kernel.GetPluginReleaseArtifact)
		v3.POST("/plugin-releases/:id/artifact", kernel.UploadPluginReleaseArtifact)
		v3.GET("/plugin-installations", kernel.ListPluginInstallations)
		v3.PUT("/plugin-installations", kernel.UpsertPluginInstallation)
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

		v3.GET("/topologies", kernel.ListTopologies)
		v3.POST("/topologies", kernel.CreateTopology)
		v3.POST("/topologies/validate", kernel.ValidateTopology)
		v3.GET("/topologies/:id/revisions", kernel.ListTopologyRevisions)
		v3.POST("/topologies/:id/revisions", kernel.CreateTopologyRevision)
		v3.GET("/topologies/:id/revisions/:revision_id", kernel.GetTopologyRevision)
		v3.POST("/topologies/:id/revisions/:revision_id/preview", kernel.PreviewTopologyDeployment)
		v3.POST("/topologies/:id/revisions/:revision_id/diagnose", kernel.DiagnoseTopologyDeployment)
		v3.GET("/topologies/:id/revisions/:revision_id/preview", kernel.PreviewTopologyDeployment)
		v3.GET("/topologies/:id/revisions/:revision_id/diagnose", kernel.DiagnoseTopologyDeployment)
		v3.GET("/deployments", kernel.ListDeployments)
		v3.POST("/deployments/preview", kernel.PreviewDeployment)
		v3.POST("/deployments/diagnose", kernel.PreviewDeployment)
		v3.POST("/deployments", kernel.PlanDeployment)
		v3.GET("/deployments/:id", kernel.GetDeploymentStatus)
		v3.POST("/deployments/:id/apply", kernel.ApplyDeployment)
		v3.POST("/deployments/:id/rollback", kernel.RollbackDeployment)
		v3.GET("/operations", kernel.ListOperations)
		v3.POST("/operations", kernel.CreateOperation)
		v3.POST("/operations/:id/cancel", kernel.CancelOperation)
		v3.GET("/observed-states", kernel.ListObservedStates)
	}

	// API v4 currently exposes only the package route gateway. Its admission
	// model is identical to v3 while package execution moves to local hosts.
	v4 := r.Group("/api/v4")
	v4.Use(adminLimiter.Middleware())
	v4.Use(middleware.JWTAuth())
	v4.Use(middleware.AdminAuth())
	v4.Use(middleware.AuditLog())
	{
		kernel := handler.NewKernelHandler()
		v4.Any("/plugins/:plugin_id/*route", kernel.PluginRouteGateway)
	}
}
