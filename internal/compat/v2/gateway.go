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
	"github.com/AnixOps/anix-control/v4/internal/requestorigin"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
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

// LegacyDispatcher serves a request with the kernel's legacy handler for
// its route without the package host (pluginhost.Supervisor). The gateway
// uses it when it cannot seal a request's node secrets.
type LegacyDispatcher interface {
	DispatchLegacy(context.Context, pluginhost.DispatchInput) (pluginhost.DispatchOutput, error)
}

// Gateway is the unary v2 compatibility boundary. It has no legacy handler
// fallback: a route must resolve from an active, verified package artifact.
type Gateway struct {
	Registry         *Registry
	Dispatcher       Dispatcher
	Timeout          time.Duration
	RequestBodyLimit int64
	// Metrics receives one observation per request; nil selects
	// DefaultGatewayMetrics.
	Metrics *GatewayMetrics
	// Freeze pauses routes; nil selects DefaultRouteFreeze.
	Freeze *RouteFreeze
	// Sealer seals the node secrets of the routes in
	// config/node-secret-fields.json and expands the handles shown in their
	// answers; nil selects sealedsecrets.DefaultSealer.
	Sealer *sealedsecrets.Sealer
}

func (g Gateway) Serve(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	started := time.Now()
	route, errorCode := g.serve(c)
	g.metrics().Observe(route.PackageID, route.PackageRoute, c.Writer.Status(), errorCode, time.Since(started))
}

// serve handles one request and returns the resolved route (zero when
// resolution failed) and the gateway error code it wrote, if any.
func (g Gateway) serve(c *gin.Context) (Route, string) {
	route, err := g.resolve(c.Request.Context(), c.Request.Method, c.Request.URL.Path)
	if err != nil {
		return Route{}, writeResolutionError(c, err)
	}
	if route.Transport != TransportHTTP {
		return route, writeGatewayError(c, http.StatusUpgradeRequired, codeRouteRequiresWebSocket, "package route requires a WebSocket connection")
	}
	if g.Dispatcher == nil {
		return route, writeGatewayError(c, http.StatusBadGateway, codePluginHostUnavailable, "plugin host is unavailable")
	}
	release, admitted := g.freeze().enter(route.PackageID, route.PackageRoute)
	if !admitted {
		c.Header("Retry-After", "5")
		return route, writeGatewayError(c, http.StatusServiceUnavailable, codePackageRouteFrozen, "package route is paused for maintenance")
	}
	defer release()
	body, err := readRequestBody(c.Request.Body, g.bodyLimit())
	if err != nil {
		return route, writeGatewayError(c, http.StatusRequestEntityTooLarge, codePluginRequestTooLarge, "plugin request body exceeds its limit")
	}
	principal, err := requestPrincipal(c, route.PackageID)
	if err != nil {
		return route, writeGatewayError(c, http.StatusInternalServerError, codePluginRequestInvalid, "plugin request principal could not be encoded")
	}
	deadline := requestDeadline(c.Request.Context(), g.timeout())
	dispatchContext, cancel := context.WithDeadline(c.Request.Context(), deadline)
	defer cancel()
	input := pluginhost.DispatchInput{
		PackageID: route.PackageID, Version: route.Version, Generation: route.Generation,
		RequestID: requestID(c), IdempotencyKey: c.GetHeader("Idempotency-Key"), RouteID: route.PackageRoute,
		Method: c.Request.Method, Body: body, PrincipalJSON: principal, Metadata: requestMetadata(c), Deadline: deadline,
	}
	sealer := g.sealer()
	sealedKey := ""
	if sealer.Listed(route.PackageRoute) {
		// The package host reads the body with every node secret sealed; the
		// bridge capability keeps the original for the legacy handler. The
		// route is sealed in every mode: the kernel cannot tell when the
		// host's polled mode leaves legacy.
		sealed, err := sealer.SealRequest(sealedsecrets.RequestInput{
			PackageID: route.PackageID, Generation: route.Generation, RequestID: input.RequestID, RouteID: route.PackageRoute,
			Deadline: deadline, Body: body, PathParams: input.Metadata.PathParams, BodyLimit: g.bodyLimit(),
		})
		if err != nil {
			return g.serveLegacy(c, dispatchContext, route, input, sealedReason(err))
		}
		defer sealer.Release(sealed.Key)
		input.Body, input.BridgeBody, input.SealedRequest = sealed.Body, body, sealed.Key
		sealedKey = sealed.Key
		if sealed.Count > 0 {
			g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageRequest, sealedResultSealed, "")
		}
	}
	response, err := g.Dispatcher.Dispatch(dispatchContext, input)
	if err != nil {
		return route, writeDispatchError(c, err)
	}
	expanded, count, err := sealer.ExpandAnswer(sealedKey, route.PackageRoute, response.Body, answerHeaders(response.Headers))
	if err != nil {
		// A handle never reaches a client, and the native handler may have
		// acted: the request is not served again.
		g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageAnswer, sealedResultRefused, sealedReason(err))
		return route, writeGatewayError(c, http.StatusBadGateway, codeSealedSecretRefused, "plugin answer holds a sealed secret handle it cannot show")
	}
	if count > 0 {
		response.Body = expanded
		g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageAnswer, sealedResultExpanded, "")
	}
	if err := writePackageResponse(c, route, response); err != nil {
		return route, writeGatewayError(c, http.StatusBadGateway, codePluginHostIncompatible, err.Error())
	}
	return route, ""
}

