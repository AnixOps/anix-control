package v2

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketGatewayRejectsUnavailablePackageBeforeUpgrade(t *testing.T) {
	upgradeCalls := 0
	gateway := WebSocketGateway{
		Registry: NewRegistry(registrySourceStub{err: ErrPackageUnavailable}),
		Upgrade: func(http.ResponseWriter, *http.Request, http.Header) (*websocket.Conn, error) {
			upgradeCalls++
			return nil, nil
		},
	}
	router := http.NewServeMux()
	router.HandleFunc("/api/v2/admin/ws/monitor", func(writer http.ResponseWriter, request *http.Request) {
		gateway.ServeHTTP(writer, request)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/admin/ws/monitor", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Zero(t, upgradeCalls)
}

func TestWebSocketGatewayUsesTextFramesAndSeparateSessionLimit(t *testing.T) {
	gateway := WebSocketGateway{Timeout: 20 * time.Millisecond, SessionTimeout: 6 * time.Hour}
	writer := &recordingWebSocketWriter{}
	require.Equal(t, websocket.TextMessage, compatibilityWebSocketMessageType())
	require.NoError(t, writeCompatibilityWebSocketData(writer, []byte(`{"type":"snapshot"}`)))
	require.Equal(t, websocket.TextMessage, writer.messageType)
	require.Equal(t, []byte(`{"type":"snapshot"}`), writer.data)
	require.Equal(t, 20*time.Millisecond, gateway.setupTimeout())
	require.Equal(t, 6*time.Hour, gateway.sessionTimeout())
}

type recordingWebSocketWriter struct {
	messageType int
	data        []byte
}

func (w *recordingWebSocketWriter) WriteMessage(messageType int, data []byte) error {
	w.messageType = messageType
	w.data = append([]byte(nil), data...)
	return nil
}
