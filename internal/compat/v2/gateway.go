package v2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultRequestTimeout = 30 * time.Second
	defaultRequestBodyMax = 1 << 20
)

type Dispatcher interface {
	Dispatch(context.Context, pluginhost.DispatchInput) (pluginhost.DispatchOutput, error)
}

// Gateway is the unary v2 compatibility boundary. It has no legacy handler
// fallback: a route must resolve from an active, verified package artifact.
type Gateway struct {
	Registry         *Registry
	Dispatcher       Dispatcher
	Timeout          time.Duration
	RequestBodyLimit int64
}

func (g Gateway) Serve(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	route, err := g.resolve(c.Request.Context(), c.Request.Method, c.Request.URL.Path)
	if err != nil {
		writeResolutionError(c, err)
		return
	}
	if route.Transport != TransportHTTP {
		writeGatewayError(c, http.StatusUpgradeRequired, "package_route_requires_websocket", "package route requires a WebSocket connection")
		return
	}
	if g.Dispatcher == nil {
		writeGatewayError(c, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		return
	}
	body, err := readRequestBody(c.Request.Body, g.bodyLimit())
	if err != nil {
		writeGatewayError(c, http.StatusRequestEntityTooLarge, "plugin_request_too_large", "plugin request body exceeds its limit")
		return
	}
	principal, err := requestPrincipal(c, route.PackageID)
	if err != nil {
		writeGatewayError(c, http.StatusInternalServerError, "plugin_request_invalid", "plugin request principal could not be encoded")
		return
	}
	deadline := requestDeadline(c.Request.Context(), g.timeout())
	dispatchContext, cancel := context.WithDeadline(c.Request.Context(), deadline)
	defer cancel()
	response, err := g.Dispatcher.Dispatch(dispatchContext, pluginhost.DispatchInput{
		PackageID: route.PackageID, Version: route.Version, Generation: route.Generation,
		RequestID: requestID(c), IdempotencyKey: c.GetHeader("Idempotency-Key"), RouteID: route.PackageRoute,
		Method: c.Request.Method, Body: body, PrincipalJSON: principal, Metadata: requestMetadata(c), Deadline: deadline,
	})
	if err != nil {
		if errors.Is(err, pluginhost.ErrHostIncompatible) {
			writeGatewayError(c, http.StatusBadGateway, "plugin_host_incompatible", "plugin host is incompatible")
		} else {
			writeGatewayError(c, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		}
		return
	}
	if err := writePackageResponse(c, route, response); err != nil {
		writeGatewayError(c, http.StatusBadGateway, "plugin_host_incompatible", err.Error())
	}
}

func (g Gateway) resolve(ctx context.Context, method, requestPath string) (Route, error) {
	if g.Registry == nil {
		return Route{}, ErrPackageUnavailable
	}
	return g.Registry.ResolveContext(ctx, method, requestPath)
}

func (g Gateway) timeout() time.Duration {
	if g.Timeout <= 0 {
		return defaultRequestTimeout
	}
	return g.Timeout
}

func (g Gateway) bodyLimit() int64 {
	if g.RequestBodyLimit <= 0 {
		return defaultRequestBodyMax
	}
	return g.RequestBodyLimit
}

func readRequestBody(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	value, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(value)) > limit {
		return nil, errors.New("request body exceeds limit")
	}
	return value, nil
}

func requestPrincipal(c *gin.Context, packageID string) ([]byte, error) {
	return requestPrincipalFor(actorID(c), actorIsAdmin(c), packageID)
}

func requestPrincipalFor(actorID uint, admin bool, packageID string) ([]byte, error) {
	return json.Marshal(struct {
		ActorID uint   `json:"actor_id"`
		Admin   bool   `json:"admin"`
		Package string `json:"package_id"`
	}{
		ActorID: actorID, Admin: admin, Package: packageID,
	})
}

func requestMetadata(c *gin.Context) pluginhost.RequestMetadata {
	metadata := pluginhost.RequestMetadata{Path: c.Request.URL.Path}
	if clientIP := net.ParseIP(strings.TrimSpace(c.ClientIP())); clientIP != nil {
		metadata.ClientIP = clientIP.String()
	}
	if userAgent := c.Request.UserAgent(); validPackageUserAgent(userAgent) {
		metadata.UserAgent = userAgent
	}
	metadata.NodeID = nodeID(c)
	metadata.TrustedAgentWebSocketAuth = contextBool(c, agentws.TrustedContextKey)
	metadata.TrustedAgentWebSocketForwardNode = contextBool(c, agentws.ForwardNodeContextKey)
	if query := c.Request.URL.Query(); len(query) > 0 {
		metadata.Query = make(map[string][]string, len(query))
		for key, values := range query {
			if metadata.TrustedAgentWebSocketAuth && agentWebSocketCredentialQueryKey(key) {
				continue
			}
			metadata.Query[key] = append([]string(nil), values...)
		}
	}
	if headers := packageRequestHeaders(c.Request.Header); len(headers) > 0 {
		metadata.Headers = headers
	}
	if len(c.Params) > 0 {
		metadata.PathParams = make(map[string]string, len(c.Params))
		for _, parameter := range c.Params {
			metadata.PathParams[parameter.Key] = parameter.Value
		}
	}
	return metadata
}

func contextBool(c *gin.Context, key string) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(key)
	return ok && value == true
}

