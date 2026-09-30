// Command storagehost is a test fixture package host. It uses only the public
// SDKs, like a real package: it leases its storage over the package bridge
// and applies its embedded migration index when the kernel asks.
package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

const packageID = "storage-fixture"

var packageVersion = "4.1.0"

//go:embed migrations
var migrations embed.FS

func main() {
	err := modulesdk.Run(context.Background(), modulesdk.Options{
		PackageID: packageID, PackageVersion: packageVersion,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			storage := packagestoresdk.SharedOpener(host.Bridge)
			return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
				PackageID: packageID, LeaseID: host.LeaseID, Bridge: host.Bridge, Logf: log.Printf,
				IndexMigration: packagestoresdk.IndexMigrator(storage, migrations, "migrations/index.json"),
			})
		},
	})
	if err != nil {
		log.Printf("storage fixture host: %v", err)
		os.Exit(1)
	}
}
