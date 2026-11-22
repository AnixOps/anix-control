package pluginhostsdk

import (
	"context"
)

type DispatchRequest struct {
	PackageID          string
	PackageVersion     string
	RouteGeneration    uint64
	RequestID          string
	IdempotencyKey     string
	RouteID            string
	Method             string
	RequestBody        []byte
	PrincipalJSON      []byte
	Metadata           RequestMetadata
	BridgeCapability   []byte
	DeadlineUnixMillis int64
}

// RequestMetadata is kernel-provided HTTP address metadata. It is distinct
// from signed route selection and lets a package preserve the legacy v2
// request contract without consulting kernel routing state.
//
// Scheme and Host are the original request's scheme ("http" or "https",
// from the connection's TLS state) and Host header (a host[:port] the kernel
// validated).
type RequestMetadata struct {
	Path       string              `json:"path,omitempty"`
	Query      map[string][]string `json:"query,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	PathParams map[string]string   `json:"path_params,omitempty"`
	ClientIP   string              `json:"client_ip,omitempty"`
	UserAgent  string              `json:"user_agent,omitempty"`
	Scheme     string              `json:"-"`
	Host       string              `json:"-"`
}

type DispatchResponse struct {
	StatusCode   uint32
	ResponseBody []byte
	Headers      []Header
	OperationID  string
	FailureCode  string
}

type Header struct {
	Name  string
	Value string
}

type MigrationRequest struct {
	PackageID          string
	PackageVersion     string
	MigrationID        string
	Checkpoint         string
	RouteGeneration    uint64
	BridgeCapability   []byte
	DeadlineUnixMillis int64
}

type MigrationResponse struct {
	Checkpoint       string
	ValidationDigest string
	Complete         bool
	FailureCode      string
}

type HealthResponse struct {
	Healthy     bool
	LeaseID     string
	DetailsJSON string
}

type DrainResponse struct {
	Drained  bool
	InFlight uint64
}

type WebSocketOpen struct {
	PackageID          string
	PackageVersion     string
	RouteGeneration    uint64
	RouteID            string
	PrincipalJSON      []byte
	Metadata           RequestMetadata
	RequestID          string
	IdempotencyKey     string
	DeadlineUnixMillis int64
	BridgeCapability   []byte
}

type WebSocketClose struct {
	Code   uint32
	Reason string
}

// WebSocketFrame is a post-open package-host frame. A nil Close denotes a
// data frame, including an empty opaque payload.
type WebSocketFrame struct {
	Data  []byte
	Close *WebSocketClose
}

type WebSocketStream interface {
	Recv() (WebSocketFrame, error)
	Send(WebSocketFrame) error
}

// WebSocketPackage is intentionally optional so a host cannot accidentally
// claim WebSocket support merely by implementing unary package methods.
type WebSocketPackage interface {
	OpenWebSocket(context.Context, WebSocketOpen, WebSocketStream) error
}

type Package interface {
	Dispatch(context.Context, DispatchRequest) (DispatchResponse, error)
	Migrate(context.Context, MigrationRequest) (MigrationResponse, error)
	Health(context.Context) (HealthResponse, error)
	Drain(context.Context) (DrainResponse, error)
}

// ResumablePackage undoes a Drain: a drained instance serves again when the
// kernel calls Resume.
type ResumablePackage interface {
	Resume(context.Context) error
}
