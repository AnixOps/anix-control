package packagebridge

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func panicTestCall(routeID, path string) Call {
	return Call{
		Host: HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7},
		Request: Request{
			RequestID: "request-panic-1", RouteID: routeID, Method: "GET",
			PrincipalJSON: []byte(`{"actor_id":9,"admin":false,"package_id":"identity-platform"}`),
			MetadataJSON:  []byte(`{"path":"` + path + `"}`),
			Deadline:      time.Now().Add(time.Second),
		},
		Operation: routeID,
	}
}

func TestHTTPAdapterRecoversLegacyHandlerPanicAndDiscardsPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewHTTPAdapter(func(c *gin.Context) {
		c.Header("X-Partial", "1")
		c.String(http.StatusOK, "partial output")
		panic("legacy handler bug")
	})

	response, err := adapter(context.Background(), panicTestCall("identity.user.info", "/api/v2/user/info"))

	require.NoError(t, err)
	require.EqualValues(t, http.StatusInternalServerError, response.StatusCode)
	require.Empty(t, response.Body)
	require.Empty(t, response.Headers)
	require.NoError(t, validateResponse(response))
}

func TestHTTPAdapterRecoversNilPanic(t *testing.T) {
	adapter := NewHTTPAdapter(func(*gin.Context) { panic(nil) })

	response, err := adapter(context.Background(), panicTestCall("identity.user.info", "/api/v2/user/info"))

	require.NoError(t, err)
	require.EqualValues(t, http.StatusInternalServerError, response.StatusCode)
}

func TestSessionSurvivesPanickingOperations(t *testing.T) {
	allowlist, err := NewAllowlistWithFallback(nil,
		Operation{
			PackageID: "identity-platform", RouteID: "identity.user.info", Name: "identity.user.info",
			Handler: NewHTTPAdapter(func(*gin.Context) { panic("legacy handler bug") }),
		},
		Operation{
			PackageID: "identity-platform", RouteID: "identity.raw.panic", Name: "identity.raw.panic",
			Handler: func(context.Context, Call) (Response, error) { panic("operation bug") },
		},
	)
	require.NoError(t, err)
	session, child, err := NewSession(HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7}, allowlist)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	mint := func(routeID string) []byte {
		call := panicTestCall(routeID, "/api/v2/user/info")
		capability, err := session.Mint(call.Request)
		require.NoError(t, err)
		return capability
	}

	// A panic below the operation handler is caught by the session's gRPC
	// recovery interceptor.
	_, err = client.Invoke(context.Background(), mint("identity.raw.panic"), "identity.raw.panic", nil)
	require.ErrorIs(t, err, packagebridgesdk.ErrBridgeUnavailable)
	require.ErrorContains(t, err, codes.Internal.String())

	// A legacy handler panic is answered by the HTTP adapter itself, and the
	// session keeps serving afterwards.
	for range 2 {
		response, err := client.Invoke(context.Background(), mint("identity.user.info"), "identity.user.info", nil)
		require.NoError(t, err)
		require.EqualValues(t, http.StatusInternalServerError, response.StatusCode)
		require.Empty(t, response.Body)
	}
}

func TestWebSocketAdapterRecoversPanicBeforeUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewWebSocketAdapter(func(*gin.Context) { panic("legacy WebSocket bug before upgrade") })
	stream := newWebSocketAdapterStream()
	defer close(stream.incoming)

	done := make(chan error, 1)
	go func() {
		done <- adapter(context.Background(), panicTestCall("telemetry.monitor.ws", "/api/v2/admin/ws/monitor"), stream)
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errLegacyHandlerPanicked)
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket adapter did not finish after the legacy handler panicked")
	}
}

func TestWebSocketAdapterRecoversPanicAfterUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgraded := make(chan struct{})
	adapter := NewWebSocketAdapter(func(c *gin.Context) {
		connection, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		_ = connection.WriteMessage(websocket.TextMessage, []byte("hello"))
		close(upgraded)
		panic("legacy WebSocket bug after upgrade")
	})
	stream := newWebSocketAdapterStream()
	defer close(stream.incoming)

	done := make(chan error, 1)
	go func() {
		done <- adapter(context.Background(), panicTestCall("telemetry.monitor.ws", "/api/v2/admin/ws/monitor"), stream)
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket adapter did not close the relay after the legacy handler panicked")
	}
	<-upgraded
	// The relay has returned, so every frame it sent is already buffered.
	var closeFrame *WebSocketClose
	for closeFrame == nil {
		select {
		case frame := <-stream.outgoing:
			closeFrame = frame.Close
		default:
			t.Fatal("WebSocket adapter did not send a close frame after the legacy handler panicked")
		}
	}
	require.EqualValues(t, websocket.CloseInternalServerErr, closeFrame.Code)
}
