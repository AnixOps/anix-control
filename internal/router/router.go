package router

import (
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/handler"
	"github.com/anixops/v2board/internal/middleware"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Setup 设置路由
func Setup(r *gin.Engine, cfg *config.Config) {
	// 全局中间件
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())

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
	subscribePath := cfg.App.SubscribePath
	if subscribePath == "" {
		subscribePath = "s"
	}
	subscribeHandler := handler.NewSubscribeHandler(cfg)
	r.GET("/"+subscribePath+"/:token", subscribeHandler.GetSubscription)

	// API v2
	v2 := r.Group("/api/v2")
	{
		// 认证接口 (无需登录)
		authHandler := handler.NewAuthHandler(cfg)
		v2.POST("/login", authHandler.Login)
		v2.POST("/register", authHandler.Register)

		// 支付接口
		paymentHandler := handler.NewPaymentHandler()
		v2.GET("/payment/methods", paymentHandler.GetPaymentMethods)
		v2.GET("/payment/status/:trade_no", paymentHandler.GetPaymentStatus)

		// 支付回调 (无需认证)
		v2.POST("/payment/x402/callback", paymentHandler.X402Callback)
		v2.POST("/payment/stripe/webhook", paymentHandler.StripeWebhook)
		v2.POST("/payment/paypal/webhook", paymentHandler.PayPalWebhook)

		// 需要登录的接口
		auth := v2.Group("")
		auth.Use(middleware.JWTAuth())
		{
			// 用户接口
			userHandler := handler.NewUserHandler()
			auth.GET("/user/profile", userHandler.GetProfile)
			auth.GET("/user/dashboard", userHandler.GetDashboard)
			// 用户订阅接口
			auth.GET("/user/subscription", userHandler.GetSubscription)

			// 知识库接口
			knowledgeHandler := handler.NewKnowledgeHandler()
			auth.GET("/user/knowledge", knowledgeHandler.GetArticles)
			auth.GET("/user/knowledge/:id", knowledgeHandler.GetArticle)

			// 工单接口
			ticketHandler := handler.NewTicketHandler()
			auth.GET("/user/ticket", ticketHandler.GetTickets)
			auth.POST("/user/ticket", ticketHandler.CreateTicket)
			auth.GET("/user/ticket/:id", ticketHandler.GetTicket)
			auth.POST("/user/ticket/:id/reply", ticketHandler.ReplyTicket)
			auth.POST("/user/ticket/:id/close", ticketHandler.CloseTicket)

			// 套餐接口
			planHandler := handler.NewUserPlanHandler()
			auth.GET("/user/plan", planHandler.GetPlans)

			// 优惠券接口
			couponHandler := handler.NewCouponHandler()
			auth.POST("/user/coupon/check", couponHandler.CheckCoupon)

			// 订单接口
			orderHandler := handler.NewOrderHandler()
			auth.GET("/user/order", orderHandler.GetOrders)
			auth.POST("/user/order/save", orderHandler.SaveOrder)
			auth.GET("/user/order/:id", orderHandler.GetOrderDetail)

			// 用户支付接口
			auth.POST("/payment/x402/create", paymentHandler.X402CreatePayment)
			auth.GET("/payment/x402/check/:id", paymentHandler.X402CheckPayment)
			auth.POST("/payment/fiat/create", paymentHandler.FiatCreatePayment)
		}

		// 管理员接口
		admin := v2.Group("/admin")
		admin.Use(middleware.JWTAuth())
		admin.Use(middleware.AdminAuth())
		{
			adminHandler := handler.NewAdminHandler()

			// 仪表盘
			admin.GET("/dashboard", adminHandler.GetDashboard)

			// 用户管理
			admin.POST("/users", adminHandler.CreateUser)
			admin.GET("/users", adminHandler.GetUserList)
			admin.GET("/users/stats", adminHandler.GetUserStats)
			admin.GET("/users/:id", adminHandler.GetUser)
			admin.PUT("/users/:id", adminHandler.UpdateUser)
			admin.DELETE("/users/:id", adminHandler.DeleteUser)
			admin.POST("/users/:id/ban", adminHandler.BanUser)
			admin.POST("/users/:id/unban", adminHandler.UnbanUser)
			admin.POST("/users/:id/reset-traffic", adminHandler.ResetUserTraffic)

			// 订单管理
			admin.GET("/orders", adminHandler.GetOrderList)
			admin.GET("/orders/stats", adminHandler.GetOrderStats)
			admin.GET("/orders/:id", adminHandler.GetOrder)
			admin.PUT("/orders/:id/status", adminHandler.UpdateOrderStatus)
			admin.POST("/orders/:id/paid", adminHandler.MarkOrderPaid)
			admin.POST("/orders/:id/cancel", adminHandler.CancelOrder)

			// 节点管理 (新版)
			nodeHandler := handler.NewNodeHandler()
			admin.GET("/nodes", nodeHandler.GetNodes)
			admin.GET("/nodes/stats", nodeHandler.GetNodeStats)
			admin.POST("/nodes", nodeHandler.CreateNode)
			admin.GET("/nodes/:id", nodeHandler.GetNode)
			admin.PUT("/nodes/:id", nodeHandler.UpdateNode)
			admin.DELETE("/nodes/:id", nodeHandler.DeleteNode)
			admin.POST("/nodes/:id/sync", nodeHandler.SyncProtocol)

			// 节点高级配置 (RawConfig - 直接JSON编辑)
			admin.GET("/nodes/:id/raw-config", nodeHandler.GetNodeRawConfig)
			admin.PUT("/nodes/:id/raw-config", nodeHandler.UpdateNodeRawConfig)
			admin.POST("/nodes/validate-config", nodeHandler.ValidateRawConfig)

			// 节点协议管理
			admin.GET("/nodes/:id/protocols", nodeHandler.GetProtocols)
			admin.POST("/nodes/:id/protocols", nodeHandler.CreateProtocol)
			admin.PUT("/nodes/:id/protocols/:protocol_id", nodeHandler.UpdateProtocol)
			admin.DELETE("/nodes/:id/protocols/:protocol_id", nodeHandler.DeleteProtocol)
			admin.GET("/protocol-templates", nodeHandler.GetProtocolTemplates)

			// 授权密钥管理
			admin.GET("/auth-keys", nodeHandler.GetAuthKeys)
			admin.POST("/auth-keys", nodeHandler.GenerateAuthKey)
			admin.DELETE("/auth-keys/:id", nodeHandler.DeleteAuthKey)

			// 订阅分组和模板管理
			subAdmin := handler.NewSubscriptionAdminHandler()

			// 订阅分组
			admin.GET("/subscription/groups", subAdmin.GetGroups)
			admin.POST("/subscription/groups", subAdmin.CreateGroup)
			admin.GET("/subscription/groups/:id", subAdmin.GetGroup)
			admin.PUT("/subscription/groups/:id", subAdmin.UpdateGroup)
			admin.DELETE("/subscription/groups/:id", subAdmin.DeleteGroup)

			// 订阅模板
			admin.GET("/subscription/groups/:id/templates", subAdmin.GetTemplates)
			admin.GET("/subscription/groups/:id/protocols", subAdmin.GetGroupProtocols)
			admin.POST("/subscription/groups/:id/protocols", subAdmin.UpdateGroupProtocols)
			admin.POST("/subscription/groups/:id/templates", subAdmin.CreateTemplate)
			admin.GET("/subscription/templates/:id", subAdmin.GetTemplate)
			admin.PUT("/subscription/templates/:id", subAdmin.UpdateTemplate)
			admin.DELETE("/subscription/templates/:id", subAdmin.DeleteTemplate)

			// 用户订阅分组关联
			admin.GET("/subscription/users/:user_id/groups", subAdmin.GetUserGroups)
			admin.POST("/subscription/users/:user_id/groups", subAdmin.AssignGroupToUser)
			admin.DELETE("/subscription/users/:user_id/groups/:group_id", subAdmin.RemoveGroupFromUser)

			// 套餐订阅分组关联
			admin.GET("/subscription/plans/:plan_id/groups", subAdmin.GetPlanGroups)
			admin.POST("/subscription/plans/:plan_id/groups", subAdmin.AssignGroupToPlan)
			admin.DELETE("/subscription/plans/:plan_id/groups/:group_id", subAdmin.RemoveGroupFromPlan)

			// 订阅工具接口
			admin.GET("/subscription/formats", subAdmin.GetSubscriptionFormats)
			admin.GET("/subscription/protocols", subAdmin.GetProtocolTypes)
			admin.GET("/subscription/protocols/available", subAdmin.GetAvailableProtocols)
			admin.POST("/subscription/preview", subAdmin.PreviewSubscription)

			// 套餐管理
			admin.GET("/plans", adminHandler.GetPlans)
			admin.POST("/plans", adminHandler.CreatePlan)
			admin.GET("/plans/:id", adminHandler.GetPlan)
			admin.PUT("/plans/:id", adminHandler.UpdatePlan)
			admin.DELETE("/plans/:id", adminHandler.DeletePlan)
			admin.POST("/plans/:id/assign", adminHandler.AssignPlanToUser)

			// 工单管理 (V2 Stub)
			ticketHandler := handler.NewAdminTicketHandler()
			admin.GET("/ticket", ticketHandler.GetTickets)
			admin.POST("/ticket/reply", ticketHandler.ReplyTicket)
			admin.POST("/ticket/:id/close", ticketHandler.CloseTicket)

			// 优惠券管理 (V2 Stub)
			couponHandler := handler.NewAdminCouponHandler()
			admin.GET("/coupon", couponHandler.GetCoupons)
			admin.POST("/coupon", couponHandler.CreateCoupon)
			admin.DELETE("/coupon/:id", couponHandler.DeleteCoupon)

			// 知识库管理 (V2 Stub)
			knowledgeHandler := handler.NewAdminKnowledgeHandler()
			admin.GET("/knowledge", knowledgeHandler.GetArticles)
			admin.POST("/knowledge", knowledgeHandler.CreateArticle)
			admin.PUT("/knowledge/:id", knowledgeHandler.UpdateArticle)
			admin.DELETE("/knowledge/:id", knowledgeHandler.DeleteArticle)

			// ========== 流量转发管理 ==========
			forwardHandler := handler.NewForwardHandler()

			// 中转节点管理
			admin.GET("/forward/nodes", forwardHandler.ListNodes)
			admin.POST("/forward/nodes", forwardHandler.CreateNode)
			admin.GET("/forward/nodes/:id", forwardHandler.GetNode)
			admin.PUT("/forward/nodes/:id", forwardHandler.UpdateNode)
			admin.DELETE("/forward/nodes/:id", forwardHandler.DeleteNode)
			admin.POST("/forward/nodes/:id/check", forwardHandler.CheckNode)
			admin.POST("/forward/nodes/:id/toggle", forwardHandler.ToggleNode)
			admin.POST("/forward/nodes/:id/sync-stats", forwardHandler.SyncNodeStats)
			admin.POST("/forward/test-connection", forwardHandler.TestGostConnection)

			// 转发规则管理
			admin.GET("/forward/rules", forwardHandler.ListRules)
			admin.POST("/forward/rules", forwardHandler.CreateRule)
			admin.GET("/forward/rules/:id", forwardHandler.GetRule)
			admin.PUT("/forward/rules/:id", forwardHandler.UpdateRule)
			admin.DELETE("/forward/rules/:id", forwardHandler.DeleteRule)
			admin.POST("/forward/rules/:id/toggle", forwardHandler.ToggleRule)

			// 转发统计
			admin.GET("/forward/stats", forwardHandler.GetForwardStats)

			// ========== 支付网关管理 ==========
			paymentGatewayHandler := handler.NewPaymentGatewayHandler()

			// 网关管理
			admin.GET("/payment/gateways", paymentGatewayHandler.ListGateways)
			admin.POST("/payment/gateways", paymentGatewayHandler.CreateGateway)
			admin.PUT("/payment/gateways/:id", paymentGatewayHandler.UpdateGateway)
			admin.DELETE("/payment/gateways/:id", paymentGatewayHandler.DeleteGateway)
			admin.POST("/payment/gateways/:id/toggle", paymentGatewayHandler.ToggleGateway)

			// 支付统计和记录
			admin.GET("/payment/stats", paymentGatewayHandler.GetPaymentStats)
			admin.GET("/payment/records", paymentGatewayHandler.ListPaymentRecords)

			// ========== Telegram Bot 管理 ==========
			telegramHandler := handler.NewTelegramHandler()

			admin.GET("/telegram/bot", telegramHandler.GetBot)
			admin.PUT("/telegram/bot", telegramHandler.UpdateBot)
			admin.POST("/telegram/webhook", telegramHandler.SetWebhook)
			admin.DELETE("/telegram/webhook", telegramHandler.DeleteWebhook)
			admin.POST("/telegram/notify", telegramHandler.SendNotification)
			admin.POST("/telegram/broadcast", telegramHandler.Broadcast)
			admin.GET("/telegram/users", telegramHandler.GetUserBindings)

			// ========== MFA 管理 ==========
			mfaHandler := handler.NewMFAHandler()
			admin.GET("/mfa/config", mfaHandler.GetAdminConfig)
			admin.PUT("/mfa/config", mfaHandler.UpdateAdminConfig)

			// ========== 通知管理 ==========
			notificationHandler := handler.NewNotificationHandler()
			admin.GET("/notification/templates", notificationHandler.ListTemplates)
			admin.POST("/notification/templates", notificationHandler.CreateTemplate)
			admin.PUT("/notification/templates/:id", notificationHandler.UpdateTemplate)
			admin.DELETE("/notification/templates/:id", notificationHandler.DeleteTemplate)
			admin.GET("/notification/logs", notificationHandler.ListLogs)
			admin.POST("/notification/test", notificationHandler.SendTestNotification)
			admin.GET("/notification/email/config", notificationHandler.GetEmailConfig)
			admin.PUT("/notification/email/config", notificationHandler.UpdateEmailConfig)

			// ========== 邀请返利管理 ==========
			inviteHandler := handler.NewInviteHandler()
			admin.GET("/invite/config", inviteHandler.GetConfig)
			admin.PUT("/invite/config", inviteHandler.UpdateConfig)
			admin.GET("/invite/stats", inviteHandler.GetInviteStats)
			admin.GET("/invite/withdrawals", inviteHandler.GetWithdrawals)
			admin.POST("/invite/withdrawals/:id/process", inviteHandler.ProcessWithdraw)

			// ========== 系统配置管理 ==========
			systemHandler := handler.NewSystemHandler()
			admin.GET("/system/configs", systemHandler.GetConfigs)
			admin.GET("/system/configs/:key", systemHandler.GetConfig)
			admin.PUT("/system/configs/:key", systemHandler.SetConfig)
			admin.DELETE("/system/configs/:key", systemHandler.DeleteConfig)

			// ========== 备份管理 ==========
			admin.GET("/system/backup/config", systemHandler.GetBackupConfig)
			admin.PUT("/system/backup/config", systemHandler.UpdateBackupConfig)
			admin.POST("/system/backup", systemHandler.CreateBackup)
			admin.GET("/system/backups", systemHandler.ListBackups)
			admin.GET("/system/backup/stats", systemHandler.GetBackupStats)
			admin.DELETE("/system/backups/:id", systemHandler.DeleteBackup)
			admin.POST("/system/backups/:id/restore", systemHandler.RestoreBackup)

			// ========== 负载均衡管理 ==========
			lbHandler := handler.NewLoadBalancerHandler()
			admin.GET("/loadbalancers", lbHandler.ListLoadBalancers)
			admin.POST("/loadbalancers", lbHandler.CreateLoadBalancer)
			admin.GET("/loadbalancers/:id", lbHandler.GetLoadBalancer)
			admin.PUT("/loadbalancers/:id", lbHandler.UpdateLoadBalancer)
			admin.DELETE("/loadbalancers/:id", lbHandler.DeleteLoadBalancer)
			admin.GET("/loadbalancers/:id/stats", lbHandler.GetLoadBalancerStats)
			admin.POST("/loadbalancers/:id/check", lbHandler.RunHealthCheck)
		}

		// 用户接口 (需认证)
		authUser := v2.Group("")
		authUser.Use(middleware.JWTAuth())
		{
			// 用户转发规则
			forwardHandler := handler.NewForwardHandler()
			authUser.GET("/user/forward/rules", forwardHandler.GetUserRules)
			authUser.POST("/user/forward/rules", forwardHandler.CreateUserRule)

			// 用户支付
			paymentGatewayHandler := handler.NewPaymentGatewayHandler()
			authUser.GET("/user/payment/channels", paymentGatewayHandler.GetChannels)
			authUser.POST("/user/payment/create", paymentGatewayHandler.CreatePayment)
			authUser.GET("/user/payment/status/:trade_no", paymentGatewayHandler.GetPaymentStatus)
			authUser.GET("/user/payment/records", paymentGatewayHandler.GetUserRecords)

			// 用户 Telegram
			telegramHandler := handler.NewTelegramHandler()
			authUser.GET("/user/telegram/status", telegramHandler.GetTelegramStatus)
			authUser.POST("/user/telegram/unbind", telegramHandler.UnbindTelegram)
			authUser.POST("/user/telegram/notify", telegramHandler.UpdateNotifySettings)

			// 用户 MFA
			mfaHandler := handler.NewMFAHandler()
			authUser.GET("/user/mfa/status", mfaHandler.GetStatus)
			authUser.POST("/user/mfa/totp/setup", mfaHandler.SetupTOTP)
			authUser.POST("/user/mfa/totp/enable", mfaHandler.EnableTOTP)
			authUser.POST("/user/mfa/disable", mfaHandler.DisableMFA)
			authUser.POST("/user/mfa/verify", mfaHandler.VerifyMFA)
			authUser.POST("/user/mfa/backup-codes/regenerate", mfaHandler.RegenerateBackupCodes)

			// 用户通知
			notificationHandler := handler.NewNotificationHandler()
			authUser.GET("/user/notifications", notificationHandler.GetUserNotifications)
			authUser.GET("/user/notifications/unread-count", notificationHandler.GetUnreadCount)
			authUser.POST("/user/notifications/:id/read", notificationHandler.MarkAsRead)
			authUser.POST("/user/notifications/read-all", notificationHandler.MarkAllAsRead)

			// 用户邀请
			inviteHandler := handler.NewInviteHandler()
			authUser.GET("/user/invite", inviteHandler.GetInviteInfo)
			authUser.POST("/user/invite/generate", inviteHandler.GenerateCode)
			authUser.GET("/user/invite/commissions", inviteHandler.GetCommissionRecords)
			authUser.POST("/user/invite/withdraw", inviteHandler.RequestWithdraw)
			authUser.GET("/user/invite/withdrawals", inviteHandler.GetWithdrawRecords)
		}

		// Telegram Webhook (公开)
		telegramHandler := handler.NewTelegramHandler()
		v2.POST("/telegram/webhook", telegramHandler.TelegramWebhook)

		// 支付回调 (公开)
		paymentGatewayHandler := handler.NewPaymentGatewayHandler()
		v2.POST("/payment/callback/:type", paymentGatewayHandler.PaymentCallback)

		// 节点自动注册 API (公开)
		agentHandler := handler.NewAgentHandler()

		nodePublic := v2.Group("/node")
		{
			nodeHandler := handler.NewNodeHandler()
			nodePublic.POST("/register", nodeHandler.Register)
			nodePublic.GET("/ws", agentHandler.AgentWebSocketUnified)
		}

		// 节点通信 API (需要 API Key 认证 + 可选签名验证)
		nodeAPI := v2.Group("/node")
		nodeAPI.Use(middleware.NodeAPIKeyAuth())
		nodeAPI.Use(middleware.SignatureAuth())    // 签名验证 (向后兼容，可选)
		nodeAPI.Use(middleware.NodeSecureLogger()) // 安全审计日志
		{
			nodeHandler := handler.NewNodeHandler()
			nodeAPI.POST("/heartbeat", nodeHandler.Heartbeat)
		}

		// UniProxy API (节点通信接口)
		uniproxy := v2.Group("/server/UniProxy")
		uniproxy.Use(middleware.NodeAuth())
		{
			h := handler.NewUniProxyHandler()
			uniproxy.GET("/config", h.GetConfig)
			uniproxy.GET("/user", h.GetUsers)
			uniproxy.GET("/alivelist", h.GetAliveList)
			uniproxy.POST("/push", h.PushTraffic)
			uniproxy.POST("/alive", h.PushAlive)
		}

		// UniProxy API v1 (兼容旧版 V2bX)
		uniproxyV1 := r.Group("/api/v1/server/UniProxy")
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
		{
			agentPublic.POST("/register", agentHandler.AgentRegister)
			agentPublic.POST("/heartbeat", agentHandler.AgentHeartbeat)
			agentPublic.GET("/tasks", agentHandler.AgentGetTasks)
			agentPublic.POST("/result", agentHandler.AgentReportResult)
			agentPublic.POST("/monitor", agentHandler.AgentMonitor)
			agentPublic.GET("/ws", agentHandler.AgentWebSocketUnified)
		}

		// 转发规则同步 (Agent 使用)
		v2.GET("/forward/agent/rules", agentHandler.AgentGetForwardRules)

		// Agent 管理接口 (管理员)
		agentAdmin := v2.Group("/admin/agent")
		agentAdmin.Use(middleware.JWTAuth())
		agentAdmin.Use(middleware.AdminAuth())
		{
			agentAdmin.GET("/list", agentHandler.ListAgents)
			agentAdmin.POST("/tasks", agentHandler.CreateTask)
			agentAdmin.POST("/execute", agentHandler.ExecuteCommand)
		}
	}
}