// serveLegacy serves a request whose node secrets could not be sealed with
// the kernel's legacy handler, without the package host, so no secret
// reaches the package. Without one it is refused.
func (g Gateway) serveLegacy(c *gin.Context, ctx context.Context, route Route, input pluginhost.DispatchInput, reason string) (Route, string) {
	legacy, ok := g.Dispatcher.(LegacyDispatcher)
	if !ok {
		g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageRequest, sealedResultRefused, reason)
		return route, writeGatewayError(c, http.StatusServiceUnavailable, codeSealedSecretUnavailable, "package route cannot seal its node secrets")
	}
	response, err := legacy.DispatchLegacy(ctx, input)
	if errors.Is(err, pluginhost.ErrLegacyUnavailable) {
		g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageRequest, sealedResultRefused, reason)
		return route, writeGatewayError(c, http.StatusServiceUnavailable, codeSealedSecretUnavailable, "package route cannot seal its node secrets")
	}
	g.metrics().ObserveSealed(route.PackageID, route.PackageRoute, sealedStageRequest, sealedResultLegacy, reason)
	if err != nil {
		return route, writeDispatchError(c, err)
	}
	if err := writePackageResponse(c, route, response); err != nil {
		return route, writeGatewayError(c, http.StatusBadGateway, codePluginHostIncompatible, err.Error())
	}
	return route, ""
}

func writeDispatchError(c *gin.Context, err error) string {
	switch {
	case errors.Is(err, pluginhost.ErrResponseTooLarge):
		return writeGatewayError(c, http.StatusBadGateway, codePluginResponseTooLarge, "plugin response exceeds its limit")
	case errors.Is(err, pluginhost.ErrHostIncompatible):
		return writeGatewayError(c, http.StatusBadGateway, codePluginHostIncompatible, "plugin host is incompatible")
	default:
		return writeGatewayError(c, http.StatusBadGateway, codePluginHostUnavailable, "plugin host is unavailable")
	}
}

func answerHeaders(headers []pluginhost.Header) []sealedsecrets.Header {
	if len(headers) == 0 {
		return nil
	}
	converted := make([]sealedsecrets.Header, len(headers))
	for index, header := range headers {
		converted[index] = sealedsecrets.Header{Name: header.Name, Value: header.Value}
	}
	return converted
}

func (g Gateway) sealer() *sealedsecrets.Sealer {
	if g.Sealer == nil {
		return sealedsecrets.DefaultSealer()
	}
	return g.Sealer
}

func (g Gateway) metrics() *GatewayMetrics {
	if g.Metrics == nil {
		return DefaultGatewayMetrics()
	}
	return g.Metrics
}

func (g Gateway) freeze() *RouteFreeze {
	if g.Freeze == nil {
		return DefaultRouteFreeze()
	}
	return g.Freeze
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
	// The original request's scheme, host and client address: forwarding
	// headers count only from a trusted reverse proxy
	// (server.trusted_proxies). The package host receives them as
	// request_scheme and request_host, and the forwarding headers
	// themselves are not passed on (packageRequestHeaders).
	origin := requestorigin.Resolve(c.Request)
	metadata := pluginhost.RequestMetadata{Path: c.Request.URL.Path, TLS: origin.Scheme == "https"}
	if validPackageRequestHost(origin.Host) {
		metadata.Host = origin.Host
	}
	if clientIP := net.ParseIP(origin.ClientIP); clientIP != nil {
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
	if name == "" || requestorigin.IsForwardingHeader(name) {
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

// validPackageRequestHost accepts a host[:port] authority: the characters of a
// registered name, an IPv4 address, or a bracketed IPv6 literal.
func validPackageRequestHost(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("-._~:[]", character) {
			continue
		}
		return false
	}
	return true
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

func writeResolutionError(c *gin.Context, err error) string {
	if errors.Is(err, ErrPackageUnavailable) {
		return writeGatewayError(c, http.StatusServiceUnavailable, codePackageUnavailable, "package route is unavailable")
	}
	return writeGatewayError(c, http.StatusNotFound, codeRouteNotFound, "package route is not declared")
}

func writeGatewayError(c *gin.Context, status int, code, message string) string {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
	return code
}
