package packagebridge

import (
	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
)

// KernelSettingsProvider returns the KernelSettings server that one calling
// host reaches. The server authorizes the host on every call.
type KernelSettingsProvider func(HostIdentity) kernelsettingsv1.KernelSettingsServer
