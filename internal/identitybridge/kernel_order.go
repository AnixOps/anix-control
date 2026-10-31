package identitybridge

// The commercial link into the identity bridge: the KernelOrder contract is
// served by the kernel only in a build that has this file.

import (
	"github.com/AnixOps/anix-control/v5/internal/kernelorder"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

func init() {
	kernelOrderProvider = func(operations service.PackageHostOperations) packagebridge.KernelOrderProvider {
		server := &kernelorder.Server{DB: operations.DB, Authorizer: operations}
		return server.For
	}
}
