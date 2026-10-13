package router

import (
	"strings"

	compatv2 "github.com/AnixOps/anix-control/v5/internal/compat/v2"
	"github.com/gin-gonic/gin"
)

// SetupNotFound makes an /api/v2 path that matches no route answer like a
// route no package declares (the gateway's unknown-route body). Call it after
// Setup on the API engine. It is outside Setup because the v2 route
// inventory (config/scripts/v2_route_inventory.go) reads Setup and accepts
// only route registrations there.
func SetupNotFound(r *gin.Engine) {
	r.NoRoute(apiV2NoRoute)
}

// apiV2NoRoute answers an /api/v2 path that matches no route with the
// gateway's unknown-route body. Other paths keep gin's default answer.
func apiV2NoRoute(c *gin.Context) {
	if c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/api/v2/") {
		compatv2.WriteRouteNotFound(c)
	}
}

// WriteAPIV2NotFound is apiV2NoRoute's answer, for an engine whose NoRoute
// also serves the web app (the panel handler of cmd/server): an /api/v2
// path that matches no route answers as SetupNotFound makes it.
func WriteAPIV2NotFound(c *gin.Context) {
	compatv2.WriteRouteNotFound(c)
}
