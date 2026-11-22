package packagebridge

import (
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// KernelForwardProvider returns the ForwardControl server that one calling
// host reaches. The server authorizes the host on every call.
type KernelForwardProvider func(HostIdentity) forwardv1.ForwardControlServer
