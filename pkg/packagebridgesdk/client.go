// Package packagebridgesdk is the narrow client surface available to a
// package-host executable. It communicates only over its inherited bridge FD.
package packagebridgesdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const BridgeFDEnvironment = "ANIX_CONTROL_PACKAGE_BRIDGE_FD"

var (
	ErrBridgeUnavailable  = errors.New("package bridge unavailable")
	ErrCapabilityRejected = errors.New("package bridge capability rejected")
)

type Client struct {
	connection *grpc.ClientConn
	rpc        packagebridgev1.KernelPackageBridgeClient
}

type Header struct {
	Name  string
	Value string
}

type Response struct {
	StatusCode uint32
	Body       []byte
	Headers    []Header
}

type WebSocketClose struct {
	Code   uint32
	Reason string
}

// WebSocketFrame is exchanged after the private bridge opening frame. A nil
// Close denotes a data frame, including an empty payload.
type WebSocketFrame struct {
	Data  []byte
	Close *WebSocketClose
}

type WebSocketStream interface {
	Recv() (WebSocketFrame, error)
	Send(WebSocketFrame) error
	CloseSend() error
}

func DialFromEnvironment(ctx context.Context) (*Client, error) {
	rawFD := os.Getenv(BridgeFDEnvironment)
	fd, err := strconv.Atoi(rawFD)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("%w: inherited bridge descriptor is required", ErrBridgeUnavailable)
	}
	return DialFile(ctx, os.NewFile(uintptr(fd), "anix-package-bridge-child"))
}

// DialFile consumes a duplicate of file and leaves no filesystem-addressable
// bridge endpoint for a package to discover or replace.
func DialFile(_ context.Context, file *os.File) (*Client, error) {
	if file == nil {
		return nil, ErrBridgeUnavailable
	}
	connection, err := net.FileConn(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		if connection != nil {
			_ = connection.Close()
		}
		if err != nil {
			return nil, fmt.Errorf("%w: open bridge descriptor: %v", ErrBridgeUnavailable, err)
		}
		return nil, fmt.Errorf("%w: close bridge descriptor: %v", ErrBridgeUnavailable, closeErr)
	}
	var once sync.Once
	var dialConnection net.Conn
	dialer := func(context.Context, string) (net.Conn, error) {
		used := false
		once.Do(func() {
			used = true
			dialConnection = connection
		})
		if !used {
			return nil, net.ErrClosed
		}
		return dialConnection, nil
	}
	grpcConnection, err := grpc.NewClient(
		"passthrough:///anix-package-bridge",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		_ = connection.Close()
		return nil, bridgeError(err)
	}
	return &Client{connection: grpcConnection, rpc: packagebridgev1.NewKernelPackageBridgeClient(grpcConnection)}, nil
}

func (c *Client) Invoke(ctx context.Context, capability []byte, operation string, payload []byte) (Response, error) {
	if c == nil || c.rpc == nil {
		return Response{}, ErrBridgeUnavailable
	}
	response, err := c.rpc.Invoke(ctx, &packagebridgev1.InvokeRequest{
		Capability: append([]byte(nil), capability...), Operation: operation, Payload: append([]byte(nil), payload...),
	})
	if err != nil {
		return Response{}, bridgeError(err)
	}
	headers := make([]Header, len(response.GetHeaders()))
	for index, header := range response.GetHeaders() {
		headers[index] = Header{Name: header.GetName(), Value: header.GetValue()}
	}
	return Response{StatusCode: response.GetStatusCode(), Body: append([]byte(nil), response.GetResponseBody()...), Headers: headers}, nil
}

func (c *Client) OpenWebSocket(ctx context.Context, capability []byte, operation string) (WebSocketStream, error) {
	if c == nil || c.rpc == nil {
		return nil, ErrBridgeUnavailable
	}
	if len(capability) != 32 || strings.TrimSpace(operation) == "" {
		return nil, ErrCapabilityRejected
	}
	stream, err := c.rpc.OpenWebSocket(ctx)
	if err != nil {
		return nil, bridgeError(err)
	}
	if err := stream.Send(&packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Open{Open: &packagebridgev1.WebSocketOpen{
		Capability: append([]byte(nil), capability...), Operation: operation,
	}}}); err != nil {
		_ = stream.CloseSend()
		return nil, bridgeError(err)
	}
	return &webSocketClientStream{stream: stream}, nil
}

func (c *Client) Close() error {
	if c == nil || c.connection == nil {
		return nil
	}
	return c.connection.Close()
}

type webSocketClientStream struct {
	stream packagebridgev1.KernelPackageBridge_OpenWebSocketClient
}

func (s *webSocketClientStream) Recv() (WebSocketFrame, error) {
	if s == nil || s.stream == nil {
		return WebSocketFrame{}, ErrBridgeUnavailable
	}
	frame, err := s.stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return WebSocketFrame{}, io.EOF
		}
		return WebSocketFrame{}, bridgeError(err)
	}
	switch value := frame.Value.(type) {
	case *packagebridgev1.WebSocketFrame_Data:
		return WebSocketFrame{Data: append([]byte(nil), value.Data...)}, nil
	case *packagebridgev1.WebSocketFrame_Close:
		if value.Close == nil {
			return WebSocketFrame{}, ErrBridgeUnavailable
		}
		return WebSocketFrame{Close: &WebSocketClose{Code: value.Close.GetCode(), Reason: value.Close.GetReason()}}, nil
	default:
		return WebSocketFrame{}, ErrBridgeUnavailable
	}
}

func (s *webSocketClientStream) Send(frame WebSocketFrame) error {
	if s == nil || s.stream == nil {
		return ErrBridgeUnavailable
	}
	if frame.Close != nil && len(frame.Data) != 0 {
		return ErrBridgeUnavailable
	}
	var wire *packagebridgev1.WebSocketFrame
	if frame.Close != nil {
		wire = &packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Close{Close: &packagebridgev1.WebSocketClose{
			Code: frame.Close.Code, Reason: frame.Close.Reason,
		}}}
	} else {
		wire = &packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Data{Data: append([]byte(nil), frame.Data...)}}
	}
	if err := s.stream.Send(wire); err != nil {
		return bridgeError(err)
	}
	return nil
}

func (s *webSocketClientStream) CloseSend() error {
	if s == nil || s.stream == nil {
		return nil
	}
	if err := s.stream.CloseSend(); err != nil {
		return bridgeError(err)
	}
	return nil
}

func bridgeError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.PermissionDenied:
		return fmt.Errorf("%w: %s", ErrCapabilityRejected, status.Code(err))
	case codes.DeadlineExceeded, codes.Canceled, codes.Unavailable:
		return fmt.Errorf("%w: %s", ErrBridgeUnavailable, status.Code(err))
	default:
		return fmt.Errorf("%w: %s", ErrBridgeUnavailable, status.Code(err))
	}
}
