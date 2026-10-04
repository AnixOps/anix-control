package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/forward/v4api"
	"google.golang.org/grpc"
)

// newForwardService returns the forward host's router. Since v4.2 (F5d)
// the package serves no v2 route natively and adopts no table: every v2
// route it declares (bridgedRoutes) relays to the kernel's legacy handler,
// whatever mode is stored for it, until the legacy cleanup (F5c) drops the
// flux tables and removes the routes.
//
// The package's own control route, v4api.Route (/api/v4/forward/* to its
// callers), is the v4 administrator API on the kernel's ForwardControl
// over the bridge connection (local socket or module listener; v4api). It
// has no legacy handler and no route mode: the host always answers it, 503
// without the connection.
func newForwardService(bridge pluginhostsdk.RouterBridge, leaseID string) (*forwardHost, error) {
	api := &v4api.Service{}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		api.Forward = forwardv1.NewForwardControlClient(conn.Conn())
	}
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "forward", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, bridgedRoute := bridgedRoutes[routeID]
			return bridgedRoute || routeID == v4api.RouteID
		},
	})
	if err != nil {
		return nil, err
	}
	return &forwardHost{Router: router, api: api}, nil
}

// forwardHost is the router plus the v4 API, which the package owns
// outright.
type forwardHost struct {
	*pluginhostsdk.Router
	api *v4api.Service
}

// Dispatch answers the v4 API itself and hands every other route to the
// router.
func (h *forwardHost) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (response pluginhostsdk.DispatchResponse, err error) {
	if request.RouteID != v4api.RouteID {
		return h.Router.Dispatch(ctx, request)
	}
	if h.Draining() {
		return pluginhostsdk.DispatchResponse{}, errors.New("package is unavailable")
	}
	// super_admin is the kernel's answer to "may this caller delete" on the
	// list routes (F5b D7); it is absent elsewhere.
	var principal struct {
		pluginhostsdk.Principal
		SuperAdmin bool `json:"super_admin"`
	}
	if len(request.PrincipalJSON) > 0 {
		if err := json.Unmarshal(request.PrincipalJSON, &principal); err != nil {
			return pluginhostsdk.DispatchResponse{}, errors.New("package request principal is invalid")
		}
	}
	if !principal.Admin {
		// The kernel admits only administrators to /api/v4; a request that
		// says otherwise is refused here too.
		return pluginhostsdk.DispatchResponse{StatusCode: 403, ResponseBody: []byte(`{"error":{"code":"forbidden","message":"administrators only"}}`), Headers: jsonHeaders}, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("v4 forward API panicked: %v", recovered)
		}
	}()
	answer := h.api.Serve(ctx, v4api.Request{
		Method: strings.ToUpper(request.Method), Path: request.Metadata.Path, Query: request.Metadata.Query, Body: request.RequestBody,
		IdempotencyKey: request.IdempotencyKey, ActorID: principal.ActorID, SuperAdmin: principal.SuperAdmin,
	})
	return pluginhostsdk.DispatchResponse{StatusCode: uint32(answer.StatusCode), ResponseBody: answer.Body, Headers: jsonHeaders}, nil // #nosec G115 -- an HTTP status.
}

var jsonHeaders = []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}}

// bridgedRoutes are the package's compatibility routes. None has a native
// handler: each relays to the kernel's legacy handler. The flux forwarding
// routes that changed what nodes run (forwards, legacy rules, tunnel and
// permission updates, node and Ansible machine management, the clean agent
// tokens) were removed in v4.2 (F5d), and /api/v4/forward/* replaces them.
// The routes below go with the legacy runtime (F5c).
var bridgedRoutes = map[string]struct{}{
	// Legacy flux reads and writes on the flux tables, which the package
	// no longer adopts (F5d), so the kernel serves them until F5c drops the
	// tables: the forward lists and display order, tunnels (list, create,
	// delete, the tunnels a forward may use), tunnel permissions (assign,
	// list), the multi-ingress comparison, the node statistics, the user's
	// legacy rules and the speed limits (create, list, delete, tunnels).
	"forward.forward.list.post":                             {},
	"forward.admin.forward.list.post":                       {},
	"forward.forward.update_order.post":                     {},
	"forward.admin.forward.update_order.post":               {},
	"forward.admin.tunnel.list.post":                        {},
	"forward.admin.tunnel.create.post":                      {},
	"forward.admin.tunnel.delete.post":                      {},
	"forward.tunnel.user.tunnel.post":                       {},
	"forward.admin.tunnel.user.tunnel.post":                 {},
	"forward.tunnel.user.assign.post":                       {},
	"forward.admin.tunnel.user.assign.post":                 {},
	"forward.tunnel.user.list.post":                         {},
	"forward.admin.tunnel.user.list.post":                   {},
	"forward.admin.forward.observability.multi_ingress.get": {},
	"forward.admin.forward.stats.get":                       {},
	"forward.user.forward.rules.get":                        {},
	"forward.speed_limit.create.post":                       {},
	"forward.speed_limit.list.post":                         {},
	"forward.speed_limit.delete.post":                       {},
	"forward.speed_limit.tunnels.post":                      {},
	// The administrator's traffic reset (POST /api/v2/user/reset, the Users
	// page's 重置流量): type 1 resets a subscriber's traffic, type 2 a tunnel
	// permission's. The kernel serves it.
	"forward.user.reset.post": {},
	// Kernel-owned (D4): runtime status and diagnosis describe the kernel's
	// own executors. They read the protected v2_system_config (NodeX address
	// and token, Ansible settings), call NodeX and inspect files on
	// Control's disk; they stay until the runtime moves to agents (A5).
	"forward.admin.forward.runtime.status.get": {},
	"forward.admin.forward.runtime.doctor.get": {},
	"forward.admin.forward.local.status.get":   {},
	"forward.admin.forward.local.doctor.get":   {},
	// Kernel-owned (D3): the agent channel. Registration, heartbeat and
	// report authenticate a clean agent, claim runtime jobs and record
	// their results and traffic; the agents' rule list authenticates a
	// forward node by its API token. A2 replaces them (enrollment, rules
	// pushed on the stream), and 5.0 removes them (D8).
	"forward.forward_agent.register.post":  {},
	"forward.forward_agent.heartbeat.post": {},
	"forward.forward_agent.report.post":    {},
	"forward.forward.agent.rules.get":      {},
	// Kernel-owned (D4): flow accounting. The forward's counters, the
	// subscriber's traffic (subscriber.RecordTrafficTx) and the
	// permission's traffic change in one kernel transaction under a
	// per-forward lock in Control's memory, and a subscriber or permission
	// that runs out pauses its forwards on their nodes. The callers send no
	// batch id, so the transaction cannot be split into idempotent steps.
	"forward.internal.forward.traffic.upload.post":   {},
	"forward.internal.forward.traffic.report.post":   {},
	"forward.internal.forward.traffic.snapshot.post": {},
}
