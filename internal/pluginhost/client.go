package pluginhost

import (
	"context"
	"errors"
	"fmt"
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
	response, err := c.rpc.Dispatch(ctx, &pluginhostv1.DispatchRequest{
		PackageId:          input.PackageID,
		PackageVersion:     input.Version,
		RouteGeneration:    input.Generation,
		RequestId:          input.RequestID,
		IdempotencyKey:     input.IdempotencyKey,
		RouteId:            input.RouteID,
		Method:             input.Method,
		RequestBody:        append([]byte(nil), input.Body...),
		PrincipalJson:      append([]byte(nil), input.PrincipalJSON...),
		DeadlineUnixMillis: input.Deadline.UnixMilli(),
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
