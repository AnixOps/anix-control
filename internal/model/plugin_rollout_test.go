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
