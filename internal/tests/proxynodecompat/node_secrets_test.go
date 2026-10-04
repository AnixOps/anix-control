package proxynodecompat

import (
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"github.com/stretchr/testify/require"
)

// These routes stay in the kernel, with or without KernelNodeOps: the
// credentials display answers the stored API key and secret (D4), node
// creation also creates the node's default protocol in protocol-runtime's
// table, node update also records a group change in the subscriber change
// log and revokes a disabled node's agents, and the configuration
// validation answers the length of a configuration whose secrets the
// package only holds as handles.
func TestNodeSecretRoutesStayInTheKernel(t *testing.T) {
	handlers := (&native.Service{NodeOps: kernelnodeopsv1.NewKernelNodeOpsClient(nil)}).Handlers()
	for _, routeID := range []string{
		"proxy.admin.nodes.id.credentials.get",
		"proxy.admin.nodes.post",
		"proxy.admin.nodes.id.put",
		"proxy.admin.nodes.validate_config.post",
	} {
		require.NotContains(t, handlers, routeID)
	}
}
