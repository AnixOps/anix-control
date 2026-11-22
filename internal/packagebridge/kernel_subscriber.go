package packagebridge

import (
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
)

// KernelSubscriberProvider returns the KernelSubscriber server that one
// calling host reaches. The server authorizes the host on every call.
type KernelSubscriberProvider func(HostIdentity) kernelsubscriberv1.KernelSubscriberServer
