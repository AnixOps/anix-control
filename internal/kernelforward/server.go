package kernelforward

import (
	"context"
	"errors"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
)

// Authorizer admits a calling host to a capability.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server serves ForwardControl to official packages: every call is
// authorized for kernel.forward.v1 against the calling host's current
// generation, then answered by Service.
type Server struct {
	Service    *Service
	Authorizer Authorizer
}

// For returns the ForwardControl server that host reaches; it has the shape
// of packagebridge.KernelForwardProvider.
func (s *Server) For(host packagebridge.HostIdentity) forwardv1.ForwardControlServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	forwardv1.UnimplementedForwardControlServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) begin(ctx context.Context) (*Service, error) {
	if h.server == nil || h.server.Service == nil || h.server.Service.DB == nil || h.server.Authorizer == nil {
		return nil, status.Error(codes.Unavailable, "kernel forwarding is not configured")
	}
	err := h.server.Authorizer.AuthorizeCapability(ctx, h.host, service.CapabilityForward)
	switch {
	case err == nil:
		return h.server.Service, nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrCapabilityNotAuthorized):
		return nil, status.Errorf(codes.PermissionDenied, "package is not authorized for %s", service.CapabilityForward)
	default:
		return nil, status.Error(codes.Unavailable, "kernel forwarding authorization failed")
	}
}

// failure maps the service's errors to the contract's codes. A refusal
// carries answer, with its violations, as the status's details.
func failure(err error, answer func(violations []*forwardv1.Violation) protoadapt.MessageV1) error {
	var refusal *RefusedError
	switch {
	case errors.As(err, &refusal):
		code := codes.InvalidArgument
		if refusal.Precondition {
			code = codes.FailedPrecondition
		}
		st := status.New(code, refusal.Error())
		if answer != nil {
			if detailed, detailErr := st.WithDetails(answer(refusal.ProtoViolations())); detailErr == nil {
				st = detailed
			}
		}
		return st.Err()
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrInvalidReport):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrNodeNotFound):
		return status.Error(codes.NotFound, "forward node not found")
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, "forward route not found")
	case errors.Is(err, ErrRevisionMismatch):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, ErrRequestConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		return status.Error(codes.Internal, "kernel forwarding failed")
	}
}

func (h *hostServer) CreateRoute(ctx context.Context, request *forwardv1.CreateRouteRequest) (*forwardv1.CreateRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	route, err := svc.CreateRoute(ctx, request.GetRequestId(), request.GetRoute())
	if err != nil {
		return nil, failure(err, func(violations []*forwardv1.Violation) protoadapt.MessageV1 {
			return &forwardv1.CreateRouteResponse{Violations: violations}
		})
	}
	return &forwardv1.CreateRouteResponse{Route: route}, nil
}

func (h *hostServer) UpdateRoute(ctx context.Context, request *forwardv1.UpdateRouteRequest) (*forwardv1.UpdateRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	route, err := svc.UpdateRoute(ctx, request.GetRequestId(), request.GetRoute(), request.GetExpectedRevision())
	if err != nil {
		return nil, failure(err, func(violations []*forwardv1.Violation) protoadapt.MessageV1 {
			return &forwardv1.UpdateRouteResponse{Violations: violations}
		})
	}
	return &forwardv1.UpdateRouteResponse{Route: route}, nil
}

func (h *hostServer) DeleteRoute(ctx context.Context, request *forwardv1.DeleteRouteRequest) (*forwardv1.DeleteRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.DeleteRoute(ctx, request.GetRequestId(), request.GetRouteId()); err != nil {
		// A delete refused by the plan of the other routes names them.
		return nil, failure(err, func(violations []*forwardv1.Violation) protoadapt.MessageV1 {
			return &forwardv1.PlanRouteResponse{Violations: violations}
		})
	}
	return &forwardv1.DeleteRouteResponse{}, nil
}

func (h *hostServer) GetRoute(ctx context.Context, request *forwardv1.GetRouteRequest) (*forwardv1.GetRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	route, err := svc.GetRoute(ctx, request.GetRouteId())
	if err != nil {
		return nil, failure(err, nil)
	}
	enforced, err := svc.Enforcement(ctx, []string{route.GetId()})
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.GetRouteResponse{Route: route, Enforced: enforced[route.GetId()]}, nil
}

func (h *hostServer) ListRoutes(ctx context.Context, request *forwardv1.ListRoutesRequest) (*forwardv1.ListRoutesResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	routes, next, err := svc.ListRoutes(ctx, ListOptions{
		Owner: request.GetOwner(), NodeRef: request.GetNodeRef(), PageSize: request.GetPageSize(), PageToken: request.GetPageToken(),
	})
	if err != nil {
		return nil, failure(err, nil)
	}
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		ids = append(ids, route.GetId())
	}
	enforced, err := svc.Enforcement(ctx, ids)
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.ListRoutesResponse{Routes: routes, NextPageToken: next, Enforced: enforced}, nil
}

