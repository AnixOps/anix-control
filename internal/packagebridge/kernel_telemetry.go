package packagebridge

import (
	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
)

// KernelTelemetryProvider returns the KernelTelemetry server that one
// calling host reaches. The server authorizes the host on every call.
type KernelTelemetryProvider func(HostIdentity) kerneltelemetryv1.KernelTelemetryServer
