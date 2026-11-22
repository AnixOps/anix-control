//go:build unix

package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/localgrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// packageClient calls a package the host built (modulesdk.InProcess.Ready)
// directly. A panic or an error of the package fails only that call.
type packageClient struct {
	packageImpl      pluginhostsdk.Package
	maxResponseBytes int64
	// services holds what the package serves beside the host protocol
	// (modulesdk.ServiceRegistrant), such as IdentityService.
	services *localgrpc.Channel
}

func newPackageClient(packageImpl pluginhostsdk.Package, maxResponseBytes int64) *packageClient {
	client := &packageClient{packageImpl: packageImpl, maxResponseBytes: normalizeMaxResponseBytes(maxResponseBytes)}
	if registrant, ok := packageImpl.(modulesdk.ServiceRegistrant); ok {
		client.services = &localgrpc.Channel{}
		registrant.RegisterServices(client.services)
	}
	return client
}

func (c *packageClient) Dispatch(ctx context.Context, input DispatchInput) (output DispatchOutput, err error) {
	defer recoverPackagePanic("dispatch", &err)
	ctx, cancel := context.WithDeadline(ctx, input.Deadline)
	defer cancel()
	response, err := c.packageImpl.Dispatch(ctx, pluginhostsdk.DispatchRequest{
		PackageID: input.PackageID, PackageVersion: input.Version, RouteGeneration: input.Generation,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey, RouteID: input.RouteID, Method: input.Method,
		RequestBody: append([]byte(nil), input.Body...), PrincipalJSON: append([]byte(nil), input.PrincipalJSON...),
		Metadata: packageRequestMetadata(input.Metadata), BridgeCapability: append([]byte(nil), input.BridgeCapability...),
		DeadlineUnixMillis: input.Deadline.UnixMilli(),
	})
	if err != nil {
		return DispatchOutput{}, packageCallError(err)
	}
	if response.StatusCode < 100 || response.StatusCode > 599 {
		return DispatchOutput{}, fmt.Errorf("%w: package response status code is invalid", ErrHostIncompatible)
	}
	if int64(len(response.ResponseBody)) > c.maxResponseBytes {
		return DispatchOutput{}, fmt.Errorf("%w: %w", ErrHostUnavailable, ErrResponseTooLarge)
	}
	headers := make([]Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = Header{Name: header.Name, Value: header.Value}
	}
	return DispatchOutput{
		StatusCode: response.StatusCode, Body: response.ResponseBody, Headers: headers,
		OperationID: response.OperationID, FailureCode: response.FailureCode,
	}, nil
}

func (c *packageClient) Migrate(ctx context.Context, input MigrationInput) (output MigrationOutput, err error) {
	defer recoverPackagePanic("migrate", &err)
	if !input.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, input.Deadline)
		defer cancel()
	}
	response, err := c.packageImpl.Migrate(ctx, pluginhostsdk.MigrationRequest{
		PackageID: input.PackageID, PackageVersion: input.Version, MigrationID: input.MigrationID,
		Checkpoint: input.Checkpoint, RouteGeneration: input.Generation,
		BridgeCapability: append([]byte(nil), input.BridgeCapability...), DeadlineUnixMillis: input.Deadline.UnixMilli(),
	})
	if err != nil {
		return MigrationOutput{}, packageCallError(err)
	}
	return MigrationOutput{
		Checkpoint: response.Checkpoint, ValidationDigest: response.ValidationDigest,
		Complete: response.Complete, FailureCode: response.FailureCode,
	}, nil
}

func (c *packageClient) Health(ctx context.Context, _ uint64) (health HostHealth, err error) {
	defer recoverPackagePanic("health", &err)
	response, err := c.packageImpl.Health(ctx)
	if err != nil {
		return HostHealth{}, packageCallError(err)
	}
	return HostHealth{Healthy: response.Healthy, LeaseID: response.LeaseID, DetailsJSON: response.DetailsJSON}, nil
}

func (c *packageClient) Drain(ctx context.Context, _ uint64, deadline time.Time) (result DrainResult, err error) {
	defer recoverPackagePanic("drain", &err)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	response, err := c.packageImpl.Drain(ctx)
	if err != nil {
		return DrainResult{}, packageCallError(err)
	}
	return DrainResult{Drained: response.Drained, InFlight: response.InFlight}, nil
}

func (c *packageClient) OpenWebSocket(ctx context.Context, input WebSocketInput) (webSocketTransport, error) {
	packageImpl, ok := c.packageImpl.(pluginhostsdk.WebSocketPackage)
	if !ok {
		return nil, fmt.Errorf("%w: package does not implement WebSocket routes", ErrHostIncompatible)
	}
	ctx, cancel := context.WithDeadline(ctx, input.Deadline)
	pipe := &webSocketPipe{
		ctx: ctx, toPackage: make(chan WebSocketFrame), toKernel: make(chan WebSocketFrame),
		sendClosed: make(chan struct{}), done: make(chan struct{}),
	}
	open := pluginhostsdk.WebSocketOpen{
		PackageID: input.PackageID, PackageVersion: input.Version, RouteGeneration: input.Generation, RouteID: input.RouteID,
		PrincipalJSON: append([]byte(nil), input.PrincipalJSON...), Metadata: packageRequestMetadata(input.Metadata),
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey, DeadlineUnixMillis: input.Deadline.UnixMilli(),
		BridgeCapability: append([]byte(nil), input.BridgeCapability...),
	}
	go func() {
		defer close(pipe.done)
		defer cancel()
		defer recoverPackagePanic("open WebSocket", &pipe.err)
		if err := packageImpl.OpenWebSocket(ctx, open, (*packageWebSocketStream)(pipe)); err != nil {
			pipe.err = packageCallError(err)
		}
	}()
	return pipe, nil
}

