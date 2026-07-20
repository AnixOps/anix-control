// Package packagebridge exposes narrow, generation-bound kernel operations to
// a single supervised package host. It intentionally has no database, file,
// configuration, or arbitrary-query API.
package packagebridge

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	capabilityBytes       = 32
	maxResponseBodyBytes  = 1 << 20
	maxResponseHeaderSize = 8192
)

var (
	ErrCapabilityRejected = errors.New("package bridge capability rejected")
	ErrBridgeClosed       = errors.New("package bridge is closed")
)

// HostIdentity is fixed when a child process is started. The socketpair makes
// this identity process-local rather than client-asserted.
type HostIdentity struct {
	PackageID  string
	Version    string
	Generation uint64
}

// Request is kernel-derived request state retained behind a capability. A
// package never supplies these values to the bridge RPC.
type Request struct {
	RequestID     string
	RouteID       string
	Method        string
	Body          []byte
	PrincipalJSON []byte
	MetadataJSON  []byte
	Deadline      time.Time
}

// Call is delivered only after a capability, package identity, route, and
// operation have all been verified.
type Call struct {
	Host      HostIdentity
	Request   Request
	Operation string
	Payload   []byte
}

type Header struct {
	Name  string
	Value string
}

// Response is an explicit parent-produced answer to one allowed bridge
// operation. Package code cannot obtain this authority except by consuming a
// verified capability.
type Response struct {
	StatusCode uint32
	Body       []byte
	Headers    []Header
}

type OperationHandler func(context.Context, Call) (Response, error)

type WebSocketClose struct {
	Code   uint32
	Reason string
}

// WebSocketFrame is a post-open bridge frame. A nil Close denotes a data
// frame, including an empty payload.
type WebSocketFrame struct {
	Data  []byte
	Close *WebSocketClose
}

// WebSocketStream is the kernel-side end of a private package bridge stream.
// Exactly one reader and one writer may operate concurrently.
type WebSocketStream interface {
	Recv() (WebSocketFrame, error)
	Send(WebSocketFrame) error
}

type WebSocketOperationHandler func(context.Context, Call, WebSocketStream) error

// WebSocketOperationResolver provides exact streaming operations registered
// during router construction. It is separate from unary operations so a
// package cannot upgrade a route merely by changing a declaration.
type WebSocketOperationResolver interface {
	ResolveWebSocket(packageID, routeID, operation string) (WebSocketOperationHandler, bool)
}

type Operation struct {
	PackageID string
	RouteID   string
	Name      string
	Handler   OperationHandler
}

// OperationResolver provides an additional exact operation source. It is used
// for handlers registered during router construction, not for path matching or
// arbitrary package-supplied operation names.
type OperationResolver interface {
	Resolve(packageID, routeID, operation string) (OperationHandler, bool)
}

// Allowlist binds every bridge operation to one package and v2 route id.
type Allowlist struct {
	operations map[string]OperationHandler
	fallback   OperationResolver
}

// SessionFactory creates a fresh private endpoint for every supervised host.
type SessionFactory interface {
	NewSession(HostIdentity) (*Session, *os.File, error)
}

type Factory struct {
	allowlist         *Allowlist
	webSocketResolver WebSocketOperationResolver
}

func NewFactory(allowlist *Allowlist, webSocketResolvers ...WebSocketOperationResolver) *Factory {
	var webSocketResolver WebSocketOperationResolver
	if len(webSocketResolvers) > 0 {
		webSocketResolver = webSocketResolvers[0]
	}
	return &Factory{allowlist: allowlist, webSocketResolver: webSocketResolver}
}

func (f *Factory) NewSession(identity HostIdentity) (*Session, *os.File, error) {
	if f == nil {
		return nil, nil, ErrBridgeClosed
	}
	return NewSession(identity, f.allowlist, f.webSocketResolver)
}

func NewAllowlist(operations ...Operation) (*Allowlist, error) {
	return NewAllowlistWithFallback(nil, operations...)
}

// NewAllowlistWithFallback composes static package operations with a resolver
// that still requires an exact package, route, and operation match.
func NewAllowlistWithFallback(fallback OperationResolver, operations ...Operation) (*Allowlist, error) {
	allowlist := &Allowlist{operations: make(map[string]OperationHandler, len(operations)), fallback: fallback}
	for _, operation := range operations {
		if !safeIdentifier(operation.PackageID) || !safeIdentifier(operation.RouteID) || !safeIdentifier(operation.Name) || operation.Handler == nil {
			return nil, errors.New("package bridge operation is invalid")
		}
		key := operationKey(operation.PackageID, operation.RouteID, operation.Name)
		if _, exists := allowlist.operations[key]; exists {
			return nil, errors.New("package bridge operation is duplicated")
		}
		allowlist.operations[key] = operation.Handler
	}
	return allowlist, nil
}