func agentWebSocketCredentialQueryKey(key string) bool {
	switch strings.ToLower(key) {
	case "node_id", "api_key", "token":
		return true
	default:
		return false
	}
}

func packageRequestHeaders(source http.Header) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	headers := make(map[string][]string)
	for name, values := range source {
		if !forwardPackageRequestHeader(name) || len(headers) >= 64 || len(values) > 32 {
			continue
		}
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "" || len(canonical) > 256 {
			continue
		}
		accepted := make([]string, 0, len(values))
		for _, value := range values {
			if len(value) == 0 || len(value) > 8192 || strings.ContainsAny(value, "\r\n\x00") {
				accepted = nil
				break
			}
			accepted = append(accepted, value)
		}
		if len(accepted) > 0 {
			headers[canonical] = accepted
		}
	}
	return headers
}

func forwardPackageRequestHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	switch name {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-app-token",
		"connection", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade", "content-length":
		return false
	default:
		return true
	}
}

func validPackageUserAgent(value string) bool {
	if value == "" || len(value) > 1024 {
		return false
	}
	for _, character := range value {
		if character == '\r' || character == '\n' || character == 0 {
			return false
		}
	}
	return true
}

func requestDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if requestDeadline, ok := ctx.Deadline(); ok && requestDeadline.Before(deadline) {
		return requestDeadline
	}
	return deadline
}

func requestID(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if id, ok := value.(string); ok && strings.TrimSpace(id) != "" {
			return id
		}
	}
	if id := strings.TrimSpace(c.GetHeader("X-Request-ID")); id != "" {
		return id
	}
	return uuid.NewString()
}

func actorID(c *gin.Context) uint {
	value, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return id
	case uint32:
		return uint(id)
	case int:
		if id > 0 {
			return uint(id)
		}
	case float64:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

func actorIsAdmin(c *gin.Context) bool {
	value, ok := c.Get("is_admin")
	return ok && value == true
}

func nodeID(c *gin.Context) uint {
	if c == nil {
		return 0
	}
	value, ok := c.Get("node_id")
	if !ok {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return id
	case uint32:
		return uint(id)
	case int:
		if id > 0 {
			return uint(id)
		}
	case float64:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

func writeResolutionError(c *gin.Context, err error) {
	if errors.Is(err, ErrPackageUnavailable) {
		writeGatewayError(c, http.StatusServiceUnavailable, "package_unavailable", "package route is unavailable")
		return
	}
	writeGatewayError(c, http.StatusNotFound, "package_route_not_found", "package route is not declared")
}

func writeGatewayError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
