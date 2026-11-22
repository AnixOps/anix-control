package packagebridge

import (
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
)

// KernelIdentityProvider returns the KernelIdentity server that one calling
// host reaches. The server authorizes the host on every call.
type KernelIdentityProvider func(HostIdentity) kernelidentityv1.KernelIdentityServer
