package main

import (
	"context"
	"log"
	"os"
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	"github.com/AnixOps/anix-control/v4/pkg/modulesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
)

var packageVersion = "4.0.0"

func main() {
	err := modulesdk.Run(context.Background(), modulesdk.Options{
		PackageID: "identity-platform", PackageVersion: packageVersion,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newIdentityService(host.Bridge, host.LeaseID)
		},
	})
	if err != nil {
		log.Printf("identity-platform control host: %v", err)
		os.Exit(1)
	}
}
