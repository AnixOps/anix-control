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
	ErrPackageFailed         = errors.New("plugin package failed")
	ErrGenerationUnavailable = errors.New("plugin host generation unavailable")
	defaultManager           struct {
		sync.RWMutex
		manager Manager
	}
)

// MigrationStep is one verified step of a package migration index.
type MigrationStep struct {
	ID     string
	Path   string
	SHA256 string
}

type ArtifactRef struct {
	PackageID        string
	Version          string
	ArtifactPath     string
	ArtifactSHA256   string
	EntrypointPath   string
	EntrypointSHA256 string
	ManifestPath     string
	ManifestSHA256   string
	// MigrationIndexSHA256 and Migrations describe the verified migration
	// index of the release; Migrations is empty when it declares no steps.
	MigrationIndexSHA256 string
	Migrations           []MigrationStep
	// Storage is set when the signed manifest declares kernel.storage.v1.
	// Only such releases run their migration index through the kernel's
	// migration ledger when the host starts.
	Storage bool
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
	// Host and TLS describe the original request address for the kernel's
	// package bridge only. They are never sent to package hosts, because hosts
	// built with the v4.0.0 SDK reject unknown metadata fields.
	Host string `json:"-"`
	TLS  bool   `json:"-"`
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
	// MaxResponseBytes is plugins.control_host_max_response_bytes. It is
	// passed to hosts as ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES and bounds the
	// kernel client's receive size. Zero selects the 1 MiB default.
	MaxResponseBytes int64
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

// Host supervision states reported by HostStats.State.
const (
	HostStateRunning    = "running"
	HostStateRestarting = "restarting"
	HostStateFailed     = "failed"
	HostStateExited     = "exited"
	HostStateStopped    = "stopped"
)

type HostStats struct {
	PackageID       string
	Version         string
	Generation      uint64
	State           string
	Starts          uint64
	UnexpectedExits uint64
	Restarts        uint64
	Failures        uint64

	HealthDetailsJSON string
	HealthCheckedAt   time.Time
}

type HostStatsProvider interface {
	Stats() []HostStats
}

func (*Supervisor) PollHealth(context.Context, time.Duration) {}

func (*Supervisor) Stats() []HostStats {
	return nil
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
var _ HostStatsProvider = (*Supervisor)(nil)
