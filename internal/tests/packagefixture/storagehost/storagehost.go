// Package storagehost is a test fixture package host. It uses only the public
// SDKs, like a real package: it leases its storage over the package bridge
// and applies its embedded migration index when the kernel asks.
package storagehost

import (
	"context"
	"embed"
	"log"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// PackageID is the fixture's package id.
const PackageID = "storage-fixture"

//go:embed migrations
var migrations embed.FS

// Run serves the fixture host for one release until ctx ends.
func Run(ctx context.Context, version string) error {
	return modulesdk.Run(ctx, modulesdk.Options{
		PackageID: PackageID, PackageVersion: version,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			storage := packagestoresdk.SharedOpener(host.Bridge)
			return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
				PackageID: PackageID, LeaseID: host.LeaseID, Bridge: host.Bridge, Logf: log.Printf,
				IndexMigration: packagestoresdk.IndexMigrator(storage, migrations, "migrations/index.json"),
			})
		},
	})
}
