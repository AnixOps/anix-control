package router

import (
	"github.com/gin-gonic/gin"

	"github.com/AnixOps/anix-control/v5/internal/config"
)

// A route contributor adds the /api/v2 routes of a package that is not part
// of the community set. SetupContributed runs them after Setup, so a file that
// registers one (router_commercial.go) can be left out of a build without
// touching Setup. The v2 route inventory reads Setup and every
// router_*.go file next to it.

var routeContributors []func(r *gin.Engine, cfg *config.Config)

// registerRouteContributor adds a contributor; it is meant for init
// functions of files that a build may leave out.
//nolint:unused // called by files only the commercial build adds (router_commercial.go)
func registerRouteContributor(fn func(r *gin.Engine, cfg *config.Config)) {
	routeContributors = append(routeContributors, fn)
}

// SetupContributed registers the routes of the contributors. Call it after
// Setup on the same engine.
func SetupContributed(r *gin.Engine, cfg *config.Config) {
	for _, fn := range routeContributors {
		fn(r, cfg)
	}
}
