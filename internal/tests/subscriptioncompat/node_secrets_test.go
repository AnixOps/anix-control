package subscriptioncompat

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/packages/subscription/native"
	"github.com/stretchr/testify/require"
)

// The kernel's handlers of these routes mask node protocol secrets
// (service.RedactNodeProtocols). They have no native handler: one added
// later must answer the same masked values, and its parity case must show
// it.
func TestNodeSecretRoutesStayInTheKernel(t *testing.T) {
	handlers := (&native.Service{}).Handlers()
	for _, routeID := range []string{
		"subscription.admin.subscription.groups.id.protocols.get",
		"subscription.admin.subscription.protocols.available.get",
	} {
		require.NotContains(t, handlers, routeID, "a native %s must mask node secrets as the kernel does", routeID)
	}
}
