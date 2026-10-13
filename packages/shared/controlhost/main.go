// Package controlhost is the generic Control host of the official packages
// without a host of their own (nat-egress, nftables-forward): every route
// passes through the package bridge to its legacy handler. Control links it
// and starts it as `anix-control package-host <id>` (packages/builtin).
package controlhost

import (
	"context"
	"errors"
	"strings"
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// Run serves the generic host for one release of a package until the kernel
// stops it (SIGTERM) or ctx ends.
func Run(ctx context.Context, packageID, version string) error {
	if strings.TrimSpace(packageID) == "" || strings.TrimSpace(version) == "" {
		return errors.New("package identity is required")
	}
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: packageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newGenericService(packageID, host.Bridge, host.LeaseID)
		},
	})
}
