//go:build unix

package v2

import "github.com/AnixOps/anix-control/v4/internal/pluginhost"

// The kernel's package host supervisor serves the gateway's fail-closed
// legacy dispatch; without it every request that cannot be sealed would be
// refused instead.
var _ LegacyDispatcher = (*pluginhost.Supervisor)(nil)
