package model

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageRolloutModelsUseKernelOwnedTables(t *testing.T) {
	tables := map[any]string{
		PackageMigrationRun{}:     "v4_kernel_package_migration_run",
		PackageValidationResult{}: "v4_kernel_package_validation_result",
		PackageRouteGeneration{}:  "v4_kernel_package_route_generation",
		PackageBackupReference{}:  "v4_kernel_package_backup_reference",
		PackageRolloutLock{}:      "v4_kernel_package_rollout_lock",
		PackageStorage{}:          "v4_kernel_package_storage",
		KernelForwardRoute{}:      "v4_kernel_forward_route",
		KernelForwardAllocation{}: "v4_kernel_forward_allocation",
		KernelForwardNode{}:       "v4_kernel_forward_node",
		KernelForwardNodeState{}:  "v4_kernel_forward_node_state",
		KernelForwardNodeReport{}: "v4_kernel_forward_node_report",
		KernelForwardCounter{}:    "v4_kernel_forward_counter",
		KernelForwardTraffic{}:    "v4_kernel_forward_traffic",
		KernelForwardRequest{}:    "v4_kernel_forward_request",
		KernelForwardPlan{}:       "v4_kernel_forward_plan",
	}

	registered := make(map[reflect.Type]bool)
	for _, value := range KernelModels() {
		registered[reflect.TypeOf(value)] = true
	}
	for value, table := range tables {
		model, ok := value.(interface{ TableName() string })
		require.True(t, ok)
		require.Equal(t, table, model.TableName())
		require.True(t, registered[reflect.PointerTo(reflect.TypeOf(value))])
	}
}
