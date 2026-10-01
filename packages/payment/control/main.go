package main

import (
	"context"
	"log"
	"os"
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

var packageVersion = "4.0.0"

func main() {
	err := modulesdk.Run(context.Background(), modulesdk.Options{
		PackageID: "payment", PackageVersion: packageVersion,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newPaymentService(host.Bridge, host.LeaseID)
		},
	})
	if err != nil {
		log.Printf("payment control host: %v", err)
		os.Exit(1)
	}
}
