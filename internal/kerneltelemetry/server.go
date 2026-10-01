// Package kerneltelemetry serves the KernelTelemetry contract
// (sdk/api/kerneltelemetry/v1, docs/architecture/kernel-caches.md) to
// official packages: the telemetry the kernel computes and caches in its
// memory, read through the same function and cache as the kernel's legacy
// handlers. Every call is authorized for its capability against the calling
// host's current generation.
package kerneltelemetry

import (
	"context"
	"errors"
	"time"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Authorizer admits a calling host to a capability.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server holds what every host's KernelTelemetry calls share.
type Server struct {
	DB         *gorm.DB
	Authorizer Authorizer
}

// For returns the KernelTelemetry server that host reaches; it has the shape
// of packagebridge.KernelTelemetryProvider.
func (s *Server) For(host packagebridge.HostIdentity) kerneltelemetryv1.KernelTelemetryServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kerneltelemetryv1.UnimplementedKernelTelemetryServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) begin(ctx context.Context, capability string) (*gorm.DB, error) {
	if h.server == nil || h.server.DB == nil || h.server.Authorizer == nil {
		return nil, status.Error(codes.Unavailable, "kernel telemetry is not configured")
	}
	err := h.server.Authorizer.AuthorizeCapability(ctx, h.host, capability)
	switch {
	case err == nil:
		return h.server.DB.WithContext(ctx), nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrCapabilityNotAuthorized):
		return nil, status.Errorf(codes.PermissionDenied, "package is not authorized for %s", capability)
	default:
		return nil, status.Error(codes.Unavailable, "kernel telemetry authorization failed")
	}
}

// GetDashboard answers the dashboard snapshot through
// service.StatsService.GetDashboardStats, the function and cache the
// kernel's legacy handler uses. Only the count of the alive set leaves the
// kernel.
func (h *hostServer) GetDashboard(ctx context.Context, request *kerneltelemetryv1.GetDashboardRequest) (*kerneltelemetryv1.GetDashboardResponse, error) {
	db, err := h.begin(ctx, service.CapabilityTelemetryDashboard)
	if err != nil {
		return nil, err
	}
	stats, err := service.NewStatsServiceOn(db).GetDashboardStats(request.GetRefresh())
	if err != nil {
		// The legacy handler shows this error to the administrator; the
		// module shows it as well.
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &kerneltelemetryv1.GetDashboardResponse{
		TotalUsers: stats.TotalUsers, ActiveUsers: stats.ActiveUsers, ExpiredUsers: stats.ExpiredUsers,
		BannedUsers: stats.BannedUsers, TodayNewUsers: stats.TodayNewUsers,
		TotalOrders: stats.TotalOrders, PendingOrders: stats.PendingOrders, PaidOrders: stats.PaidOrders,
		TotalRevenue: stats.TotalRevenue, MonthlyIncome: stats.MonthlyIncome, TodayIncome: stats.TodayIncome,
		TotalNodes: stats.TotalNodes, ActiveNodes: stats.ActiveNodes, OnlineUsers: stats.OnlineUsers,
		TotalTrafficUsed: stats.TotalTrafficUsed, TodayTraffic: stats.TodayTraffic,
		CachedAt: stats.CachedAt.Format(time.RFC3339Nano),
	}, nil
}
