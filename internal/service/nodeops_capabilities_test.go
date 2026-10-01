package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The KernelNodeOps family capabilities are in the kernel's grammar (the
// contract is binding since NO-1); other kernel.nodeops names are not, and
// the ledger tables cannot be adopted.
func TestNodeOpsCapabilityGrammar(t *testing.T) {
	require.NoError(t, validateManifestCapabilities(NodeOpsCapabilities()))
	for _, capability := range NodeOpsCapabilities() {
		require.NoError(t, validateManifestCapabilities([]string{capability}), capability)
	}
	for _, capability := range []string{
		"kernel.nodeops.v1", "kernel.nodeops.forward.v2", "kernel.nodeops.shell.v1", "kernel.nodeops.forward",
		"kernel.nodeops.agents.write.v1",
	} {
		require.ErrorContains(t, validateManifestCapabilities([]string{capability}), "unknown kernel capability", capability)
	}
	for _, table := range []string{"v4_kernel_node_operation", "v4_kernel_node_operation_event", "v4_kernel_node_operation_target"} {
		require.True(t, protectedKernelTable(table), table)
		require.True(t, protectedTables[table], "listed by name: %s", table)
		require.Error(t, validateManifestCapabilities([]string{CapabilityStorage, "kernel.storage.adopt:" + table}), table)
	}
}
