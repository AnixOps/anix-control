package packagebridge

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketAdapterRelaysFramesToAnExactLegacyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgradeErrors := make(chan error, 1)
	adapter := NewWebSocketAdapter(func(c *gin.Context) {
		connection, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			upgradeErrors <- err
			return
		}
		defer connection.Close()
		_, data, err := connection.ReadMessage()
		if err != nil {
			return
		}
		_ = connection.WriteMessage(websocket.TextMessage, append([]byte("echo:"), data...))
	})
	stream := newWebSocketAdapterStream()
	deadline := time.Now().Add(time.Second)
	done := make(chan error, 1)
	go func() {
		done <- adapter(context.Background(), Call{
			Host: HostIdentity{PackageID: "machine-telemetry", Version: "4.0.0", Generation: 7},
			Request: Request{
				RequestID: "websocket-adapter-1", RouteID: "telemetry.monitor.ws", Method: "GET",
				PrincipalJSON: []byte(`{"actor_id":9,"admin":true,"package_id":"machine-telemetry"}`),
				MetadataJSON:  []byte(`{"path":"/api/v2/admin/ws/monitor"}`),
				Deadline:      deadline,
			},
			Operation: "telemetry.monitor.ws",
		}, stream)
	}()

	stream.incoming <- WebSocketFrame{Data: []byte("ping")}
	select {
	case frame := <-stream.outgoing:
		require.Nil(t, frame.Close)
		require.Equal(t, []byte("echo:ping"), frame.Data)
	case err := <-upgradeErrors:
		require.NoError(t, err)
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("legacy handler did not emit a bridged WebSocket frame")
	}
	stream.incoming <- WebSocketFrame{Close: &WebSocketClose{Code: websocket.CloseNormalClosure, Reason: "done"}}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("WebSocket adapter did not finish after the close frame")
	}
}

type webSocketAdapterStream struct {
	incoming chan WebSocketFrame
	outgoing chan WebSocketFrame
}

func newWebSocketAdapterStream() *webSocketAdapterStream {
	return &webSocketAdapterStream{
		incoming: make(chan WebSocketFrame, 4),
		outgoing: make(chan WebSocketFrame, 4),
	}
}

func (s *webSocketAdapterStream) Recv() (WebSocketFrame, error) {
	frame, ok := <-s.incoming
	if !ok {
		return WebSocketFrame{}, io.EOF
	}
	return frame, nil
}

func (s *webSocketAdapterStream) Send(frame WebSocketFrame) error {
	s.outgoing <- frame
	return nil
}
