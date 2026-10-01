package bridgecontract

import (
	"context"
	"testing"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type completionRecorder struct {
	kernelorderv1.UnimplementedKernelOrderServer
	host packagebridge.HostIdentity
	seen *[]packagebridge.HostIdentity
}

func (r completionRecorder) CompleteOrderPayment(_ context.Context, request *kernelorderv1.CompleteOrderPaymentRequest) (*kernelorderv1.CompleteOrderPaymentResponse, error) {
	*r.seen = append(*r.seen, r.host)
	return &kernelorderv1.CompleteOrderPaymentResponse{Applied: true, OrderId: request.GetOrderId(),
		Outcome: kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_COMPLETED}, nil
}

// A local host reaches KernelOrder over its bridge connection, as the
// payment host does from its callbacks, and the kernel serves it as the
// session's host identity.
func TestKernelOrderOverTheLocalBridge(t *testing.T) {
	var seen []packagebridge.HostIdentity
	client := dialPlanSession(t, packagebridge.SessionOptions{
		KernelOrder: func(host packagebridge.HostIdentity) kernelorderv1.KernelOrderServer {
			return completionRecorder{host: host, seen: &seen}
		},
	})
	response, err := kernelorderv1.NewKernelOrderClient(client.Conn()).CompleteOrderPayment(context.Background(),
		&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 7})
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.EqualValues(t, 7, response.GetOrderId())
	require.Equal(t, []packagebridge.HostIdentity{{PackageID: "plan", Version: "4.0.0", Generation: 3}}, seen)
}

func TestKernelOrderIsAbsentWithoutAProvider(t *testing.T) {
	client := dialPlanSession(t, packagebridge.SessionOptions{})
	_, err := kernelorderv1.NewKernelOrderClient(client.Conn()).CompleteOrderPayment(context.Background(),
		&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 7})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