func (h *hostServer) PlanRoute(ctx context.Context, request *forwardv1.PlanRouteRequest) (*forwardv1.PlanRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	response, err := svc.PlanRoute(ctx, request)
	if err != nil {
		return nil, failure(err, nil)
	}
	return response, nil
}

func (h *hostServer) GetRouteStats(ctx context.Context, request *forwardv1.GetRouteStatsRequest) (*forwardv1.GetRouteStatsResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	counters, err := svc.RouteStats(ctx, request.GetRouteId())
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.GetRouteStatsResponse{Counters: counters}, nil
}

func (h *hostServer) GetRouteHealth(ctx context.Context, request *forwardv1.GetRouteHealthRequest) (*forwardv1.GetRouteHealthResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	health, err := svc.RouteHealth(ctx, request.GetRouteId())
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.GetRouteHealthResponse{Health: health}, nil
}

// DiagnoseRoute diagnoses a stored route (F3c, diagnose.go): Control's
// records, the forward checks of agent.diagnostic on the nodes that run
// them, and dials from Control for the rest.
func (h *hostServer) DiagnoseRoute(ctx context.Context, request *forwardv1.DiagnoseRouteRequest) (*forwardv1.DiagnoseRouteResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	answer, err := svc.DiagnoseRoute(ctx, request.GetRouteId(), time.Duration(request.GetTimeoutMs())*time.Millisecond)
	if errors.Is(err, ErrDiagnoseBusy) {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	if err != nil {
		return nil, failure(err, nil)
	}
	return answer, nil
}

func (h *hostServer) ListNodes(ctx context.Context, request *forwardv1.ListNodesRequest) (*forwardv1.ListNodesResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := svc.ListNodes(ctx, NodeFilter{Kind: request.GetKind(), Transport: request.GetTransport()})
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.ListNodesResponse{Nodes: nodes}, nil
}

func (h *hostServer) GetNode(ctx context.Context, request *forwardv1.GetNodeRequest) (*forwardv1.GetNodeResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	response, err := svc.GetNode(ctx, request.GetNodeRef())
	if err != nil {
		return nil, failure(err, nil)
	}
	return response, nil
}

func (h *hostServer) SetNodeSettings(ctx context.Context, request *forwardv1.SetNodeSettingsRequest) (*forwardv1.SetNodeSettingsResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	response, err := svc.SetNodeSettingsAnswer(ctx, request.GetNodeRef(), SettingsFromProto(request.GetSettings()))
	if err != nil {
		return nil, failure(err, nil)
	}
	return response, nil
}

func (h *hostServer) CreateForwardNode(ctx context.Context, request *forwardv1.CreateForwardNodeRequest) (*forwardv1.CreateForwardNodeResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	node, err := svc.CreateForwardNode(ctx, request.GetRequestId(), request.GetNode(), request.GetSettings())
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.CreateForwardNodeResponse{Node: node}, nil
}

func (h *hostServer) UpdateForwardNode(ctx context.Context, request *forwardv1.UpdateForwardNodeRequest) (*forwardv1.UpdateForwardNodeResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	response, err := svc.UpdateForwardNode(ctx, request.GetRequestId(), request.GetNode())
	if err != nil {
		return nil, failure(err, func(violations []*forwardv1.Violation) protoadapt.MessageV1 {
			return &forwardv1.UpdateForwardNodeResponse{Violations: violations}
		})
	}
	return response, nil
}

func (h *hostServer) DeleteForwardNode(ctx context.Context, request *forwardv1.DeleteForwardNodeRequest) (*forwardv1.DeleteForwardNodeResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.DeleteForwardNode(ctx, request.GetRequestId(), request.GetId()); err != nil {
		return nil, failure(err, func(violations []*forwardv1.Violation) protoadapt.MessageV1 {
			return &forwardv1.DeleteForwardNodeResponse{Violations: violations}
		})
	}
	return &forwardv1.DeleteForwardNodeResponse{}, nil
}

func (h *hostServer) GetTraffic(ctx context.Context, request *forwardv1.GetTrafficRequest) (*forwardv1.GetTrafficResponse, error) {
	svc, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	var since, until time.Time
	if ms := request.GetSinceUnixMs(); ms > 0 {
		since = time.UnixMilli(ms)
	}
	if ms := request.GetUntilUnixMs(); ms > 0 {
		until = time.UnixMilli(ms)
	}
	buckets, truncated, err := svc.TrafficBuckets(ctx, request.GetRouteId(), request.GetNodeRef(), since, until)
	if err != nil {
		return nil, failure(err, nil)
	}
	return &forwardv1.GetTrafficResponse{Buckets: buckets, Truncated: truncated}, nil
}
