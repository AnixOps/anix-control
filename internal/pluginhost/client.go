package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type hostClient struct {
	connection *grpc.ClientConn
	rpc        pluginhostv1.ControlPackageHostClient
}

func dialHostClient(ctx context.Context, socketPath string) (*hostClient, error) {
	connection, err := grpc.DialContext(
		ctx,
		"passthrough:///anix-control-plugin-host",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, hostClientError(err)
	}
	return &hostClient{connection: connection, rpc: pluginhostv1.NewControlPackageHostClient(connection)}, nil
}

func (c *hostClient) Dispatch(ctx context.Context, input DispatchInput) (DispatchOutput, error) {
	if c == nil || c.rpc == nil {
		return DispatchOutput{}, ErrHostUnavailable
	}
	metadata, err := marshalRequestMetadata(input.Metadata)
	if err != nil {
		return DispatchOutput{}, err
	}
	response, err := c.rpc.Dispatch(ctx, &pluginhostv1.DispatchRequest{
		PackageId:           input.PackageID,
		PackageVersion:      input.Version,
		RouteGeneration:     input.Generation,
		RequestId:           input.RequestID,
		IdempotencyKey:      input.IdempotencyKey,
		RouteId:             input.RouteID,
		Method:              input.Method,
		RequestBody:         append([]byte(nil), input.Body...),
		PrincipalJson:       append([]byte(nil), input.PrincipalJSON...),
		DeadlineUnixMillis:  input.Deadline.UnixMilli(),
		RequestMetadataJson: metadata,
		BridgeCapability:    append([]byte(nil), input.BridgeCapability...),
	})
	if err != nil {
		return DispatchOutput{}, hostClientError(err)
	}
	headers := make([]Header, len(response.GetHeaders()))
	for index, header := range response.GetHeaders() {
		headers[index] = Header{Name: header.GetName(), Value: header.GetValue()}
	}
	return DispatchOutput{
		StatusCode: response.GetStatusCode(), Body: append([]byte(nil), response.GetResponseBody()...), Headers: headers,
		OperationID: response.GetOperationId(), FailureCode: response.GetFailureCode(),
	}, nil
}

func (c *hostClient) OpenWebSocket(ctx context.Context, input WebSocketInput) (webSocketTransport, error) {
	if c == nil || c.rpc == nil {
		return nil, ErrHostUnavailable
	}
	metadata, err := marshalRequestMetadata(input.Metadata)
	if err != nil {
		return nil, err
	}
	stream, err := c.rpc.OpenWebSocket(ctx)
	if err != nil {
		return nil, hostClientError(err)
	}
	if err := stream.Send(&pluginhostv1.WebSocketFrame{Value: &pluginhostv1.WebSocketFrame_Open{Open: &pluginhostv1.WebSocketOpen{
		PackageId:           input.PackageID,
		PackageVersion:      input.Version,
		RouteGeneration:     input.Generation,
		RouteId:             input.RouteID,
		PrincipalJson:       append([]byte(nil), input.PrincipalJSON...),
		RequestId:           input.RequestID,
		IdempotencyKey:      input.IdempotencyKey,
		DeadlineUnixMillis:  input.Deadline.UnixMilli(),
		RequestMetadataJson: metadata,
		BridgeCapability:    append([]byte(nil), input.BridgeCapability...),
	}}}); err != nil {
		_ = stream.CloseSend()
		return nil, hostClientError(err)
	}
	return &webSocketClientStream{stream: stream}, nil
}

func marshalRequestMetadata(metadata RequestMetadata) ([]byte, error) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("%w: request metadata is invalid", ErrHostIncompatible)
	}
	return encoded, nil
}

func (c *hostClient) Health(ctx context.Context, generation uint64) (HostHealth, error) {
	if c == nil || c.rpc == nil {
		return HostHealth{}, ErrHostUnavailable
	}
	response, err := c.rpc.Health(ctx, &pluginhostv1.HealthRequest{RouteGeneration: generation})
	if err != nil {
		return HostHealth{}, hostClientError(err)
	}
	return HostHealth{Healthy: response.GetHealthy(), LeaseID: response.GetLeaseId(), DetailsJSON: response.GetDetailsJson()}, nil
}

func (c *hostClient) Drain(ctx context.Context, generation uint64, deadline time.Time) (DrainResult, error) {
	if c == nil || c.rpc == nil {
		return DrainResult{}, ErrHostUnavailable
	}
	response, err := c.rpc.Drain(ctx, &pluginhostv1.DrainRequest{
		RouteGeneration: generation, DeadlineUnixMillis: deadline.UnixMilli(),
	})
	if err != nil {
		return DrainResult{}, hostClientError(err)
	}
	return DrainResult{Drained: response.GetDrained(), InFlight: response.GetInFlight()}, nil
}

func (c *hostClient) Close() error {
	if c == nil || c.connection == nil {
		return nil
	}
	return c.connection.Close()
}

type webSocketClientStream struct {
	stream pluginhostv1.ControlPackageHost_OpenWebSocketClient
}

func (s *webSocketClientStream) Send(frame WebSocketFrame) error {
	if s == nil || s.stream == nil {
		return ErrHostUnavailable
	}
	if frame.Close != nil && len(frame.Data) != 0 {
		return fmt.Errorf("%w: WebSocket frame cannot contain data and close", ErrHostIncompatible)
	}
	var wireFrame *pluginhostv1.WebSocketFrame
	if frame.Close != nil {
		wireFrame = &pluginhostv1.WebSocketFrame{Value: &pluginhostv1.WebSocketFrame_Close{Close: &pluginhostv1.WebSocketClose{
			Code: frame.Close.Code, Reason: frame.Close.Reason,
		}}}
	} else {
		wireFrame = &pluginhostv1.WebSocketFrame{Value: &pluginhostv1.WebSocketFrame_Data{Data: append([]byte(nil), frame.Data...)}}
	}
	if err := s.stream.Send(wireFrame); err != nil {
		return hostClientError(err)
	}
	return nil
}

func (s *webSocketClientStream) Recv() (WebSocketFrame, error) {
	if s == nil || s.stream == nil {
		return WebSocketFrame{}, ErrHostUnavailable
	}
	frame, err := s.stream.Recv()
	if errors.Is(err, io.EOF) {
		return WebSocketFrame{}, io.EOF
	}
	if err != nil {
		return WebSocketFrame{}, hostClientError(err)
	}
	switch value := frame.Value.(type) {
	case *pluginhostv1.WebSocketFrame_Data:
		return WebSocketFrame{Data: append([]byte(nil), value.Data...)}, nil
	case *pluginhostv1.WebSocketFrame_Close:
		if value.Close == nil {
			return WebSocketFrame{}, fmt.Errorf("%w: WebSocket close frame is required", ErrHostIncompatible)
		}
		return WebSocketFrame{Close: &WebSocketClose{Code: value.Close.GetCode(), Reason: value.Close.GetReason()}}, nil
	default:
		return WebSocketFrame{}, fmt.Errorf("%w: WebSocket opening frame must be first and unique", ErrHostIncompatible)
	}
}

func (s *webSocketClientStream) CloseSend() error {
	if s == nil || s.stream == nil {
		return nil
	}
	return s.stream.CloseSend()
}

func hostClientError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrHostUnavailable, err)
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.Unimplemented, codes.DataLoss:
		return fmt.Errorf("%w: %s", ErrHostIncompatible, status.Code(err))
	default:
		return fmt.Errorf("%w: %s", ErrHostUnavailable, status.Code(err))
	}
}
