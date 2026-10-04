package handler

import (
	"net/http"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/edition"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// The forward package's v4 administrator API (forward-sdk.md section 3,
// F5a). The package owns /api/v4/forward/*: the kernel resolves and
// dispatches it as the package's manifest control route
// /api/v4/plugins/forward/*, so the package host answers it on
// ForwardControl (kernel.forward.v1) like any other control route. Two
// checks stay in the kernel because only it can make them: a DELETE
// needs a super administrator (as route-mode switches and install tokens
// do), and the paths config/editions.json reserves for the commercial
// edition do not exist in the community edition.

const (
	// ForwardV4Prefix is the forward package's v4 API.
	ForwardV4Prefix = "/api/v4/forward"
	// forwardV4ControlPrefix is the manifest control route namespace the
	// package declares (/api/v4/plugins/forward/*).
	forwardV4ControlPrefix = "/api/v4/plugins/forward"
	forwardPackageID       = "forward"
)

// forwardV4ControlPath maps a /api/v4/forward path to the package's
// control route path: /api/v4/forward/routes/x is
// /api/v4/plugins/forward/routes/x.
func forwardV4ControlPath(requestPath string) (string, bool) {
	rest, ok := strings.CutPrefix(requestPath, ForwardV4Prefix)
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return "", false
	}
	return forwardV4ControlPrefix + rest, true
}

// ForwardGateway serves /api/v4/forward/*.
func (h *KernelHandler) ForwardGateway(c *gin.Context) {
	if edition.For(config.Get()).HidesPath(c.Request.URL.Path) {
		kernelError(c, http.StatusNotFound, "plugin_route_not_found", "route not found")
		return
	}
	controlPath, ok := forwardV4ControlPath(c.Request.URL.Path)
	if !ok {
		kernelError(c, http.StatusNotFound, "plugin_route_not_found", "route not found")
		return
	}
	if c.Request.Method == http.MethodDelete {
		allowed, err := service.IsSuperAdmin(h.db, kernelActorID(c))
		if err != nil {
			kernelDBError(c, err)
			return
		}
		if !allowed {
			kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may delete forward routes and nodes")
			return
		}
	}
	h.dispatchPluginControlRoute(c, forwardPackageID, controlPath)
}
