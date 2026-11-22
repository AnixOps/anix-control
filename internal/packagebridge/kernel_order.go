package packagebridge

import (
	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
)

// KernelOrderProvider returns the KernelOrder server that one calling host
// reaches. The server authorizes the host on every call.
type KernelOrderProvider func(HostIdentity) kernelorderv1.KernelOrderServer
