// Package native implements seven of the proxy-node package's v2 routes in
// the package itself. Legacy handlers and native routes share the rows, so a
// route can switch between them at any time, and the answers are
// byte-compatible with the legacy handlers (internal/tests/proxynodecompat).
//
//   - The load balancer list, detail, creation, update and deletion work on
//     the kernel's v2_load_balancer table, adopted in place
//     (kernel.storage.adopt). Only these routes use it.
//   - A node's runtime logs come from the adopted v2_node_log table, which
//     the kernel's agent control writes when nodes report logs.
//   - The node statistics, and whether a node exists, come from the kernel
//     view kapi_node_status_v1: each node's id, status, last check and
//     traffic counters.
//
// The package never reads v2_node or v2_node_protocol. v2_node holds each
// node's API key, key hash and shared secret, which the kernel's node
// authentication checks (UniProxy, the node API, the agent WebSocket and
// gRPC control stream, agent package downloads): a package that could read
// or write them could act as any node, and so read every subscriber's proxy
// credentials. Both are protected kernel tables. The other 24 routes stay
// bridged; packages/proxy-node/control lists why for each.
package native

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Pagination limits of the kernel's handler.ClampPagination.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the kapi_node_status_v1 view are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"proxy.loadbalancer.get":        s.ListLoadBalancers,
		"proxy.loadbalancer.post":       s.CreateLoadBalancer,
		"proxy.loadbalancer.id.get":     s.GetLoadBalancer,
		"proxy.loadbalancer.id.put":     s.UpdateLoadBalancer,
		"proxy.loadbalancer.id.delete":  s.DeleteLoadBalancer,
		"proxy.admin.nodes.stats.get":   s.GetNodeStats,
		"proxy.admin.nodes.id.logs.get": s.GetNodeLogs,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// jsonAnswer is the legacy handlers' c.JSON(code, body): encoding/json, as
// gin uses, with gin's content type.
func jsonAnswer(code int, body any) (pluginhostsdk.NativeResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: uint32(code), Body: encoded, // #nosec G115 -- HTTP status codes.
		Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}

// getQuery is gin's Context.GetQuery on the forwarded query string.
func getQuery(request pluginhostsdk.NativeRequest, key string) (string, bool) {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0], true
	}
	return "", false
}

// query is gin's Context.Query.
func query(request pluginhostsdk.NativeRequest, key string) string {
	value, _ := getQuery(request, key)
	return value
}

// defaultQuery is gin's Context.DefaultQuery.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if value, ok := getQuery(request, key); ok {
		return value
	}
	return fallback
}

// pathID parses the route's :id as the legacy handlers do.
func pathID(request pluginhostsdk.NativeRequest) (uint, bool) {
	id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	return uint(id), err == nil
}

// pagination reads page and page_size as the legacy handlers do, clamped by
// the kernel's handler.ClampPagination: a value that does not parse counts
// as what strconv.Atoi returns.
func pagination(request pluginhostsdk.NativeRequest) (int, int) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
