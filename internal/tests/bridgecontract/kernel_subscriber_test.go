package bridgecontract

import (
	"context"
	"testing"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grantRecorder struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	host packagebridge.HostIdentity
	seen *[]packagebridge.HostIdentity
}

func (r grantRecorder) ApplyEntitlement(_ context.Context, request *kernelsubscriberv1.ApplyEntitlementRequest) (*kernelsubscriberv1.ApplyEntitlementResponse, error) {
	*r.seen = append(*r.seen, r.host)
	return &kernelsubscriberv1.ApplyEntitlementResponse{Applied: request.GetRequestId() == "plan.assign:1"}, nil
}

func dialPlanSession(t *testing.T, options packagebridge.SessionOptions) *packagebridgesdk.Client {
	t.Helper()
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	session, child, err := packagebridge.NewSessionWithOptions(
		packagebridge.HostIdentity{PackageID: "plan", Version: "4.0.0", Generation: 3}, allowlist, options,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

// A local host reaches KernelSubscriber over its bridge connection, as the
// plan host does for assignments, and the kernel serves it as the session's
// host identity.
func TestKernelSubscriberOverTheLocalBridge(t *testing.T) {
	var seen []packagebridge.HostIdentity
	client := dialPlanSession(t, packagebridge.SessionOptions{
		KernelSubscriber: func(host packagebridge.HostIdentity) kernelsubscriberv1.KernelSubscriberServer {
			return grantRecorder{host: host, seen: &seen}
		},
	})
	response, err := kernelsubscriberv1.NewKernelSubscriberClient(client.Conn()).ApplyEntitlement(context.Background(),
		&kernelsubscriberv1.ApplyEntitlementRequest{RequestId: "plan.assign:1", UserId: 2, Plan: &kernelsubscriberv1.PlanSnapshot{PlanId: 1}})
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.Equal(t, []packagebridge.HostIdentity{{PackageID: "plan", Version: "4.0.0", Generation: 3}}, seen)
}

func TestKernelSubscriberIsAbsentWithoutAProvider(t *testing.T) {
	client := dialPlanSession(t, packagebridge.SessionOptions{})
	_, err := kernelsubscriberv1.NewKernelSubscriberClient(client.Conn()).ApplyEntitlement(context.Background(),
		&kernelsubscriberv1.ApplyEntitlementRequest{RequestId: "plan.assign:1", UserId: 2})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
