package pluginhost

import "errors"

// ErrResponseTooLarge reports a package response above the configured
// response limit. It is always wrapped together with ErrHostUnavailable so
// existing callers keep failing closed.
var ErrResponseTooLarge = errors.New("plugin response exceeds its limit")

const (
	// defaultMaxResponseBytes is the response body limit when none is
	// configured.
	defaultMaxResponseBytes = 1 << 20
	// maxConfigurableResponseBytes matches the configuration bound.
	maxConfigurableResponseBytes = 64 << 20
)

// normalizeMaxResponseBytes returns the effective response body limit.
func normalizeMaxResponseBytes(value int64) int64 {
	if value <= 0 || value > maxConfigurableResponseBytes {
		return defaultMaxResponseBytes
	}
	return value
}
