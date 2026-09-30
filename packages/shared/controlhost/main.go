package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	"github.com/AnixOps/anix-control/v4/pkg/modulesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
)

var (
	packageID      = ""
	packageVersion = "4.0.0"
)

func main() {
	if err := run(); err != nil {
		log.Printf("generic control package host: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if strings.TrimSpace(packageID) == "" || strings.TrimSpace(packageVersion) == "" {
		return errors.New("package identity is required")
	}
	return modulesdk.Run(context.Background(), modulesdk.Options{
		PackageID: packageID, PackageVersion: packageVersion,
		Build: func(host modulesdk.Host) (pluginhostsdk.Package, error) {
			return newGenericService(packageID, host.Bridge, host.LeaseID)
		},
	})
}
