package packagebridge

import (
	"errors"
	"sync"

	"github.com/gin-gonic/gin"
)

type registeredRoute struct {
	packageID string
	handler   OperationHandler
}

type registeredWebSocketRoute struct {
	packageID string
	handler   WebSocketOperationHandler
}

// RouteRegistry is populated only while Gin routes are constructed. It keeps
// the legacy-compatible handler behind the kernel's exact bridge operation
// checks; it does not inspect package-controlled paths or route declarations.
type RouteRegistry struct {
	mu              sync.RWMutex
	routes          map[string]registeredRoute
	webSocketRoutes map[string]registeredWebSocketRoute
	owners          map[string]string
}

var defaultRouteRegistry = NewRouteRegistry()

func NewRouteRegistry() *RouteRegistry {
	return &RouteRegistry{
		routes: make(map[string]registeredRoute), webSocketRoutes: make(map[string]registeredWebSocketRoute), owners: make(map[string]string),
	}
}

// DefaultRouteRegistry is shared by the production router and its host
// supervisor. Tests may use NewRouteRegistry for isolated contracts.
func DefaultRouteRegistry() *RouteRegistry { return defaultRouteRegistry }

// Register binds an exact package route to one Gin handler. Rebuilding a
// router may replace the handler for the same package/route, but a route id
// can never be rebound to another package.
func (r *RouteRegistry) Register(packageID, routeID string, handler gin.HandlerFunc) error {
	if r == nil || !safeIdentifier(packageID) || !safeIdentifier(routeID) || handler == nil {
		return errors.New("package bridge route registration is invalid")
	}
	key := operationKey(packageID, routeID, routeID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMapsLocked()
	if existing, ok := r.owners[routeID]; ok && existing != packageID {
		return errors.New("package bridge route is already owned by another package")
	}
	r.owners[routeID] = packageID
	r.routes[key] = registeredRoute{packageID: packageID, handler: NewHTTPAdapter(handler)}
	return nil
}

// MustRegister is for static router declarations. A bad declaration is a
// process-startup error, not a recoverable request-time condition.
func (r *RouteRegistry) MustRegister(packageID, routeID string, handler gin.HandlerFunc) {
	if err := r.Register(packageID, routeID, handler); err != nil {
		panic(err)
	}
}

func (r *RouteRegistry) Resolve(packageID, routeID, operation string) (OperationHandler, bool) {
	if r == nil || operation != routeID {
		return nil, false
	}
	r.mu.RLock()
	entry, ok := r.routes[operationKey(packageID, routeID, operation)]
	r.mu.RUnlock()
	if !ok || entry.packageID != packageID {
		return nil, false
	}
	return entry.handler, true
}

// RegisterWebSocket binds an exact package route to a legacy-compatible
// WebSocket handler. It shares ownership checks with unary routes.
func (r *RouteRegistry) RegisterWebSocket(packageID, routeID string, handler gin.HandlerFunc) error {
	if r == nil || !safeIdentifier(packageID) || !safeIdentifier(routeID) || handler == nil {
		return errors.New("package bridge WebSocket route registration is invalid")
	}
	key := operationKey(packageID, routeID, routeID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMapsLocked()
	if existing, ok := r.owners[routeID]; ok && existing != packageID {
		return errors.New("package bridge route is already owned by another package")
	}
	r.owners[routeID] = packageID
	r.webSocketRoutes[key] = registeredWebSocketRoute{packageID: packageID, handler: NewWebSocketAdapter(handler)}
	return nil
}

// MustRegisterWebSocket is for static router declarations. Invalid ownership
// is a process-startup error, not a request-time fallback condition.
func (r *RouteRegistry) MustRegisterWebSocket(packageID, routeID string, handler gin.HandlerFunc) {
	if err := r.RegisterWebSocket(packageID, routeID, handler); err != nil {
		panic(err)
	}
}

func (r *RouteRegistry) ResolveWebSocket(packageID, routeID, operation string) (WebSocketOperationHandler, bool) {
	if r == nil || operation != routeID {
		return nil, false
	}
	r.mu.RLock()
	entry, ok := r.webSocketRoutes[operationKey(packageID, routeID, operation)]
	r.mu.RUnlock()
	if !ok || entry.packageID != packageID {
		return nil, false
	}
	return entry.handler, true
}

func (r *RouteRegistry) ensureMapsLocked() {
	if r.routes == nil {
		r.routes = make(map[string]registeredRoute)
	}
	if r.webSocketRoutes == nil {
		r.webSocketRoutes = make(map[string]registeredWebSocketRoute)
	}
	if r.owners == nil {
		r.owners = make(map[string]string)
	}
}

// Allowlist exposes the registered routes through the same exact-operation
// interface used by static bridge handlers.
func (r *RouteRegistry) Allowlist() *Allowlist {
	allowlist, err := NewAllowlistWithFallback(r)
	if err != nil {
		panic(err)
	}
	return allowlist
}
