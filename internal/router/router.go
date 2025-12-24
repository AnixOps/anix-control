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

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 认证接口 (无需登录)
		authHandler := handler.NewAuthHandler(cfg)
		v1.POST("/login", authHandler.Login)
		v1.POST("/register", authHandler.Register)

		// 支付接口
		paymentHandler := handler.NewPaymentHandler()
		v1.GET("/payment/methods", paymentHandler.GetPaymentMethods)
		v1.GET("/payment/status/:trade_no", paymentHandler.GetPaymentStatus)

		// 支付回调 (无需认证)
		v1.POST("/payment/x402/callback", paymentHandler.X402Callback)
		v1.POST("/payment/stripe/webhook", paymentHandler.StripeWebhook)
		v1.POST("/payment/paypal/webhook", paymentHandler.PayPalWebhook)

		// 需要登录的接口
		auth := v1.Group("")
		auth.Use(middleware.JWTAuth())
		{
			// 用户接口
			userHandler := handler.NewUserHandler()
			auth.GET("/user/profile", userHandler.GetProfile)
			auth.GET("/user/dashboard", userHandler.GetDashboard)
			auth.GET("/user/subscription", userHandler.GetSubscription)

			// 用户支付接口
			auth.POST("/payment/x402/create", paymentHandler.X402CreatePayment)
			auth.GET("/payment/x402/check/:id", paymentHandler.X402CheckPayment)
			auth.POST("/payment/fiat/create", paymentHandler.FiatCreatePayment)
		}

		// 管理员接口
		admin := v1.Group("/admin")
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
		}

		// 节点自动注册 API (公开)
		nodePublic := v1.Group("/node")
		{
			nodeHandler := handler.NewNodeHandler()
			nodePublic.POST("/register", nodeHandler.Register)
		}

		// 节点通信 API (需要 API Key 认证)
		nodeAPI := v1.Group("/node")
		nodeAPI.Use(middleware.NodeAPIKeyAuth())
		{
			nodeHandler := handler.NewNodeHandler()
			nodeAPI.POST("/heartbeat", nodeHandler.Heartbeat)
		}

		// UniProxy API (节点通信接口 - 旧版兼容)
		uniproxy := v1.Group("/server/UniProxy")
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
