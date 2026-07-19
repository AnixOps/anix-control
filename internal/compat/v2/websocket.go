package v2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var defaultWebSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     utils.CheckWebSocketOrigin,
}

type WebSocketUpgrade func(http.ResponseWriter, *http.Request, http.Header) (*websocket.Conn, error)

// WebSocketGateway resolves and opens a package relay before accepting the
// browser upgrade. Legacy socket handlers are intentionally not reachable.
type WebSocketGateway struct {
	Registry       *Registry
	Dispatcher     pluginhost.WebSocketManager
	Timeout        time.Duration
	SessionTimeout time.Duration
	Upgrade        WebSocketUpgrade
}

func (g WebSocketGateway) Serve(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	g.serve(c.Writer, c.Request, requestMetadata(c), requestID(c), actorID(c), actorIsAdmin(c))
}

// ServeHTTP is useful for non-Gin integration points and keeps resolution
// before upgrade testable without a real WebSocket connection.
func (g WebSocketGateway) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		writeWebSocketError(writer, http.StatusBadRequest, "plugin_request_invalid", "request is required")
		return
	}
	g.serve(writer, request, requestMetadataFromRequest(request), requestIDFromRequest(request), 0, false)
}

func (g WebSocketGateway) serve(writer http.ResponseWriter, request *http.Request, metadata pluginhost.RequestMetadata, id string, actor uint, admin bool) {
	if g.Registry == nil {
		writeWebSocketError(writer, http.StatusServiceUnavailable, "package_unavailable", "package route is unavailable")
		return
	}
	route, err := g.Registry.ResolveContext(request.Context(), request.Method, request.URL.Path)
	if err != nil {
		if errors.Is(err, ErrPackageUnavailable) {
			writeWebSocketError(writer, http.StatusServiceUnavailable, "package_unavailable", "package route is unavailable")
		} else {
			writeWebSocketError(writer, http.StatusNotFound, "package_route_not_found", "package route is not declared")
		}
		return
	}
	if route.Transport != TransportWebSocket {
		writeWebSocketError(writer, http.StatusBadRequest, "package_route_requires_http", "package route requires HTTP")
		return
	}
	if g.Dispatcher == nil {
		writeWebSocketError(writer, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		return
	}
	principal, err := requestPrincipalFor(actor, admin, route.PackageID)
	if err != nil {
		writeWebSocketError(writer, http.StatusInternalServerError, "plugin_request_invalid", "plugin request principal could not be encoded")
		return
	}
	setupDeadline := requestDeadline(request.Context(), g.setupTimeout())
	setupContext, cancelSetup := context.WithDeadline(request.Context(), setupDeadline)
	sessionDeadline := time.Now().Add(g.sessionTimeout())
	relay, err := g.Dispatcher.OpenWebSocket(setupContext, pluginhost.WebSocketInput{
		PackageID: route.PackageID, Version: route.Version, Generation: route.Generation, RouteID: route.PackageRoute,
		PrincipalJSON: principal, Metadata: metadata, RequestID: id, IdempotencyKey: request.Header.Get("Idempotency-Key"), Deadline: sessionDeadline,
	})
	cancelSetup()
	if err != nil {
		writeWebSocketError(writer, http.StatusBadGateway, "plugin_host_unavailable", "plugin host is unavailable")
		return
	}
	upgrader := g.Upgrade
	if upgrader == nil {
		upgrader = defaultWebSocketUpgrader.Upgrade
	}
	connection, err := upgrader(writer, request, nil)
	if err != nil {
		_ = relay.Close(pluginhost.WebSocketClose{Code: websocket.CloseInternalServerErr, Reason: "WebSocket upgrade failed"})
		return
	}
	defer func() { _ = connection.Close() }()
	g.relay(connection, relay)
}

func (g WebSocketGateway) setupTimeout() time.Duration {
	if g.Timeout <= 0 {
		return defaultRequestTimeout
	}
	return g.Timeout
}

func (g WebSocketGateway) sessionTimeout() time.Duration {
	if g.SessionTimeout <= 0 {
		return 24 * time.Hour
	}
	return g.SessionTimeout
}

// Existing v2 socket clients exchange JSON strings. The package-host relay
// carries opaque bytes, so its compatibility boundary emits text frames.
func compatibilityWebSocketMessageType() int { return websocket.TextMessage }

type webSocketMessageWriter interface {
	WriteMessage(int, []byte) error
}

func writeCompatibilityWebSocketData(writer webSocketMessageWriter, data []byte) error {
	return writer.WriteMessage(compatibilityWebSocketMessageType(), data)
}

func (g WebSocketGateway) relay(connection *websocket.Conn, relay *pluginhost.WebSocketRelay) {
	done := make(chan struct{})
	var once sync.Once
	finish := func(closeFrame pluginhost.WebSocketClose) {
		once.Do(func() {
			_ = relay.Close(closeFrame)
			_ = connection.Close()
			close(done)
		})
	}
	go func() {
		for {
			messageType, data, err := connection.ReadMessage()
			if err != nil {
				finish(clientClose(err))
				return
			}
			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}
			if err := relay.Send(pluginhost.WebSocketFrame{Data: data}); err != nil {
				finish(pluginhost.WebSocketClose{Code: websocket.CloseInternalServerErr, Reason: "package relay unavailable"})
				return
			}
		}
	}()
	go func() {
		for {
			frame, err := relay.Recv()
			if err != nil {
				finish(pluginhost.WebSocketClose{Code: websocket.CloseInternalServerErr, Reason: "package relay unavailable"})
				return
			}
			if frame.Close != nil {
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(int(frame.Close.Code), frame.Close.Reason), time.Now().Add(time.Second))
				finish(*frame.Close)
				return
			}
			if err := writeCompatibilityWebSocketData(connection, frame.Data); err != nil {
				finish(clientClose(err))
				return
			}
		}
	}()
	<-done
}

func clientClose(err error) pluginhost.WebSocketClose {
	var closeError *websocket.CloseError
	if errors.As(err, &closeError) && closeError.Code >= websocket.CloseNormalClosure && closeError.Code <= 4999 {
		return pluginhost.WebSocketClose{Code: uint32(closeError.Code), Reason: closeError.Text}
	}
	return pluginhost.WebSocketClose{Code: websocket.CloseNormalClosure, Reason: "client disconnected"}
}

func requestMetadataFromRequest(request *http.Request) pluginhost.RequestMetadata {
	metadata := pluginhost.RequestMetadata{Path: request.URL.Path}
	if query := request.URL.Query(); len(query) > 0 {
		metadata.Query = make(map[string][]string, len(query))
		for key, values := range query {
			metadata.Query[key] = append([]string(nil), values...)
		}
	}
	if headers := packageRequestHeaders(request.Header); len(headers) > 0 {
		metadata.Headers = headers
	}
	return metadata
}

func requestIDFromRequest(request *http.Request) string {
	if id := strings.TrimSpace(request.Header.Get("X-Request-ID")); id != "" {
		return id
	}
	return uuid.NewString()
}

func writeWebSocketError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
