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
// checks stay in the kernel because only it can make them: a DELETE, and
// a write of a DNS provider (whose credentials it carries), needs a super
// administrator (as route-mode switches and install tokens do), and the
// paths config/editions.json reserves for the commercial edition do not
// exist in the community edition. The list answers (GET /routes, /nodes,
// /ansible-machines) learn the first rule through the principal's
// super_admin, so a UI shows delete only to those who may.

const (
	// ForwardV4Prefix is the forward package's v4 API.
	ForwardV4Prefix = "/api/v4/forward"
	// forwardV4ControlPrefix is the manifest control route namespace the
	// package declares (/api/v4/plugins/forward/*).
	forwardV4ControlPrefix = "/api/v4/plugins/forward"
	forwardPackageID       = "forward"
	// pluginPrincipalSuperAdminKey is the gin context key the forward
	// gateway sets when the principal it dispatches carries super_admin.
	pluginPrincipalSuperAdminKey = "plugin_principal_super_admin"
)

// forwardV4ListPaths are the list answers that carry can_delete (F5b D7):
// the kernel tells the package whether the caller is a super
// administrator, the rule every DELETE below them needs.
var forwardV4ListPaths = map[string]struct{}{
	forwardV4ControlPrefix + "/routes":           {},
	forwardV4ControlPrefix + "/nodes":            {},
	forwardV4ControlPrefix + "/ansible-machines": {},
}

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

// forwardV4DNSProviders is where DNS provider credentials are written
// (entry HA, forward-sdk.md section 7.4, L2).
const forwardV4DNSProviders = ForwardV4Prefix + "/dns/providers"

// forwardV4NeedsSuperAdmin: every DELETE, and every write of a DNS
// provider (its credentials), needs a super administrator.
func forwardV4NeedsSuperAdmin(method, requestPath string) bool {
	if method == http.MethodDelete {
		return true
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return requestPath == forwardV4DNSProviders || strings.HasPrefix(requestPath, forwardV4DNSProviders+"/")
}

// ForwardGateway serves /api/v4/forward/*.
func (h *KernelHandler) ForwardGateway(c *gin.Context) {
	// The v4.1 forwarding archive is the kernel's own (F5c).
	if c.Request.URL.Path == forwardLegacyArchivePath {
		h.ForwardLegacyArchive(c)
		return
	}
	if edition.For(config.Get()).HidesPath(c.Request.URL.Path) {
		kernelError(c, http.StatusNotFound, "plugin_route_not_found", "route not found")
		return
	}
	controlPath, ok := forwardV4ControlPath(c.Request.URL.Path)
	if !ok {
		kernelError(c, http.StatusNotFound, "plugin_route_not_found", "route not found")
		return
	}
	_, isList := forwardV4ListPaths[strings.TrimSuffix(controlPath, "/")]
	isList = isList && c.Request.Method == http.MethodGet
	needsSuperAdmin := forwardV4NeedsSuperAdmin(c.Request.Method, c.Request.URL.Path)
	if needsSuperAdmin || isList {
		allowed, err := service.IsSuperAdmin(h.db, kernelActorID(c))
		if err != nil {
			kernelDBError(c, err)
			return
		}
		if needsSuperAdmin && !allowed {
			kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may delete forward resources or change DNS provider credentials")
			return
		}
		if isList {
			// The package answers can_delete from it (F5b D7).
			c.Set(pluginPrincipalSuperAdminKey, allowed)
		}
	}
	h.dispatchPluginControlRoute(c, forwardPackageID, controlPath)
}

// forwardKernelChecks applies ForwardGateway's kernel checks to the forward
// package's own spelling, /api/v{3,4}/plugins/forward/*, so neither the
// super administrator rule nor the community edition's hidden prefixes can
// be bypassed through it. It answers false after refusing the request.
func (h *KernelHandler) forwardKernelChecks(c *gin.Context) bool {
	rest := c.Param("route")
	if rest == "" {
		rest = "/"
	}
	publicPath := ForwardV4Prefix + rest
	if edition.For(config.Get()).HidesPath(publicPath) {
		kernelError(c, http.StatusNotFound, "plugin_route_not_found", "route not found")
		return false
	}
	if !forwardV4NeedsSuperAdmin(c.Request.Method, publicPath) {
		return true
	}
	allowed, err := service.IsSuperAdmin(h.db, kernelActorID(c))
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if !allowed {
		kernelError(c, http.StatusForbidden, "super_admin_required", "only a super administrator may delete forward resources or change DNS provider credentials")
		return false
	}
	return true
}
