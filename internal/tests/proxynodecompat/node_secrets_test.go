package proxynodecompat

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native"
	"github.com/stretchr/testify/require"
)

// The kernel's handlers of these routes mask node protocol and raw
// configuration secrets and registration keys (service.RedactNode,
// service.MaskAuthorizedKeys), and the credentials read is audited. They
// have no native handler: one added later must answer the same masked
// values, and its parity case must show it.
func TestNodeSecretRoutesStayInTheKernel(t *testing.T) {
	handlers := (&native.Service{}).Handlers()
	for _, routeID := range []string{
		"proxy.admin.nodes.get",
		"proxy.admin.nodes.id.get",
		"proxy.admin.nodes.id.put",
		"proxy.admin.nodes.id.raw_config.get",
		"proxy.admin.nodes.id.raw_config.put",
		"proxy.admin.nodes.id.credentials.get",
		"proxy.admin.auth_keys.get",
		"proxy.admin.auth_keys.post",
	} {
		require.NotContains(t, handlers, routeID, "a native %s must mask node secrets as the kernel does", routeID)
	}
}
