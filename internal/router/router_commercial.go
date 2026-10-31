package router

// The commercial routes: payment, orders, coupons and invite commission.
// This file is the whole of their link into the router: a build without it
// (the community edition) serves none of them. config/commercial-boundary.json
// lists it with the rest of the set.

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/middleware"
)

func init() {
	registerRouteContributor(setupCommercialRoutes)
}

// setupCommercialRoutes binds the commercial routes to the package gateway
// behind the same limiters and authentication stacks Setup gives the other
// /api/v2 routes.
func setupCommercialRoutes(r *gin.Engine, cfg *config.Config) {
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
	adminLimiter.StartCleanup(1 * time.Minute)
	userLimiter.StartCleanup(1 * time.Minute)
	publicLimiter.StartCleanup(1 * time.Minute)
	adminAPIToken := middleware.AdminAPIToken()

	v2 := r.Group("/api/v2")
	v2PackageGateway := handler.NewV2CompatibilityGateway(cfg, handler.NewV2RouteCache())
	{
		// Payment status and the payment provider callbacks (no sign-in).
		v2.GET("/payment/methods", v2PackageGateway.Serve)
		v2.GET("/payment/status/:trade_no", v2PackageGateway.Serve)
		v2.POST("/payment/x402/callback", v2PackageGateway.Serve)
		v2.POST("/payment/stripe/webhook", v2PackageGateway.Serve)
		v2.POST("/payment/paypal/webhook", v2PackageGateway.Serve)

		paymentCallback := v2.Group("")
		paymentCallback.Use(publicLimiter.Middleware())
		{
			paymentCallback.POST("/payment/callback/:type", v2PackageGateway.Serve)
		}

		auth := v2.Group("")
		auth.Use(userLimiter.Middleware())
		auth.Use(middleware.JWTAuth())
		{
			auth.POST("/user/coupon/check", v2PackageGateway.Serve)
			auth.GET("/user/order", v2PackageGateway.Serve)
			auth.POST("/user/order/save", v2PackageGateway.Serve)
			auth.GET("/user/order/:id", v2PackageGateway.Serve)
			auth.POST("/payment/x402/create", v2PackageGateway.Serve)
			auth.GET("/payment/x402/check/:id", v2PackageGateway.Serve)
			auth.POST("/payment/fiat/create", v2PackageGateway.Serve)
		}

		admin := v2.Group("/admin")
		admin.Use(adminLimiter.Middleware())
		admin.Use(adminAPIToken)
		admin.Use(middleware.JWTAuth())
		admin.Use(middleware.AdminAuth())
		admin.Use(middleware.AuditLog())
		{
			admin.GET("/orders", v2PackageGateway.Serve)
			admin.GET("/orders/stats", v2PackageGateway.Serve)
			admin.GET("/orders/:id", v2PackageGateway.Serve)
			admin.PUT("/orders/:id/status", v2PackageGateway.Serve)
			admin.POST("/orders/:id/paid", v2PackageGateway.Serve)
			admin.POST("/orders/:id/cancel", v2PackageGateway.Serve)
			admin.GET("/coupon", v2PackageGateway.Serve)
			admin.POST("/coupon", v2PackageGateway.Serve)
			admin.DELETE("/coupon/:id", v2PackageGateway.Serve)
			admin.GET("/payment/gateways", v2PackageGateway.Serve)
			admin.POST("/payment/gateways", v2PackageGateway.Serve)
			admin.PUT("/payment/gateways/:id", v2PackageGateway.Serve)
			admin.DELETE("/payment/gateways/:id", v2PackageGateway.Serve)
			admin.POST("/payment/gateways/:id/toggle", v2PackageGateway.Serve)
			admin.GET("/payment/stats", v2PackageGateway.Serve)
			admin.GET("/payment/records", v2PackageGateway.Serve)
			admin.GET("/invite/codes", v2PackageGateway.Serve)
			admin.POST("/invite/codes", v2PackageGateway.Serve)
			admin.DELETE("/invite/codes/:id", v2PackageGateway.Serve)
			admin.GET("/invite/config", v2PackageGateway.Serve)
			admin.PUT("/invite/config", v2PackageGateway.Serve)
			admin.GET("/invite/stats", v2PackageGateway.Serve)
			admin.GET("/invite/withdrawals", v2PackageGateway.Serve)
			admin.POST("/invite/withdrawals/:id/process", v2PackageGateway.Serve)
		}

		authUser := v2.Group("")
		authUser.Use(userLimiter.Middleware())
		authUser.Use(middleware.JWTAuth())
		{
			authUser.GET("/user/payment/channels", v2PackageGateway.Serve)
			authUser.POST("/user/payment/create", v2PackageGateway.Serve)
			authUser.GET("/user/payment/status/:trade_no", v2PackageGateway.Serve)
			authUser.GET("/user/payment/records", v2PackageGateway.Serve)
			authUser.GET("/user/invite", v2PackageGateway.Serve)
			authUser.POST("/user/invite/generate", v2PackageGateway.Serve)
			authUser.GET("/user/invite/commissions", v2PackageGateway.Serve)
			authUser.POST("/user/invite/withdraw", v2PackageGateway.Serve)
			authUser.GET("/user/invite/withdrawals", v2PackageGateway.Serve)
		}
	}
}