func (a *Allowlist) Invoke(ctx context.Context, call Call) (Response, error) {
	if a == nil {
		return Response{}, ErrCapabilityRejected
	}
	handler, ok := a.resolve(call.Host.PackageID, call.Request.RouteID, call.Operation)
	if !ok {
		return Response{}, ErrCapabilityRejected
	}
	return handler(ctx, cloneCall(call))
}

func (a *Allowlist) Allows(packageID, routeID, operation string) bool {
	if a == nil {
		return false
	}
	_, ok := a.resolve(packageID, routeID, operation)
	return ok
}

func (a *Allowlist) resolve(packageID, routeID, operation string) (OperationHandler, bool) {
	if a == nil {
		return nil, false
	}
	if handler, ok := a.operations[operationKey(packageID, routeID, operation)]; ok {
		return handler, true
	}
	if a.fallback == nil {
		return nil, false
	}
	return a.fallback.Resolve(packageID, routeID, operation)
}

type capability struct {
	request Request
}

// Session owns one endpoint of an inherited Unix socketpair. The returned
// child file is passed once through exec.ExtraFiles and is never addressable
// by a filesystem path.
type Session struct {
	packagebridgev1.UnimplementedKernelPackageBridgeServer

	mu                sync.Mutex
	identity          HostIdentity
	handler           *Allowlist
	webSocketResolver WebSocketOperationResolver
	capabilities      map[string]capability
	listener          *singleConnListener
	server            *grpc.Server
	closed            bool
}

var _ packagebridgev1.KernelPackageBridgeServer = (*Session)(nil)

func NewSession(identity HostIdentity, handler *Allowlist, webSocketResolvers ...WebSocketOperationResolver) (*Session, *os.File, error) {
	if !safeIdentifier(identity.PackageID) || !safeIdentifier(identity.Version) || identity.Generation == 0 || handler == nil {
		return nil, nil, errors.New("package bridge session is invalid")
	}
	fds, err := newSessionSocketpair()
	if err != nil {
		return nil, nil, fmt.Errorf("create package bridge socketpair: %w", err)
	}
	parentFile := os.NewFile(uintptr(fds[0]), "anix-package-bridge-parent")
	childFile := os.NewFile(uintptr(fds[1]), "anix-package-bridge-child")
	if parentFile == nil || childFile == nil {
		if parentFile != nil {
			_ = parentFile.Close()
		} else {
			_ = closeSessionSocket(fds[0])
		}
		if childFile != nil {
			_ = childFile.Close()
		} else {
			_ = closeSessionSocket(fds[1])
		}
		return nil, nil, errors.New("create package bridge file handles")
	}
	parentConnection, err := net.FileConn(parentFile)
	closeErr := parentFile.Close()
	if err != nil || closeErr != nil {
		_ = childFile.Close()
		if parentConnection != nil {
			_ = parentConnection.Close()
		}
		if err != nil {
			return nil, nil, fmt.Errorf("open package bridge parent connection: %w", err)
		}
		return nil, nil, fmt.Errorf("close package bridge parent descriptor: %w", closeErr)
	}

	var webSocketResolver WebSocketOperationResolver
	if len(webSocketResolvers) > 0 {
		webSocketResolver = webSocketResolvers[0]
	}
	session := &Session{
		identity: identity, handler: handler, webSocketResolver: webSocketResolver, capabilities: make(map[string]capability),
		listener: newSingleConnListener(parentConnection), server: grpc.NewServer(),
	}
	packagebridgev1.RegisterKernelPackageBridgeServer(session.server, session)
	go func() { _ = session.server.Serve(session.listener) }()
	return session, childFile, nil
}

