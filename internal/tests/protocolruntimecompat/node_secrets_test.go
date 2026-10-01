package protocolruntimecompat

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native"
	"github.com/stretchr/testify/require"
)

// The kernel's handlers of these routes mask node protocol secrets
// (service.RedactNodeProtocols) and keep them when an update sends the
// placeholder back. They have no native handler: one added later must answer
// the same masked values, and its parity case must show it.
func TestNodeSecretRoutesStayInTheKernel(t *testing.T) {
	handlers := (&native.Service{}).Handlers()
	for _, routeID := range []string{
		"protocol.admin.nodes.id.protocols.get",
		"protocol.admin.nodes.id.protocols.post",
		"protocol.admin.nodes.id.protocols.protocol_id.put",
	} {
		require.NotContains(t, handlers, routeID, "a native %s must mask node secrets as the kernel does", routeID)
	}
}
