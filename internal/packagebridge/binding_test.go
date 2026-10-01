package packagebridge

import (
	"context"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func bindingSession(t *testing.T, identity HostIdentity, handler OperationHandler) *GenerationSession {
	t.Helper()
	allowlist, err := NewAllowlistWithFallback(nil, Operation{PackageID: identity.PackageID, RouteID: "proxy.admin.nodes.post", Name: "proxy.admin.nodes.post", Handler: handler})
	require.NoError(t, err)
	session, err := NewGenerationSession(identity, allowlist, SessionOptions{})
	require.NoError(t, err)
	return session
}

func answer(context.Context, Call) (Response, error) {
	return Response{StatusCode: 200, Body: []byte(`{}`)}, nil
}

// Peek finds a live capability without consuming it; a consumed, revoked,
// expired or fenced one is gone.
func TestPeekVerifiesALiveCapabilityWithoutConsumingIt(t *testing.T) {
	identity := HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 5}
	session := bindingSession(t, identity, answer)
	request := Request{RequestID: "req-1", RouteID: "proxy.admin.nodes.post", Method: "POST", Body: []byte(`{"raw":"x"}`), Deadline: time.Now().Add(time.Minute), SealedRequest: "sealed-key"}
	capability, err := session.Mint(request)
	require.NoError(t, err)

	peeked, ok := session.Peek(capability)
	require.True(t, ok)
	require.Equal(t, request, peeked)
	_, ok = session.Peek(capability)
	require.True(t, ok, "a peek does not consume")
	_, err = session.take(capability, "proxy.admin.nodes.post")
	require.NoError(t, err, "the legacy relay still consumes it")
	_, ok = session.Peek(capability)
	require.False(t, ok, "a consumed capability binds nothing")

	revoked, err := session.Mint(request)
	require.NoError(t, err)
	session.Revoke(revoked)
	_, ok = session.Peek(revoked)
	require.False(t, ok)

	expiring := request
	expiring.Deadline = time.Now().Add(20 * time.Millisecond)
	expired, err := session.Mint(expiring)
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)
	_, ok = session.Peek(expired)
	require.False(t, ok)

	_, ok = session.Peek([]byte("short"))
	require.False(t, ok)
	fenced, err := session.Mint(request)
	require.NoError(t, err)
	require.NoError(t, session.Close())
	_, ok = session.Peek(fenced)
	require.False(t, ok)
}

// A binding resolves only on the generation the call arrived on.
func TestBoundRequestIsFoundOnlyOnTheCallsGeneration(t *testing.T) {
	identity := HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 5}
	session := bindingSession(t, identity, answer)
	other := bindingSession(t, HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 6}, answer)
	capability, err := session.Mint(Request{RequestID: "req-1", RouteID: "proxy.admin.nodes.post", Method: "POST", Deadline: time.Now().Add(time.Minute)})
	require.NoError(t, err)

	_, _, err = BoundRequest(context.Background(), capability)
	require.ErrorIs(t, err, ErrBindingRejected, "a call without a generation binds nothing")
	_, _, err = BoundRequest(withGeneration(context.Background(), other), capability)
	require.ErrorIs(t, err, ErrBindingRejected, "another generation's capability")
	host, request, err := BoundRequest(withGeneration(context.Background(), session), capability)
	require.NoError(t, err)
	require.Equal(t, identity, host)
	require.Equal(t, "req-1", request.RequestID)
}

// bindingProbe answers whether a call's context resolves the capability.
type bindingProbe struct {
	kernelnodeopsv1.UnimplementedKernelNodeOpsServer
	capability []byte
}

func (p bindingProbe) SubmitOperation(ctx context.Context, request *kernelnodeopsv1.SubmitOperationRequest) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	_, bound, err := BoundRequest(ctx, request.GetRequest().GetBridgeCapability())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "unbound")
	}
	return &kernelnodeopsv1.SubmitOperationResponse{Operation: &kernelnodeopsv1.Operation{RequestId: bound.RequestID}}, nil
}

func (p bindingProbe) WatchOperations(_ *kernelnodeopsv1.WatchOperationsRequest, stream grpc.ServerStreamingServer[kernelnodeopsv1.OperationEvent]) error {
	_, bound, err := BoundRequest(stream.Context(), p.capability)
	if err != nil {
		return status.Error(codes.PermissionDenied, "unbound")
	}
	return stream.Send(&kernelnodeopsv1.OperationEvent{Operation: &kernelnodeopsv1.Operation{RequestId: bound.RequestID}})
}

// The local session's server puts its generation on every call, unary and
// streaming, so a contract finds the bindings minted for its host.
func TestLocalSessionCallsCarryTheirGeneration(t *testing.T) {
	identity := HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 5}
	allowlist, err := NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	probe := &bindingProbe{}
	session, child, err := NewSessionWithOptions(identity, allowlist, SessionOptions{
		KernelNodeOps: func(HostIdentity) kernelnodeopsv1.KernelNodeOpsServer { return probe },
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	capability, err := session.Mint(Request{RequestID: "req-9", RouteID: "proxy.admin.nodes.post", Method: "POST", Deadline: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	probe.capability = capability
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	nodeOps := kernelnodeopsv1.NewKernelNodeOpsClient(client.Conn())

	response, err := nodeOps.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: capability}})
	require.NoError(t, err)
	require.Equal(t, "req-9", response.GetOperation().GetRequestId())
	_, err = nodeOps.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: make([]byte, 32)}})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	stream, err := nodeOps.WatchOperations(context.Background(), &kernelnodeopsv1.WatchOperationsRequest{})
	require.NoError(t, err)
	event, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "req-9", event.GetOperation().GetRequestId())
}

// ServeLegacy runs a route's allowlisted legacy operation in the kernel,
// without a capability, on the request as sent; any other operation, a past
// deadline or a fenced generation is refused.
func TestServeLegacyRunsOnlyTheRoutesLegacyOperation(t *testing.T) {
	identity := HostIdentity{PackageID: "proxy-node", Version: "4.1.0", Generation: 5}
	calls := make(chan Call, 1)
	session := bindingSession(t, identity, func(_ context.Context, call Call) (Response, error) {
		calls <- call
		return Response{StatusCode: 200, Body: []byte(`{"legacy":true}`)}, nil
	})
	request := Request{RequestID: "req-1", RouteID: "proxy.admin.nodes.post", Method: "POST", Body: []byte(`{"raw_config":"x"}`), Deadline: time.Now().Add(time.Minute)}
	response, err := session.ServeLegacy(context.Background(), request, "proxy.admin.nodes.post")
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, string(response.Body))
	call := <-calls
	require.Equal(t, identity, call.Host)
	require.Equal(t, `{"raw_config":"x"}`, string(call.Request.Body))

	_, err = session.ServeLegacy(context.Background(), request, "proxy.admin.nodes.delete")
	require.ErrorIs(t, err, ErrCapabilityRejected)
	other := request
	other.RouteID = "proxy.admin.nodes.id.put"
	_, err = session.ServeLegacy(context.Background(), other, "proxy.admin.nodes.id.put")
	require.ErrorIs(t, err, ErrCapabilityRejected)
	past := request
	past.Deadline = time.Now().Add(-time.Second)
	_, err = session.ServeLegacy(context.Background(), past, "proxy.admin.nodes.post")
	require.ErrorIs(t, err, ErrCapabilityRejected)
	require.NoError(t, session.Close())
	_, err = session.ServeLegacy(context.Background(), request, "proxy.admin.nodes.post")
	require.ErrorIs(t, err, ErrBridgeClosed)
}
