// Command package-host is one official package host as a standalone binary,
// for the network module image (Dockerfile.module) and the full package
// format of packages/shared/build_package.py (without --builtin), whose
// bin/control-host-<os>-<arch> it is. Link the package with
// -X main.packageID=<id> -X main.packageVersion=<version>. Control itself
// runs the same hosts as `anix-control package-host <id>`.
package main

import (
	"context"
	"log"
	"os"

	"github.com/AnixOps/anix-control/v5/packages/builtin"
)

var (
	packageID      = ""
	packageVersion = ""
)

func main() {
	if err := builtin.Run(context.Background(), packageID, packageVersion); err != nil {
		log.Printf("%s control host: %v", packageID, err)
		os.Exit(1)
	}
}
