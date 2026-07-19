package packagebridge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// NewWebSocketAdapter runs an allowlisted legacy Gin WebSocket handler over a
// private in-memory connection. The package host receives only opaque frames;
// the legacy handler and its database access remain in the kernel process.
func NewWebSocketAdapter(handler gin.HandlerFunc) WebSocketOperationHandler {
	return func(ctx context.Context, call Call, stream WebSocketStream) error {
		if handler == nil || stream == nil {
			return ErrCapabilityRejected
		}
		metadata, err := decodeHTTPMetadata(call.Request.MetadataJSON)
		if err != nil {
			return ErrCapabilityRejected
		}
		principal, err := decodeBridgePrincipal(call.Request.PrincipalJSON, call.Host.PackageID)
		if err != nil {
			return ErrCapabilityRejected
		}
		request, err := bridgeHTTPRequest(ctx, call.Request, metadata)
		if err != nil {
			return ErrCapabilityRejected
		}
		prepareWebSocketUpgradeRequest(request)

		serverConnection, clientConnection := net.Pipe()
		defer serverConnection.Close()
		defer clientConnection.Close()
		responseWriter := newBridgeWebSocketResponseWriter(serverConnection)
		ginContext, _ := gin.CreateTestContext(responseWriter)
		ginContext.Request = request
		ginContext.Set("request_id", call.Request.RequestID)
		ginContext.Set("user_id", principal.ActorID)
		ginContext.Set("is_admin", principal.Admin)
		if metadata.NodeID != 0 {
			ginContext.Set("node_id", metadata.NodeID)
		}
		if metadata.TrustedAgentWebSocketAuth {
			ginContext.Set(agentws.TrustedContextKey, true)
			ginContext.Set(agentws.ForwardNodeContextKey, metadata.TrustedAgentWebSocketForwardNode)
		}
		for key, value := range metadata.PathParams {
			ginContext.Params = append(ginContext.Params, gin.Param{Key: key, Value: value})
		}

		clientResult := make(chan webSocketClientResult, 1)
		go func() {
			connection, _, clientErr := websocket.NewClient(clientConnection, bridgeWebSocketURL(request.URL), nil, 1024, 1024)
			clientResult <- webSocketClientResult{connection: connection, err: clientErr}
		}()
		if err := responseWriter.consumeClientHandshake(); err != nil {
			return fmt.Errorf("read legacy WebSocket bridge handshake: %w", err)
		}
		request.Header.Set("Sec-WebSocket-Key", responseWriter.clientChallengeKey())
		handlerDone := make(chan struct{})
		go func() {
			handler(ginContext)
			close(handlerDone)
		}()

		var client *websocket.Conn
		select {
		case result := <-clientResult:
			if result.err != nil {
				return fmt.Errorf("open legacy WebSocket bridge: %w", result.err)
			}
			client = result.connection
		case <-ctx.Done():
			return ctx.Err()
		}
		defer client.Close()
		return relayBridgeWebSocket(ctx, client, stream, handlerDone)
	}
}

type webSocketClientResult struct {
	connection *websocket.Conn
	err        error
}

func prepareWebSocketUpgradeRequest(request *http.Request) {
	if request == nil {
		return
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
}

func bridgeWebSocketURL(source *url.URL) *url.URL {
	if source == nil {
		return &url.URL{Scheme: "ws", Host: "package-bridge", Path: "/api/v2"}
	}
	copy := *source
	copy.Scheme = "ws"
	if copy.Host == "" {
		copy.Host = "package-bridge"
	}
	return &copy
}

func relayBridgeWebSocket(ctx context.Context, client *websocket.Conn, stream WebSocketStream, handlerDone <-chan struct{}) error {
	if client == nil || stream == nil {
		return ErrCapabilityRejected
	}
	results := make(chan error, 2)
	go func() {
		for {
			frame, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					results <- nil
					return
				}
				results <- err
				return
			}
			if frame.Close != nil {
				_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(int(frame.Close.Code), frame.Close.Reason), time.Now().Add(time.Second))
				results <- nil
				return
			}
			if err := client.WriteMessage(websocket.TextMessage, frame.Data); err != nil {
				results <- err
				return
			}
		}
	}()
	go func() {
		for {
			messageType, data, err := client.ReadMessage()
			if err != nil {
				if close := bridgeClose(err); close != nil {
					_ = stream.Send(WebSocketFrame{Close: close})
				}
				results <- nil
				return
			}
			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}
			if err := stream.Send(WebSocketFrame{Data: data}); err != nil {
				results <- err
				return
			}
		}
	}()

	select {
	case err := <-results:
		_ = client.Close()
		select {
		case <-handlerDone:
		case <-time.After(time.Second):
		}
		return err
	case <-ctx.Done():
		_ = client.Close()
		return ctx.Err()
	}
}

