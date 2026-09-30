package packagebridge

import (
	"context"
	"errors"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Route modes a package host applies per v2 route.
const (
	RouteModeLegacy = "legacy"
	RouteModeShadow = "shadow"
	RouteModeNative = "native"
)

// ErrHostFenced reports that the calling host no longer matches the current
// installation: another version or lifecycle generation is desired, or the
// package is disabled. The host must not act on stale authority.
var ErrHostFenced = errors.New("package host is not the current installation generation")

// PackageConfig is the configuration a host reads for itself.
type PackageConfig struct {
	Revision   int64
	ConfigHash string
	// RouteModes maps route ids to RouteModeShadow or RouteModeNative; routes
	// that are absent run in legacy mode.
	RouteModes map[string]string
}

// HostOperations serves session-scoped RPCs. They carry no per-request
// capability: the caller is authorized by the session identity, which the
// kernel bound to the inherited socketpair when it started the host.
// Implementations must fence every call against the current installation.
type HostOperations interface {
	PackageConfig(ctx context.Context, host HostIdentity) (PackageConfig, error)
}

// GetPackageConfig returns the calling host's installation configuration.
func (s *Session) GetPackageConfig(ctx context.Context, _ *packagebridgev1.GetPackageConfigRequest) (*packagebridgev1.GetPackageConfigResponse, error) {
	operations, identity, err := s.hostOperationContext()
	if err != nil {
		return nil, err
	}
	config, err := operations.PackageConfig(ctx, identity)
	if err != nil {
		return nil, hostOperationStatusError(err)
	}
	modes := make(map[string]string, len(config.RouteModes))
	for route, mode := range config.RouteModes {
		modes[route] = mode
	}
	return &packagebridgev1.GetPackageConfigResponse{Revision: config.Revision, ConfigHash: config.ConfigHash, RouteModes: modes}, nil
}

func (s *Session) hostOperationContext() (HostOperations, HostIdentity, error) {
	if s == nil {
		return nil, HostIdentity{}, status.Error(codes.Unavailable, "package bridge is closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, HostIdentity{}, status.Error(codes.Unavailable, "package bridge is closed")
	}
	if s.hostOperations == nil {
		return nil, HostIdentity{}, status.Error(codes.Unimplemented, "package bridge session operations are not configured")
	}
	return s.hostOperations, s.identity, nil
}

func hostOperationStatusError(err error) error {
	switch {
	case errors.Is(err, ErrHostFenced):
		return status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return status.Error(codes.DeadlineExceeded, "package bridge session operation deadline exceeded")
	default:
		return status.Error(codes.Internal, "package bridge session operation failed")
	}
}
