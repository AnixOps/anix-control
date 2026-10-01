package packagebridge

import (
	"context"
	"testing"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unboundStream is a server stream from a caller with no transport peer.
type unboundStream struct{ grpc.ServerStream }

func (unboundStream) Context() context.Context { return context.Background() }
func (unboundStream) RecvMsg(any) error        { return nil }

// The module listener serves each contract through a wrapper that resolves
// the bound instance and hands the call to the kernel's server for its
// generation. A method the wrapper does not forward falls through to the
// generated Unimplemented stub, and a network module gets Unimplemented
// where a local host is served. So every method of every contract, unary
// and streaming, must reach the instance check: a caller without a
// transport peer is Unauthenticated, never Unimplemented.
func TestModuleContractServersForwardEveryMethod(t *testing.T) {
	bridge, err := NewModuleBridge(nil, ModuleBridgeOptions{Cluster: "prod"})
	require.NoError(t, err)
	contracts := []struct {
		desc   grpc.ServiceDesc
		server any
	}{
		{kernelidentityv1.KernelIdentity_ServiceDesc, bridge.KernelIdentityServer(nil)},
		{kernelorderv1.KernelOrder_ServiceDesc, bridge.KernelOrderServer(nil)},
		{kernelsettingsv1.KernelSettings_ServiceDesc, bridge.KernelSettingsServer(nil)},
		{kernelsubscriberv1.KernelSubscriber_ServiceDesc, bridge.KernelSubscriberServer(nil)},
		{kerneltelemetryv1.KernelTelemetry_ServiceDesc, bridge.KernelTelemetryServer(nil)},
		{kernelnodeopsv1.KernelNodeOps_ServiceDesc, bridge.KernelNodeOpsServer(nil)},
	}
	for _, contract := range contracts {
		require.NotEmpty(t, contract.desc.Methods, contract.desc.ServiceName)
		for _, method := range contract.desc.Methods {
			_, err := method.Handler(contract.server, context.Background(), func(any) error { return nil }, nil)
			require.Equal(t, codes.Unauthenticated, status.Code(err), "%s/%s is not forwarded: %v", contract.desc.ServiceName, method.MethodName, err)
		}
		for _, stream := range contract.desc.Streams {
			err := stream.Handler(contract.server, unboundStream{})
			require.Equal(t, codes.Unauthenticated, status.Code(err), "%s/%s is not forwarded: %v", contract.desc.ServiceName, stream.StreamName, err)
		}
	}
}
