package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

// kernel.forward.v1 is in the kernel's grammar (ForwardControl is served
// since F3a); other kernel.forward names are not, and no forwarding table
// can be adopted.
func TestForwardCapabilityGrammar(t *testing.T) {
	require.NoError(t, validateManifestCapabilities([]string{CapabilityForward}))
	for _, capability := range []string{"kernel.forward.v2", "kernel.forward", "kernel.forward.write.v1"} {
		require.ErrorContains(t, validateManifestCapabilities([]string{capability}), "unknown kernel capability", capability)
	}
	for _, value := range model.KernelForwardModels() {
		table := value.(interface{ TableName() string }).TableName()
		require.True(t, protectedKernelTable(table), table)
		require.True(t, protectedTables[table], "listed by name: %s", table)
		require.Error(t, validateManifestCapabilities([]string{CapabilityStorage, "kernel.storage.adopt:" + table}), table)
	}
}