// Mint creates an opaque one-shot capability for a dispatch already admitted
// by the kernel. It is not an authority token that a package can fabricate.
func (s *Session) Mint(request Request) ([]byte, error) {
	if s == nil || !safeIdentifier(request.RequestID) || !safeIdentifier(request.RouteID) || request.Deadline.IsZero() || !request.Deadline.After(time.Now()) {
		return nil, ErrCapabilityRejected
	}
	var raw [capabilityBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("generate package bridge capability: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrBridgeClosed
	}
	s.capabilities[string(raw[:])] = capability{request: cloneRequest(request)}
	return append([]byte(nil), raw[:]...), nil
}

func (s *Session) Invoke(ctx context.Context, request *packagebridgev1.InvokeRequest) (*packagebridgev1.InvokeResponse, error) {
	if request == nil || len(request.GetCapability()) != capabilityBytes || !safeIdentifier(request.GetOperation()) {
		return nil, capabilityStatusError()
	}
	capability, err := s.take(request.GetCapability(), request.GetOperation())
	if err != nil {
		return nil, capabilityStatusError()
	}
	callContext, cancel := context.WithDeadline(ctx, capability.request.Deadline)
	defer cancel()
	response, err := s.handler.Invoke(callContext, Call{
		Host: s.identity, Request: cloneRequest(capability.request), Operation: request.GetOperation(), Payload: append([]byte(nil), request.GetPayload()...),
	})
	if err != nil {
		if errors.Is(err, ErrCapabilityRejected) {
			return nil, capabilityStatusError()
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.DeadlineExceeded, "package bridge operation deadline exceeded")
		}
		return nil, status.Error(codes.Internal, "package bridge operation failed")
	}
	if err := validateResponse(response); err != nil {
		return nil, status.Error(codes.Internal, "package bridge response is invalid")
	}
	headers := make([]*packagebridgev1.ResponseHeader, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = &packagebridgev1.ResponseHeader{Name: header.Name, Value: header.Value}
	}
	return &packagebridgev1.InvokeResponse{
		ResponseBody: append([]byte(nil), response.Body...), StatusCode: response.StatusCode, Headers: headers,
	}, nil
}

func (s *Session) OpenWebSocket(stream packagebridgev1.KernelPackageBridge_OpenWebSocketServer) error {
	if stream == nil {
		return status.Error(codes.InvalidArgument, "package bridge WebSocket stream is required")
	}
	first, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return status.Error(codes.InvalidArgument, "package bridge WebSocket opening frame is required")
	}
	if err != nil {
		return err
	}
	open := first.GetOpen()
	if open == nil || len(open.GetCapability()) != capabilityBytes || !safeIdentifier(open.GetOperation()) {
		return capabilityStatusError()
	}
	issued, handler, err := s.takeWebSocket(open.GetCapability(), open.GetOperation())
	if err != nil {
		return capabilityStatusError()
	}
	callContext, cancel := context.WithDeadline(stream.Context(), issued.request.Deadline)
	defer cancel()
	err = handler(callContext, Call{
		Host: s.identity, Request: cloneRequest(issued.request), Operation: open.GetOperation(),
	}, &sessionWebSocketStream{stream: stream})
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	if errors.Is(err, ErrCapabilityRejected) {
		return capabilityStatusError()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return status.Error(codes.DeadlineExceeded, "package bridge WebSocket deadline exceeded")
	}
	return status.Error(codes.Internal, "package bridge WebSocket operation failed")
}

func (s *Session) take(raw []byte, operation string) (capability, error) {
	if s == nil {
		return capability{}, ErrBridgeClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return capability{}, ErrBridgeClosed
	}
	issued, ok := s.capabilities[string(raw)]
	if !ok || !issued.request.Deadline.After(time.Now()) {
		delete(s.capabilities, string(raw))
		return capability{}, ErrCapabilityRejected
	}
	// Consume before dispatching. A caller cannot retry an operation after a
	// partial transport failure and accidentally turn a mutation into a replay.
	delete(s.capabilities, string(raw))
	if !s.handler.Allows(s.identity.PackageID, issued.request.RouteID, operation) {
		return capability{}, ErrCapabilityRejected
	}
	return issued, nil
}

func (s *Session) takeWebSocket(raw []byte, operation string) (capability, WebSocketOperationHandler, error) {
	if s == nil {
		return capability{}, nil, ErrBridgeClosed
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return capability{}, nil, ErrBridgeClosed
	}
	issued, ok := s.capabilities[string(raw)]
	if !ok || !issued.request.Deadline.After(time.Now()) {
		delete(s.capabilities, string(raw))
		s.mu.Unlock()
		return capability{}, nil, ErrCapabilityRejected
	}
	// Consume before resolver lookup so an unavailable stream operation cannot
	// be retried later as a privileged replay.
	delete(s.capabilities, string(raw))
	resolver := s.webSocketResolver
	identity := s.identity
	s.mu.Unlock()
	if resolver == nil {
		return capability{}, nil, ErrCapabilityRejected
	}
	handler, ok := resolver.ResolveWebSocket(identity.PackageID, issued.request.RouteID, operation)
	if !ok || handler == nil {
		return capability{}, nil, ErrCapabilityRejected
	}
	return issued, handler, nil
}

