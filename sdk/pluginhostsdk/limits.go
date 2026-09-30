package pluginhostsdk

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
)

const (
	// MaxResponseBytesEnvironment carries the kernel's configured response
	// body limit (plugins.control_host_max_response_bytes) to a package host.
	MaxResponseBytesEnvironment = "ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES"
	// MaxConfigurableResponseBytes is the largest limit a host accepts from
	// its environment; it matches the kernel configuration bound.
	MaxConfigurableResponseBytes = 64 << 20
	// ResponseEnvelopeBytes is the fixed allowance for everything in a
	// DispatchResponse except the body (status, headers, identifiers). The
	// body alone is held to the configured limit, which is the same rule the
	// kernel package bridge applies, so a legacy response the bridge accepts
	// is never rejected here for its envelope.
	ResponseEnvelopeBytes = 64 << 10
	// defaultGRPCMessageBytes is grpc-go's default receive limit.
	defaultGRPCMessageBytes = 4 << 20
)

// MaxResponseBytesFromEnvironment returns the response body limit configured
// by the kernel, or zero (the SDK default) when the variable is unset.
func MaxResponseBytesFromEnvironment() (int, error) {
	raw := strings.TrimSpace(os.Getenv(MaxResponseBytesEnvironment))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > MaxConfigurableResponseBytes {
		return 0, fmt.Errorf("%s must be between 1 and %d bytes", MaxResponseBytesEnvironment, MaxConfigurableResponseBytes)
	}
	return value, nil
}

// MessageSizeLimit returns the gRPC message size needed to carry a body of
// bodyLimit bytes plus the fixed envelope, never below grpc-go's default.
func MessageSizeLimit(bodyLimit int) int {
	if bodyLimit <= 0 {
		bodyLimit = DefaultMaxResponseBytes
	}
	if size := bodyLimit + ResponseEnvelopeBytes; size > defaultGRPCMessageBytes {
		return size
	}
	return defaultGRPCMessageBytes
}

// HostServerOptions returns the recommended gRPC server options for a package
// host: RecoveryServerOptions plus a receive limit large enough for any
// request body the kernel may be configured to forward. The host socket is
// private to the kernel, which enforces the configured request limit before a
// request reaches the host.
func HostServerOptions() []grpc.ServerOption {
	return append(RecoveryServerOptions(), grpc.MaxRecvMsgSize(MessageSizeLimit(MaxConfigurableResponseBytes)))
}
