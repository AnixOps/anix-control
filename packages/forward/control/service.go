package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/forward/v4api"
	"google.golang.org/grpc"
)

// newForwardService returns the forward host's router. The package serves
// no v2 route natively and adopts no table: the one v2 route it declares
// (bridgedRoutes) relays to the kernel's legacy handler, whatever mode is
// stored for it.
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
// routes went with the flux runtime and gost; /api/v4/forward/* replaces
// them.
var bridgedRoutes = map[string]struct{}{
	// The administrator's traffic reset (POST /api/v2/user/reset, the Users
	// page's 重置流量): type 1 resets a subscriber's traffic. The kernel
	// serves it.
	"forward.user.reset.post": {},
}