// Revoke removes an unconsumed capability once the outer package-host
// dispatch returns. Calling it after a successful bridge call is harmless.
func (s *Session) Revoke(raw []byte) {
	if s == nil || len(raw) != capabilityBytes {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		delete(s.capabilities, string(raw))
	}
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.capabilities = nil
	listener := s.listener
	server := s.server
	s.mu.Unlock()
	if server != nil {
		server.Stop()
	}
	if listener != nil {
		return listener.Close()
	}
	return nil
}

func capabilityStatusError() error {
	return status.Error(codes.PermissionDenied, "package bridge capability rejected")
}

func operationKey(packageID, routeID, operation string) string {
	return packageID + "\x00" + routeID + "\x00" + operation
}

func safeIdentifier(value string) bool {
	if value == "" || len(value) > 200 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func cloneRequest(request Request) Request {
	request.Body = append([]byte(nil), request.Body...)
	request.PrincipalJSON = append([]byte(nil), request.PrincipalJSON...)
	request.MetadataJSON = append([]byte(nil), request.MetadataJSON...)
	return request
}

func cloneCall(call Call) Call {
	call.Request = cloneRequest(call.Request)
	call.Payload = append([]byte(nil), call.Payload...)
	return call
}

func validateResponse(response Response) error {
	if response.StatusCode < 100 || response.StatusCode > 599 || len(response.Body) > maxResponseBodyBytes {
		return errors.New("package bridge response is invalid")
	}
	for _, header := range response.Headers {
		if header.Name == "" || len(header.Name) > 256 || len(header.Value) == 0 || len(header.Value) > maxResponseHeaderSize {
			return errors.New("package bridge response header is invalid")
		}
		for _, character := range header.Name {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return errors.New("package bridge response header is invalid")
			}
		}
		for _, character := range header.Value {
			if character == '\r' || character == '\n' || character == 0 {
				return errors.New("package bridge response header is invalid")
			}
		}
	}
	return nil
}

type sessionWebSocketStream struct {
	stream packagebridgev1.KernelPackageBridge_OpenWebSocketServer
}

func (s *sessionWebSocketStream) Recv() (WebSocketFrame, error) {
	if s == nil || s.stream == nil {
		return WebSocketFrame{}, ErrBridgeClosed
	}
	frame, err := s.stream.Recv()
	if err != nil {
		return WebSocketFrame{}, err
	}
	switch value := frame.Value.(type) {
	case *packagebridgev1.WebSocketFrame_Data:
		return WebSocketFrame{Data: append([]byte(nil), value.Data...)}, nil
	case *packagebridgev1.WebSocketFrame_Close:
		if value.Close == nil {
			return WebSocketFrame{}, errors.New("package bridge WebSocket close frame is invalid")
		}
		return WebSocketFrame{Close: &WebSocketClose{Code: value.Close.GetCode(), Reason: value.Close.GetReason()}}, nil
	default:
		return WebSocketFrame{}, errors.New("package bridge WebSocket opening frame must be first and unique")
	}
}

func (s *sessionWebSocketStream) Send(frame WebSocketFrame) error {
	if s == nil || s.stream == nil {
		return ErrBridgeClosed
	}
	if frame.Close != nil && len(frame.Data) != 0 {
		return errors.New("package bridge WebSocket frame cannot contain data and close")
	}
	if frame.Close != nil {
		return s.stream.Send(&packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Close{Close: &packagebridgev1.WebSocketClose{
			Code: frame.Close.Code, Reason: frame.Close.Reason,
		}}})
	}
	return s.stream.Send(&packagebridgev1.WebSocketFrame{Value: &packagebridgev1.WebSocketFrame_Data{Data: append([]byte(nil), frame.Data...)}})
}

type singleConnListener struct {
	mu       sync.Mutex
	conn     net.Conn
	accepted bool
	done     chan struct{}
	close    sync.Once
}

func newSingleConnListener(connection net.Conn) *singleConnListener {
	return &singleConnListener{conn: connection, done: make(chan struct{})}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if !l.accepted && l.conn != nil {
		l.accepted = true
		connection := l.conn
		l.mu.Unlock()
		return connection, nil
	}
	l.mu.Unlock()
	<-l.done
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	if l == nil {
		return nil
	}
	var closeErr error
	l.close.Do(func() {
		close(l.done)
		l.mu.Lock()
		connection := l.conn
		l.conn = nil
		l.mu.Unlock()
		if connection != nil {
			closeErr = connection.Close()
		}
	})
	return closeErr
}

func (l *singleConnListener) Addr() net.Addr { return packageBridgeAddress{} }

type packageBridgeAddress struct{}

func (packageBridgeAddress) Network() string { return "unix" }
func (packageBridgeAddress) String() string  { return "anix-package-bridge" }
