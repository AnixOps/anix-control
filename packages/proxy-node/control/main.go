// Package control is the proxy-node package's Control host. Control links it
// (packages/builtin) and runs it inside its own process.
package control

import (
	"context"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// PackageID is the official package this host serves.
const PackageID = "proxy-node"

// Run serves the host for one release of the package until the kernel stops
// it: ctx ends (inside Control) or SIGTERM arrives (as a child process).
func Run(ctx context.Context, version string) error {
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: PackageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newProxyNodeService(host.Bridge, host.LeaseID)
		},
	})
}
