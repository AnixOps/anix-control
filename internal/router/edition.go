package router

import (
	"strings"

	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/edition"
	"github.com/gin-gonic/gin"
)

// editionRouteFilter is the one place the configured edition (app.edition)
// hides routes: a request that matched an /api/v2 route whose package or
// route id config/editions.json marks commercial is answered, in the
// community edition, exactly as a route that no package declares. It runs
// before authentication and rate limits, as the unknown-route answer does.
func editionRouteFilter(policy *edition.Policy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if policy.HidesRequest(c.Request.Method, c.FullPath()) {
			compatv2.WriteRouteNotFound(c)
			return
		}
		c.Next()
	}
}

// SetupNotFound makes an /api/v2 path that matches no route answer like a
// route the edition hides, so the two cannot be told apart. Call it after
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
