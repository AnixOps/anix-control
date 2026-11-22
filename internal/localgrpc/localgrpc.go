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
	mu       sync.RWMutex
	services map[string]service
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
		defer recoverPanic(method, &err)
		decode := func(target any) error {
			proto.Merge(target.(proto.Message), args.(proto.Message))
			return nil
		}
		result, err := desc.Handler(found.impl, serverContext(ctx), decode, nil)
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
		serverCtx, cancel := context.WithCancel(serverContext(ctx))
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

// serverContext gives the handler the caller's outgoing metadata as incoming
// metadata, as a transport would.
func serverContext(ctx context.Context) context.Context {
	outgoing, _ := metadata.FromOutgoingContext(ctx)
	return metadata.NewIncomingContext(ctx, outgoing.Copy())
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
	select {
	case s.toClient <- proto.Clone(message.(proto.Message)):
		return nil
	case <-s.serverCtx.Done():
		return status.FromContextError(s.serverCtx.Err()).Err()
	}
}

func (s *serverStream) RecvMsg(message any) error {
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
