package router

import (
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/handler"
	"github.com/anixops/v2board/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Setup 设置路由
func Setup(r *gin.Engine, cfg *config.Config) {
	// 全局中间件
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

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
		}

		// 节点自动注册 API (公开)
		nodePublic := v2.Group("/node")
		{
			nodeHandler := handler.NewNodeHandler()
			nodePublic.POST("/register", nodeHandler.Register)
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
	}
}
