package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/forward/native"
	"github.com/AnixOps/anix-control/v4/packages/forward/v4api"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// forwardBridge is what the forward host needs from the package bridge.
type forwardBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newForwardService returns the forward host's router. The routes in
// forwardRoutes have a native handler on the adopted forward tables and the
// forward node, runtime settings, user directory and entitlement views;
// such a route serves natively once the kernel sets its mode, and falls
// back to the legacy handler otherwise. The traffic reset of a subscriber
// goes through the kernel's KernelSubscriber over the bridge connection
// (local socket or module listener); a bridge without one leaves it legacy.
// The routes in bridgedRoutes always relay to the legacy handler.
//
// The package's own control route, v4api.Route (/api/v4/forward/* to its
// callers), is the v4 administrator API on the kernel's ForwardControl
// over the same bridge connection (v4api). It has no legacy handler and no
// route mode: the host always answers it, 503 without the connection.
func newForwardService(bridge forwardBridge, leaseID string) (*forwardHost, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	api := &v4api.Service{}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Subscriber = kernelsubscriberv1.NewKernelSubscriberClient(conn.Conn())
		api.Forward = forwardv1.NewForwardControlClient(conn.Conn())
	}
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "forward", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := forwardRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute || routeID == v4api.RouteID
		},
		Native: service.Handlers(),
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

// forwardRoutes are the package's compatibility routes with a native
// handler. None of them changes what a node runs or state the kernel keeps
// in memory.
var forwardRoutes = map[string]struct{}{
	// Panel forwards: the lists, and the display order, which no node runs.
	"forward.forward.list.post":               {},
	"forward.admin.forward.list.post":         {},
	"forward.forward.update_order.post":       {},
	"forward.admin.forward.update_order.post": {},
	// Tunnels: the list, creation and deletion of an unused tunnel. A
	// tunnel runs nothing until a forward uses it.
	"forward.admin.tunnel.list.post":   {},
	"forward.admin.tunnel.create.post": {},
	"forward.admin.tunnel.delete.post": {},
	// The tunnels a forward may use, for the caller.
	"forward.tunnel.user.tunnel.post":       {},
	"forward.admin.tunnel.user.tunnel.post": {},
	// Tunnel permissions: granting one, and the list.
	"forward.tunnel.user.assign.post":       {},
	"forward.admin.tunnel.user.assign.post": {},
	"forward.tunnel.user.list.post":         {},
	"forward.admin.tunnel.user.list.post":   {},
	// Reads: a forward's ingress latencies, the node statistics and the
	// caller's legacy rules (their nodes without API tokens).
	"forward.admin.forward.observability.multi_ingress.get": {},
	"forward.admin.forward.stats.get":                       {},
	"forward.user.forward.rules.get":                        {},
	// The administrator's traffic reset (KernelSubscriber.ResetTraffic, or
	// a tunnel permission's traffic).
	"forward.user.reset.post": {},
	// Speed limits (v2_speed_limit), moved from plan: creation, the list,
	// the deletion of an unused limit and the tunnels a limit may name. A
	// limit runs nothing until a permission names it.
	"forward.speed_limit.create.post":  {},
	"forward.speed_limit.list.post":    {},
	"forward.speed_limit.delete.post":  {},
	"forward.speed_limit.tunnels.post": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. The flux
// forwarding routes that changed what nodes run (forwards, legacy rules,
// tunnel and permission updates, node and Ansible machine management, the
// clean agent tokens) were removed in v4.2 (F5d): /api/v4/forward/* replaces
// them.
var bridgedRoutes = map[string]struct{}{
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
