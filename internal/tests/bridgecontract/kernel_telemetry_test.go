package bridgecontract

import (
	"context"
	"testing"

	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type dashboardRecorder struct {
	kerneltelemetryv1.UnimplementedKernelTelemetryServer
	host packagebridge.HostIdentity
	seen *[]packagebridge.HostIdentity
}

func (r dashboardRecorder) GetDashboard(_ context.Context, request *kerneltelemetryv1.GetDashboardRequest) (*kerneltelemetryv1.GetDashboardResponse, error) {
	*r.seen = append(*r.seen, r.host)
	cachedAt := "2026-10-01T08:00:00+08:00"
	if request.GetRefresh() {
		cachedAt = "2026-10-01T08:00:30.5+08:00"
	}
	return &kerneltelemetryv1.GetDashboardResponse{TotalUsers: 4, OnlineUsers: 2, CachedAt: cachedAt}, nil
}

// A local host reaches KernelTelemetry over its bridge connection, as the
// machine-telemetry host does for the dashboard, and the kernel serves it
// as the session's host identity.
func TestKernelTelemetryOverTheLocalBridge(t *testing.T) {
	var seen []packagebridge.HostIdentity
	client := dialPlanSession(t, packagebridge.SessionOptions{
		KernelTelemetry: func(host packagebridge.HostIdentity) kerneltelemetryv1.KernelTelemetryServer {
			return dashboardRecorder{host: host, seen: &seen}
		},
	})
	telemetry := kerneltelemetryv1.NewKernelTelemetryClient(client.Conn())
	response, err := telemetry.GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 4, response.GetTotalUsers())
	require.EqualValues(t, 2, response.GetOnlineUsers())
	require.Equal(t, "2026-10-01T08:00:00+08:00", response.GetCachedAt())
	refreshed, err := telemetry.GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{Refresh: true})
	require.NoError(t, err)
	require.Equal(t, "2026-10-01T08:00:30.5+08:00", refreshed.GetCachedAt(), "refresh reaches the kernel")
	host := packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 3}
	require.Equal(t, []packagebridge.HostIdentity{host, host}, seen)
}

func TestKernelTelemetryIsAbsentWithoutAProvider(t *testing.T) {
	client := dialPlanSession(t, packagebridge.SessionOptions{})
	_, err := kerneltelemetryv1.NewKernelTelemetryClient(client.Conn()).GetDashboard(context.Background(), &kerneltelemetryv1.GetDashboardRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
