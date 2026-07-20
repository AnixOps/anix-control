//go:build !unix

// Package pluginhost exposes fail-closed stubs on platforms that cannot
// provide the Unix socket and descriptor isolation required by package hosts.
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
)

var (
	ErrHostUnavailable       = errors.New("plugin host unavailable")
	ErrHostNotFound          = errors.New("plugin host not found")
	ErrHostIncompatible      = errors.New("plugin host incompatible")
	ErrGenerationUnavailable = errors.New("plugin host generation unavailable")
	defaultManager           struct {
		sync.RWMutex
		manager Manager
	}
)

type ArtifactRef struct {
	PackageID        string
	Version          string
	ArtifactPath     string
	ArtifactSHA256   string
	EntrypointPath   string
	EntrypointSHA256 string
	ManifestPath     string
	ManifestSHA256   string
}

type DispatchInput struct {
	PackageID        string
	Version          string
	Generation       uint64
	RequestID        string
	IdempotencyKey   string
	RouteID          string
	Method           string
	Body             []byte
	PrincipalJSON    []byte
	Metadata         RequestMetadata
	BridgeCapability []byte
	Deadline         time.Time
}

type RequestMetadata struct {
	Path                             string              `json:"path,omitempty"`
	Query                            map[string][]string `json:"query,omitempty"`
	Headers                          map[string][]string `json:"headers,omitempty"`
	PathParams                       map[string]string   `json:"path_params,omitempty"`
	ClientIP                         string              `json:"client_ip,omitempty"`
	UserAgent                        string              `json:"user_agent,omitempty"`
	NodeID                           uint                `json:"node_id,omitempty"`
	TrustedAgentWebSocketAuth        bool                `json:"trusted_agent_websocket_auth,omitempty"`
	TrustedAgentWebSocketForwardNode bool                `json:"trusted_agent_websocket_forward_node,omitempty"`
}

type DispatchOutput struct {
	StatusCode  uint32
	Body        []byte
	Headers     []Header
	OperationID string
	FailureCode string
}

type Header struct {
	Name  string
	Value string
}

type HostHealth struct {
	Healthy     bool
	LeaseID     string
	DetailsJSON string
}

type DrainResult struct {
	Drained  bool
	InFlight uint64
}

type Manager interface {
	Start(context.Context, ArtifactRef, uint64) error
	Dispatch(context.Context, DispatchInput) (DispatchOutput, error)
	Health(context.Context, string, string, uint64) (HostHealth, error)
	Drain(context.Context, string, string, uint64, time.Time) error
	Stop(context.Context, string, string, uint64) error
	Rollback(context.Context, string, string, uint64) error
}

type MigrationManager interface {
	Migrate(context.Context, MigrationInput, MigrationCheckpointRecorder) (MigrationOutput, error)
}

type WebSocketManager interface {
	OpenWebSocket(context.Context, WebSocketInput) (*WebSocketRelay, error)
}

type ManagerConfig struct {
	RuntimeDir     string
	StartupTimeout time.Duration
	BridgeFactory  packagebridge.SessionFactory
}

type Supervisor struct{}

type MigrationInput struct {
	PackageID        string
	Version          string
	MigrationID      string
	Checkpoint       string
	Generation       uint64
	BridgeCapability []byte
	Deadline         time.Time
}

type MigrationOutput struct {
	Checkpoint       string
	ValidationDigest string
	Complete         bool
	FailureCode      string
	HealthLeaseID    string
	HealthGeneration uint64
}

type MigrationCheckpointRecorder func(context.Context, MigrationOutput) error

type WebSocketInput struct {
	PackageID        string
	Version          string
	Generation       uint64
	RouteID          string
	PrincipalJSON    []byte
	Metadata         RequestMetadata
	RequestID        string
	IdempotencyKey   string
	BridgeCapability []byte
	Deadline         time.Time
}

type WebSocketClose struct {
	Code   uint32
	Reason string
}

type WebSocketFrame struct {
	Data  []byte
	Close *WebSocketClose
}

type WebSocketRelay struct{}

func SetDefaultManager(manager Manager) {
	defaultManager.Lock()
	defer defaultManager.Unlock()
	defaultManager.manager = manager
}

func DefaultManager() Manager {
	defaultManager.RLock()
	defer defaultManager.RUnlock()
	return defaultManager.manager
}

func NewManager(ManagerConfig) (*Supervisor, error) {
	return nil, fmt.Errorf("%w: package hosts require Unix socket and descriptor isolation", ErrHostUnavailable)
}

func (*Supervisor) Start(context.Context, ArtifactRef, uint64) error {
	return ErrHostUnavailable
}

func (*Supervisor) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, ErrHostUnavailable
}

func (*Supervisor) Health(context.Context, string, string, uint64) (HostHealth, error) {
	return HostHealth{}, ErrHostUnavailable
}

func (*Supervisor) Drain(context.Context, string, string, uint64, time.Time) error {
	return ErrHostUnavailable
}

func (*Supervisor) Stop(context.Context, string, string, uint64) error {
	return ErrHostUnavailable
}

func (*Supervisor) Rollback(context.Context, string, string, uint64) error {
	return ErrHostUnavailable
}

func (*Supervisor) Migrate(context.Context, MigrationInput, MigrationCheckpointRecorder) (MigrationOutput, error) {
	return MigrationOutput{}, ErrHostUnavailable
}

func (*Supervisor) OpenWebSocket(context.Context, WebSocketInput) (*WebSocketRelay, error) {
	return nil, ErrHostUnavailable
}

func (*Supervisor) Shutdown(context.Context) error {
	return nil
}

func (*Supervisor) Close() error {
	return nil
}

func (*WebSocketRelay) Send(WebSocketFrame) error {
	return ErrHostUnavailable
}

func (*WebSocketRelay) Recv() (WebSocketFrame, error) {
	return WebSocketFrame{}, ErrHostUnavailable
}

func (*WebSocketRelay) Close(WebSocketClose) error {
	return ErrHostUnavailable
}

var _ Manager = (*Supervisor)(nil)
var _ MigrationManager = (*Supervisor)(nil)
var _ WebSocketManager = (*Supervisor)(nil)
