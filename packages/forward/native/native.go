// Package native implements part of the forward package's v2 routes in the
// package itself, on the kernel's forward tables adopted in place
// (kernel.storage.adopt): v2_forward, v2_forward_tunnel,
// v2_forward_user_tunnel, v2_speed_limit, v2_forward_rule and
// v2_forward_latency_bucket. Legacy handlers and native routes share the
// tables, so a route can switch between them at any time. Responses are
// byte-compatible with the legacy handlers (internal/tests/forwardcompat).
//
// A route is native only when it neither changes what a node runs nor
// touches state the kernel keeps in memory: its writes leave the rows the
// kernel's handlers would, and the kernel's forward runtime re-reads them.
// Every route that pushes a forward or rule to a node (NodeX, the local
// Ansible executor or a clean agent), probes the network, authenticates an
// agent or accounts traffic stays bridged; the host's route map says why
// for each one.
//
// Other domains, and the forward data that holds credentials, are read
// through kernel views only:
//   - kapi_forward_node_v1: the forward nodes without their API tokens.
//     v2_forward_node, v2_forward_clean_agent and v2_forward_runtime_job
//     hold agent credentials and are protected kernel tables;
//   - kapi_forward_runtime_settings_v1: the three system configuration keys
//     that choose the forward runtime backend, no other row of the
//     protected v2_system_config;
//   - kapi_user_directory_v1, whether a user exists, and
//     kapi_subscriber_entitlement_v1, a subscriber's speed limit.
//
// The administrator's traffic reset of a subscriber (POST /api/v2/user/reset,
// type 1) goes through the kernel's KernelSubscriber.ResetTraffic
// (kernel.subscriber.traffic.v1); the package never writes v2_user.
package native

import (
	"context"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Subscriber is the part of KernelSubscriber the native routes call.
type Subscriber interface {
	ResetTraffic(ctx context.Context, in *kernelsubscriberv1.ResetTrafficRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.ResetTrafficResponse, error)
}

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// tables and the granted views are visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Subscriber is the kernel's KernelSubscriber; without it the traffic
	// reset has no native handler and stays legacy.
	Subscriber Subscriber
	// NewToken identifies a reset request that has neither an
	// Idempotency-Key nor a request id; it defaults to a random UUID.
	NewToken func() string
	// Now defaults to time.Now.
	Now func() time.Time
}

// ResetRouteID is the administrator's traffic reset, the one route that
// needs KernelSubscriber.
const ResetRouteID = "forward.user.reset.post"

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"forward.forward.list.post":                             s.ListForwards,
		"forward.admin.forward.list.post":                       s.ListForwards,
		"forward.forward.update_order.post":                     s.UpdateForwardOrder,
		"forward.admin.forward.update_order.post":               s.UpdateForwardOrder,
		"forward.admin.tunnel.list.post":                        s.ListAdminTunnels,
		"forward.admin.tunnel.create.post":                      s.CreateTunnel,
		"forward.admin.tunnel.delete.post":                      s.DeleteTunnel,
		"forward.tunnel.user.tunnel.post":                       s.ListTunnels,
		"forward.admin.tunnel.user.tunnel.post":                 s.ListTunnels,
		"forward.tunnel.user.assign.post":                       s.AssignUserTunnel,
		"forward.admin.tunnel.user.assign.post":                 s.AssignUserTunnel,
		"forward.tunnel.user.list.post":                         s.ListUserTunnels,
		"forward.admin.tunnel.user.list.post":                   s.ListUserTunnels,
		"forward.admin.forward.observability.multi_ingress.get": s.MultiIngressLatency,
		"forward.admin.forward.stats.get":                       s.Stats,
		"forward.user.forward.rules.get":                        s.UserRules,
		SpeedLimitCreateRouteID:                                 s.CreateSpeedLimit,
		SpeedLimitListRouteID:                                   s.ListSpeedLimits,
		SpeedLimitDeleteRouteID:                                 s.DeleteSpeedLimit,
		SpeedLimitTunnelsRouteID:                                s.SpeedLimitTunnels,
	}
	if s.Subscriber != nil {
		handlers[ResetRouteID] = s.ResetFlow
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// panel is the legacy handlers' panelSuccess.
func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

// panelError is the legacy handlers' panelError.
func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// query is gin's c.Query: the first value of a query key, else "".
func query(request pluginhostsdk.NativeRequest, key string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// actor is the caller and whether the kernel authenticated an
// administrator, the legacy handlers' c.GetUint("user_id") and
// c.GetBool("is_admin").
func actor(request pluginhostsdk.NativeRequest) (uint, bool) {
	return request.Principal.ActorID, request.Principal.Admin
}
