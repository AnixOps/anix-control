package native

import (
	"context"
	"time"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// DashboardRouteID is GET /api/v2/admin/dashboard.
const DashboardRouteID = "telemetry.admin.dashboard.get"

// Telemetry is the kernel's KernelTelemetry as the dashboard reads it
// (kernel.telemetry.dashboard.v1).
type Telemetry interface {
	GetDashboard(ctx context.Context, in *kerneltelemetryv1.GetDashboardRequest, opts ...grpc.CallOption) (*kerneltelemetryv1.GetDashboardResponse, error)
}

// Dashboard is the dashboard snapshot, with the fields and tags of the
// kernel's service.DashboardStats.
type Dashboard struct {
	TotalUsers       int64     `json:"total_users"`
	ActiveUsers      int64     `json:"active_users"`
	ExpiredUsers     int64     `json:"expired_users"`
	BannedUsers      int64     `json:"banned_users"`
	TodayNewUsers    int64     `json:"today_new_users"`
	TotalOrders      int64     `json:"total_orders"`
	PendingOrders    int64     `json:"pending_orders"`
	PaidOrders       int64     `json:"paid_orders"`
	TotalRevenue     int64     `json:"total_revenue"`
	MonthlyIncome    int64     `json:"monthly_income"`
	TodayIncome      int64     `json:"today_income"`
	TotalNodes       int64     `json:"total_nodes"`
	ActiveNodes      int64     `json:"active_nodes"`
	OnlineUsers      int64     `json:"online_users"`
	TotalTrafficUsed int64     `json:"total_traffic_used"`
	TodayTraffic     int64     `json:"today_traffic"`
	CachedAt         time.Time `json:"cached_at"`
}

// Dashboard is GET /api/v2/admin/dashboard: the snapshot the kernel caches
// for 60 seconds, read through KernelTelemetry.GetDashboard. The kernel
// keeps the one cache both modes read, so a native answer carries the
// snapshot and cached_at the legacy handler would, and refresh=true (only
// that exact value, as gin's c.Query) rebuilds it. The online users arrive
// as a count; the set stays in the kernel.
func (s *Service) Dashboard(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	snapshot, err := s.Telemetry.GetDashboard(ctx, &kerneltelemetryv1.GetDashboardRequest{Refresh: query(request, "refresh") == "true"})
	if err != nil {
		// The kernel answers its own error, which the legacy handler shows.
		return pluginhostsdk.PanelJSON(v2compat.PanelError("获取统计失败: "+status.Convert(err).Message(), s.now()))
	}
	cachedAt, err := time.Parse(time.RFC3339Nano, snapshot.GetCachedAt())
	if err != nil {
		return pluginhostsdk.PanelJSON(v2compat.PanelError("获取统计失败: "+err.Error(), s.now()))
	}
	return s.panel(Dashboard{
		TotalUsers: snapshot.GetTotalUsers(), ActiveUsers: snapshot.GetActiveUsers(), ExpiredUsers: snapshot.GetExpiredUsers(),
		BannedUsers: snapshot.GetBannedUsers(), TodayNewUsers: snapshot.GetTodayNewUsers(),
		TotalOrders: snapshot.GetTotalOrders(), PendingOrders: snapshot.GetPendingOrders(), PaidOrders: snapshot.GetPaidOrders(),
		TotalRevenue: snapshot.GetTotalRevenue(), MonthlyIncome: snapshot.GetMonthlyIncome(), TodayIncome: snapshot.GetTodayIncome(),
		TotalNodes: snapshot.GetTotalNodes(), ActiveNodes: snapshot.GetActiveNodes(), OnlineUsers: snapshot.GetOnlineUsers(),
		TotalTrafficUsed: snapshot.GetTotalTrafficUsed(), TodayTraffic: snapshot.GetTodayTraffic(),
		CachedAt: cachedAt,
	})
}
