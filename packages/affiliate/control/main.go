// Package control is the affiliate package's Control host. Control links it
// and starts it as `anix-control package-host affiliate` (packages/builtin);
// packages/builtin/cmd/package-host builds it as a standalone binary.
package control

import (
	"context"
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// PackageID is the official package this host serves.
const PackageID = "affiliate"

// Run serves the host for one release of the package until the kernel stops
// it (SIGTERM) or ctx ends. modulesdk picks the kernel-supervised child
// process or the network module runtime from the environment.
func Run(ctx context.Context, version string) error {
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: PackageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newAffiliateService(host.Bridge, host.LeaseID)
		},
	})
}
