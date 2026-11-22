package packagebridge

import (
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
)

// KernelNodeOpsProvider returns the KernelNodeOps server that one calling
// host reaches. The server authorizes the host on every call.
type KernelNodeOpsProvider func(HostIdentity) kernelnodeopsv1.KernelNodeOpsServer
