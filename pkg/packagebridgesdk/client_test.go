package packagebridgesdk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/stretchr/testify/require"
)

func TestClientOpensAWebSocketBridgeWithItsMintedCapability(t *testing.T) {
	allowlist, err := packagebridge.NewAllowlist()
	require.NoError(t, err)
	session, child, err := packagebridge.NewSession(
		packagebridge.HostIdentity{PackageID: "machine-telemetry", Version: "4.0.0", Generation: 7},
		allowlist,
		webSocketResolver{handler: func(_ context.Context, _ packagebridge.Call, stream packagebridge.WebSocketStream) error {
			frame, err := stream.Recv()
			if err != nil {
				return err
			}
			if string(frame.Data) != "ping" {
				return errors.New("unexpected frame")
			}
			return stream.Send(packagebridge.WebSocketFrame{Data: []byte("pong")})
		}},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	capability, err := session.Mint(packagebridge.Request{
		RequestID: "sdk-websocket-1", RouteID: "telemetry.monitor.ws", Method: "GET", Deadline: time.Now().Add(time.Second),
	})
	require.NoError(t, err)
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	stream, err := client.OpenWebSocket(context.Background(), capability, "telemetry.monitor.ws")
	require.NoError(t, err)
	require.NoError(t, stream.Send(packagebridgesdk.WebSocketFrame{Data: []byte("ping")}))
	frame, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), frame.Data)
}

type webSocketResolver struct {
	handler packagebridge.WebSocketOperationHandler
}

func (s webSocketResolver) ResolveWebSocket(packageID, routeID, operation string) (packagebridge.WebSocketOperationHandler, bool) {
	if packageID != "machine-telemetry" || routeID != "telemetry.monitor.ws" || operation != routeID {
		return nil, false
	}
	return s.handler, s.handler != nil
}
