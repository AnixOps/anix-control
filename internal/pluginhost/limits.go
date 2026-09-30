package pluginhost

import (
	"errors"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// ErrResponseTooLarge reports a package response above the configured
// response limit (codes.ResourceExhausted from the host, or a host message
// the kernel client refused to receive). It is always wrapped together with
// ErrHostUnavailable so existing callers keep failing closed.
var ErrResponseTooLarge = errors.New("plugin response exceeds its limit")

// normalizeMaxResponseBytes returns the effective response body limit.
func normalizeMaxResponseBytes(value int64) int64 {
	if value <= 0 || value > pluginhostsdk.MaxConfigurableResponseBytes {
		return pluginhostsdk.DefaultMaxResponseBytes
	}
	return value
}

// hostReceiveMessageLimit sizes the kernel client's gRPC receive window for
// a response body limit plus the SDK's fixed envelope allowance.
func hostReceiveMessageLimit(maxResponseBytes int64) int {
	return pluginhostsdk.MessageSizeLimit(int(normalizeMaxResponseBytes(maxResponseBytes)))
}