// packageRequestMetadata is the request address as the package sees it: the
// scheme the kernel resolved (internal/requestorigin) and the validated Host.
func packageRequestMetadata(metadata RequestMetadata) pluginhostsdk.RequestMetadata {
	scheme := "http"
	if metadata.TLS {
		scheme = "https"
	}
	return pluginhostsdk.RequestMetadata{
		Path: metadata.Path, Query: metadata.Query, Headers: metadata.Headers, PathParams: metadata.PathParams,
		ClientIP: metadata.ClientIP, UserAgent: metadata.UserAgent, Scheme: scheme, Host: metadata.Host,
	}
}

// marshalBridgeRequestMetadata encodes the kernel-side bridge snapshot, which
// adds the original request address to the request metadata.
func marshalBridgeRequestMetadata(metadata RequestMetadata) ([]byte, error) {
	encoded, err := json.Marshal(struct {
		RequestMetadata
		Host string `json:"host,omitempty"`
		TLS  bool   `json:"tls,omitempty"`
	}{RequestMetadata: metadata, Host: metadata.Host, TLS: metadata.TLS})
	if err != nil {
		return nil, fmt.Errorf("%w: request metadata is invalid", ErrHostIncompatible)
	}
	return encoded, nil
}

// packageCallError classifies what a package returned: a status error keeps
// its code, a plain error is a package failure.
func packageCallError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrHostUnavailable, err)
	}
	switch status.Code(err) {
	case codes.Unknown, codes.Internal:
		return fmt.Errorf("%w: %w", ErrHostUnavailable, ErrPackageFailed)
	case codes.InvalidArgument, codes.FailedPrecondition, codes.Unimplemented, codes.DataLoss:
		return fmt.Errorf("%w: %s", ErrHostIncompatible, status.Code(err))
	case codes.ResourceExhausted:
		return fmt.Errorf("%w: %w", ErrHostUnavailable, ErrResponseTooLarge)
	default:
		return fmt.Errorf("%w: %s", ErrHostUnavailable, status.Code(err))
	}
}

// recoverPackagePanic must be deferred directly: a panic in a package call
// is logged and fails that call only.
func recoverPackagePanic(operation string, err *error) {
	if recovered := recover(); recovered != nil {
		log.Printf("panic recovered in package %s: %v\n%s", operation, recovered, debug.Stack())
		*err = fmt.Errorf("%w: %w", ErrHostUnavailable, ErrPackageFailed)
	}
}

// webSocketPipe joins the kernel's relay to the package's stream. The
// channels are unbuffered, so every frame the package sends is received
// before its OpenWebSocket returns.
type webSocketPipe struct {
	ctx                 context.Context
	toPackage, toKernel chan WebSocketFrame
	closeSendOnce       sync.Once
	sendClosed          chan struct{}
	done                chan struct{}
	err                 error
}

func (p *webSocketPipe) Send(frame WebSocketFrame) error {
	if frame.Close != nil && len(frame.Data) != 0 {
		return fmt.Errorf("%w: WebSocket frame cannot contain data and close", ErrHostIncompatible)
	}
	select {
	case p.toPackage <- copyFrame(frame):
		return nil
	case <-p.done:
		return io.EOF
	case <-p.ctx.Done():
		return packageCallError(p.ctx.Err())
	}
}

func (p *webSocketPipe) Recv() (WebSocketFrame, error) {
	select {
	case frame := <-p.toKernel:
		return frame, nil
	case <-p.done:
		if p.err != nil {
			return WebSocketFrame{}, p.err
		}
		return WebSocketFrame{}, io.EOF
	case <-p.ctx.Done():
		return WebSocketFrame{}, packageCallError(p.ctx.Err())
	}
}

func (p *webSocketPipe) CloseSend() error {
	p.closeSendOnce.Do(func() { close(p.sendClosed) })
	return nil
}

// packageWebSocketStream is the package's end of a webSocketPipe.
type packageWebSocketStream webSocketPipe

func (s *packageWebSocketStream) Recv() (pluginhostsdk.WebSocketFrame, error) {
	select {
	case frame := <-s.toPackage:
		if frame.Close != nil {
			return pluginhostsdk.WebSocketFrame{Close: &pluginhostsdk.WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}}, nil
		}
		return pluginhostsdk.WebSocketFrame{Data: frame.Data}, nil
	case <-s.sendClosed:
		return pluginhostsdk.WebSocketFrame{}, io.EOF
	case <-s.ctx.Done():
		return pluginhostsdk.WebSocketFrame{}, s.ctx.Err()
	}
}

func (s *packageWebSocketStream) Send(frame pluginhostsdk.WebSocketFrame) error {
	converted := WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
	if frame.Close != nil {
		converted = WebSocketFrame{Close: &WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}}
	}
	select {
	case s.toKernel <- converted:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func copyFrame(frame WebSocketFrame) WebSocketFrame {
	if frame.Close != nil {
		return WebSocketFrame{Close: &WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}}
	}
	return WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
}
