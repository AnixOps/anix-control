// Package native implements the proxy-node package's v2 routes in the
// package itself, but for the node channel and three routes that need what
// no kernel contract offers yet. Legacy handlers and native routes share
// the rows, so a route can switch between them at any time, and the answers
// are byte-compatible with the legacy handlers
// (internal/tests/proxynodecompat).
//
//   - The load balancer list, detail, creation, update and deletion work on
//     the kernel's v2_load_balancer table, adopted in place
//     (kernel.storage.adopt). A load balancer's statistics read the forward
//     nodes through kapi_forward_node_v1, and its health check is the
//     kernel's (CheckEndpoints with record_status).
//   - A node's runtime logs come from the adopted v2_node_log table, which
//     the kernel's agent control writes when nodes report logs.
//   - The node statistics, and whether a node exists, come from the kernel
//     view kapi_node_status_v1: each node's id, status, last check and
//     traffic counters.
//   - The node list, detail, deletion and raw configuration work on
//     v2_node, which the package adopts once the node credential split
//     finalized it (a grant the kernel honours only then): its API key, key
//     hash and shared secret hold tombstones, and its raw configuration the
//     placeholder at every secret position. A node's protocols are read
//     through kapi_node_protocol_public_v1. A deletion retires what the
//     kernel holds for the node (RetireNode); a raw configuration is stored
//     by the kernel (PutSecretDocument), its typed secrets as sealed handles
//     the package never resolves. Until the lease grants them, these routes
//     answer from the legacy handler.
//   - Registration keys are the kernel's: listed through
//     kapi_registration_key_v1 (never a key), issued and revoked through
//     KernelNodeOps, a new key answered as a sealed handle.
//
// The package never reads a node's credentials, and never reads or writes
// v2_node_protocol, which is protocol-runtime's. The other routes stay in
// the kernel; packages/proxy-node/control lists why for each.
package native

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
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
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Leased reports whether the package's storage lease adopts the kernel
	// table, or grants the kernel view, called name
	// (packagestoresdk.Store.Leased); nil leaves the node and registration
	// key routes that need a conditional grant legacy.
	Leased func(ctx context.Context, name string) bool
	// NodeOps is the kernel's KernelNodeOps; without it the routes that
	// need it stay legacy.
	NodeOps kernelnodeopsv1.KernelNodeOpsClient
	// Now defaults to time.Now.
	Now func() time.Time
	// NewToken names a request that carries neither an Idempotency-Key nor
	// an X-Request-ID, in its operations' request ids; it defaults to a
	// random UUID.
	NewToken func() string
}

// Route ids of the native routes of M3-2.
const (
	NodesRouteID             = "proxy.admin.nodes.get"
	NodeRouteID              = "proxy.admin.nodes.id.get"
	DeleteNodeRouteID        = "proxy.admin.nodes.id.delete"
	RawConfigRouteID         = "proxy.admin.nodes.id.raw_config.get"
	UpdateRawConfigRouteID   = "proxy.admin.nodes.id.raw_config.put"
	AuthKeysRouteID          = "proxy.admin.auth_keys.get"
	GenerateAuthKeyRouteID   = "proxy.admin.auth_keys.post"
	DeleteAuthKeyRouteID     = "proxy.admin.auth_keys.id.delete"
	InternalAuthKeyRouteID   = "proxy.internal.auth_keys.post"
	LoadBalancerStatsRouteID = "proxy.loadbalancer.id.stats.get"
	LoadBalancerCheckRouteID = "proxy.loadbalancer.id.check.post"
)

// Handlers returns the native handlers by route id. A host without
// KernelNodeOps has none for the routes that need it.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"proxy.loadbalancer.get":        s.ListLoadBalancers,
		"proxy.loadbalancer.post":       s.CreateLoadBalancer,
		"proxy.loadbalancer.id.get":     s.GetLoadBalancer,
		"proxy.loadbalancer.id.put":     s.UpdateLoadBalancer,
		"proxy.loadbalancer.id.delete":  s.DeleteLoadBalancer,
		"proxy.admin.nodes.stats.get":   s.GetNodeStats,
		"proxy.admin.nodes.id.logs.get": s.GetNodeLogs,
		NodesRouteID:                    s.ListNodes,
		NodeRouteID:                     s.GetNode,
		RawConfigRouteID:                s.GetRawConfig,
		AuthKeysRouteID:                 s.ListAuthKeys,
		LoadBalancerStatsRouteID:        s.LoadBalancerStats,
	}
	if s.NodeOps != nil {
		handlers[DeleteNodeRouteID] = s.DeleteNode
		handlers[UpdateRawConfigRouteID] = s.UpdateRawConfig
		handlers[GenerateAuthKeyRouteID] = s.GenerateAuthKey
		handlers[DeleteAuthKeyRouteID] = s.DeleteAuthKey
		handlers[InternalAuthKeyRouteID] = s.InternalGenerateAuthKey
		handlers[LoadBalancerCheckRouteID] = s.RunHealthCheck
	}
	return handlers
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