func bridgeClose(err error) *WebSocketClose {
	if err == nil {
		return nil
	}
	var closeError *websocket.CloseError
	if errors.As(err, &closeError) && closeError.Code >= websocket.CloseNormalClosure && closeError.Code <= 4999 {
		return &WebSocketClose{Code: uint32(closeError.Code), Reason: closeError.Text}
	}
	return &WebSocketClose{Code: websocket.CloseNormalClosure, Reason: "kernel WebSocket bridge closed"}
}

type bridgeWebSocketResponseWriter struct {
	connection net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	header     http.Header

	mu             sync.Mutex
	consumeOnce    sync.Once
	consumeErr     error
	clientRequest  *http.Request
	wroteHeader    bool
	hijacked       bool
	responseStatus int
}

func newBridgeWebSocketResponseWriter(connection net.Conn) *bridgeWebSocketResponseWriter {
	return &bridgeWebSocketResponseWriter{
		connection: connection,
		reader:     bufio.NewReader(connection),
		writer:     bufio.NewWriter(connection),
		header:     make(http.Header),
	}
}

func (w *bridgeWebSocketResponseWriter) Header() http.Header { return w.header }

func (w *bridgeWebSocketResponseWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wroteHeader || w.hijacked {
		return
	}
	if err := w.consumeClientHandshake(); err != nil {
		return
	}
	w.wroteHeader = true
	w.responseStatus = status
	_, _ = fmt.Fprintf(w.writer, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	for name, values := range w.header {
		for _, value := range values {
			_, _ = fmt.Fprintf(w.writer, "%s: %s\r\n", name, value)
		}
	}
	_, _ = w.writer.WriteString("\r\n")
	_ = w.writer.Flush()
}

func (w *bridgeWebSocketResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hijacked || w.consumeErr != nil {
		return 0, net.ErrClosed
	}
	count, err := w.writer.Write(data)
	if err != nil {
		return count, err
	}
	return count, w.writer.Flush()
}

func (w *bridgeWebSocketResponseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.hijacked {
		_ = w.writer.Flush()
	}
}

func (w *bridgeWebSocketResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hijacked {
		return nil, nil, net.ErrClosed
	}
	if err := w.consumeClientHandshake(); err != nil {
		return nil, nil, err
	}
	w.hijacked = true
	return w.connection, bufio.NewReadWriter(w.reader, w.writer), nil
}

func (w *bridgeWebSocketResponseWriter) consumeClientHandshake() error {
	w.consumeOnce.Do(func() {
		request, err := http.ReadRequest(w.reader)
		if err != nil {
			w.consumeErr = err
			return
		}
		w.clientRequest = request
		if request.Body != nil {
			_ = request.Body.Close()
		}
	})
	return w.consumeErr
}

func (w *bridgeWebSocketResponseWriter) clientChallengeKey() string {
	if w == nil || w.clientRequest == nil {
		return ""
	}
	return w.clientRequest.Header.Get("Sec-WebSocket-Key")
}

var _ http.Hijacker = (*bridgeWebSocketResponseWriter)(nil)
var _ http.Flusher = (*bridgeWebSocketResponseWriter)(nil)
