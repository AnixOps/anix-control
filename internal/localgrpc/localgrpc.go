// Package localgrpc serves gRPC services to callers in the same process
// without a transport: a call runs the registered handler on the caller's
// goroutine (unary) or on a goroutine of its own (streams), and messages are
// copied, never encoded.
package localgrpc

import (
	"context"
	"io"
	"log"
	"runtime/debug"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Channel is both the registrar the services are registered on and the
// connection their clients use.
type Channel struct {
	// ServerContext, when set, derives every handler's context, as a
	// server interceptor would.
	ServerContext func(context.Context) context.Context

	mu       sync.RWMutex
	services map[string]service
	// closed refuses new calls; running ends the handlers that run.
	closed  bool
	running map[*context.CancelFunc]struct{}
}

type service struct {
	desc *grpc.ServiceDesc
	impl any
}

var (
	_ grpc.ServiceRegistrar    = (*Channel)(nil)
	_ grpc.ClientConnInterface = (*Channel)(nil)
)

// RegisterService implements grpc.ServiceRegistrar.
func (c *Channel) RegisterService(desc *grpc.ServiceDesc, impl any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.services == nil {
		c.services = make(map[string]service)
	}
	c.services[desc.ServiceName] = service{desc: desc, impl: impl}
}

// Close ends every running handler and refuses later calls with
// codes.Unavailable.
func (c *Channel) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for cancel := range c.running {
		(*cancel)()
	}
}

// handlerContext is the context a handler runs with: the caller's, with its
// outgoing metadata as incoming metadata as a transport would, also ended
// by Close.
func (c *Channel) handlerContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	outgoing, _ := metadata.FromOutgoingContext(ctx)
	ctx, cancel := context.WithCancel(metadata.NewIncomingContext(ctx, outgoing.Copy()))
	key := &cancel
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel()
		return nil, nil, status.Error(codes.Unavailable, "channel is closed")
	}
	if c.running == nil {
		c.running = make(map[*context.CancelFunc]struct{})
	}
	c.running[key] = struct{}{}
	c.mu.Unlock()
	if c.ServerContext != nil {
		ctx = c.ServerContext(ctx)
	}
	return ctx, func() {
		c.mu.Lock()
		delete(c.running, key)
		c.mu.Unlock()
		cancel()
	}, nil
}

func (c *Channel) lookup(method string) (service, string, error) {
	name, methodName, ok := strings.Cut(strings.TrimPrefix(method, "/"), "/")
	c.mu.RLock()
	found, registered := c.services[name]
	c.mu.RUnlock()
	if !ok || !registered {
		return service{}, "", status.Errorf(codes.Unimplemented, "unknown service %s", name)
	}
	return found, methodName, nil
}

// Invoke runs a unary method.
func (c *Channel) Invoke(ctx context.Context, method string, args, reply any, _ ...grpc.CallOption) (err error) {
	found, name, err := c.lookup(method)
	if err != nil {
		return err
	}
	for _, desc := range found.desc.Methods {
		if desc.MethodName != name {
			continue
		}
		handlerCtx, cancel, contextErr := c.handlerContext(ctx)
		if contextErr != nil {
			return contextErr
		}
		defer cancel()
		defer recoverPanic(method, &err)
		decode := func(target any) error {
			proto.Merge(target.(proto.Message), args.(proto.Message))
			return nil
		}
		result, err := desc.Handler(found.impl, handlerCtx, decode, nil)
		if err != nil {
			return err
		}
		proto.Merge(reply.(proto.Message), result.(proto.Message))
		return nil
	}
	return status.Errorf(codes.Unimplemented, "unknown method %s", method)
}

// NewStream runs a streaming method on its own goroutine.
func (c *Channel) NewStream(ctx context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	found, name, err := c.lookup(method)
	if err != nil {
		return nil, err
	}
	for _, desc := range found.desc.Streams {
		if desc.StreamName != name {
			continue
		}
		serverCtx, cancel, err := c.handlerContext(ctx)
		if err != nil {
			return nil, err
		}
		pipe := &pipe{
			clientCtx: ctx, serverCtx: serverCtx,
			toServer: make(chan proto.Message), toClient: make(chan proto.Message),
			sendClosed: make(chan struct{}), done: make(chan struct{}),
		}
		go func() {
			defer close(pipe.done)
			defer cancel()
			defer recoverPanic(method, &pipe.err)
			pipe.err = desc.Handler(found.impl, (*serverStream)(pipe))
		}()
		return (*clientStream)(pipe), nil
	}
	return nil, status.Errorf(codes.Unimplemented, "unknown method %s", method)
}

// recoverPanic turns a handler panic into codes.Internal, like the
// recovery interceptors of a served host did.
func recoverPanic(method string, err *error) {
	if recovered := recover(); recovered != nil {
		log.Printf("panic recovered in %s: %v\n%s", method, recovered, debug.Stack())
		*err = status.Error(codes.Internal, "internal error")
	}
}

// pipe joins one client stream to its handler. The channels are unbuffered,
// so every message the handler sends is received before it returns.
type pipe struct {
	clientCtx, serverCtx context.Context
	toServer, toClient   chan proto.Message
	closeSendOnce        sync.Once
	sendClosed           chan struct{}
	done                 chan struct{}
	err                  error
}

type clientStream pipe

func (s *clientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *clientStream) Trailer() metadata.MD         { return nil }
func (s *clientStream) Context() context.Context     { return s.clientCtx }

func (s *clientStream) CloseSend() error {
	s.closeSendOnce.Do(func() { close(s.sendClosed) })
	return nil
}

func (s *clientStream) SendMsg(message any) error {
	if err := s.clientCtx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	select {
	case s.toServer <- proto.Clone(message.(proto.Message)):
		return nil
	case <-s.done:
		return io.EOF
	case <-s.clientCtx.Done():
		return status.FromContextError(s.clientCtx.Err()).Err()
	}
}

func (s *clientStream) RecvMsg(message any) error {
	if err := s.clientCtx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	select {
	case received := <-s.toClient:
		proto.Merge(message.(proto.Message), received)
		return nil
	case <-s.done:
		if s.err != nil {
			return s.err
		}
		return io.EOF
	case <-s.clientCtx.Done():
		return status.FromContextError(s.clientCtx.Err()).Err()
	}
}

type serverStream pipe

func (s *serverStream) SetHeader(metadata.MD) error  { return nil }
func (s *serverStream) SendHeader(metadata.MD) error { return nil }
func (s *serverStream) SetTrailer(metadata.MD)       {}
func (s *serverStream) Context() context.Context     { return s.serverCtx }

func (s *serverStream) SendMsg(message any) error {
	if err := s.serverCtx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	select {
	case s.toClient <- proto.Clone(message.(proto.Message)):
		return nil
	case <-s.serverCtx.Done():
		return status.FromContextError(s.serverCtx.Err()).Err()
	}
}

func (s *serverStream) RecvMsg(message any) error {
	if err := s.serverCtx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	select {
	case received := <-s.toServer:
		proto.Merge(message.(proto.Message), received)
		return nil
	case <-s.sendClosed:
		return io.EOF
	case <-s.serverCtx.Done():
		return status.FromContextError(s.serverCtx.Err()).Err()
	}
}
