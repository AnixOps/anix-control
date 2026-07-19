package fixture

import "github.com/gin-gonic/gin"

func Setup(r *gin.Engine) {
	api := r.Group("/api")
	v2 := api.Group("/v2")

	public := v2.Group("")
	public.Use(publicLimiter.Middleware())
	public.GET("/login", authHandler.Login)

	user := v2.Group("")
	user.Use(userLimiter.Middleware())
	user.Use(middleware.JWTAuth())
	private := user.Group("/private")
	private.POST("/item/:id", itemHandler.Create)

	admin := v2.Group("/admin", adminLimiter.Middleware())
	admin.Use(middleware.JWTAuth())
	admin.Use(middleware.AdminAuth())
	admin.DELETE("/users/:id", adminHandler.Delete)

	v2.Any(
		"/fanout",
		fanoutHandler.Handle,
	)
	v2.PATCH("/patch", patchHandler.Update)
	v2.GET("/inline-auth", middleware.JWTAuth(), inlineHandler.Get)
	r.PUT("/api/v2/direct/:id", directHandler.Update)

	// v2.GET("/comment-only", decoyHandler.Get)
	_ = `v2.POST("/string-only", decoyHandler.Post)`
}

func ParentGroupUse(r *gin.Engine) {
	api := r.Group("/api")
	api.Use(middleware.NodeAuth())
	v2 := api.Group("/v2")
	v2.GET("/inherited-node-auth", inheritedHandler.Get)
}

func ParentGroupInlineMiddleware(r *gin.Engine) {
	api := r.Group("/api", middleware.NodeAuth())
	v2 := api.Group("/v2")
	v2.GET("/inherited-inline-node-auth", inheritedHandler.Inline)
}

func ChainedRoutes(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	v2.Use(middleware.JWTAuth()).GET("/chained-use", chainedHandler.Get)
	v2.Handle("PATCH", "/handled", handledHandler.Patch)
	v2.Match([]string{"PATCH", "DELETE"}, "/matched", matchedHandler.Patch)
}
