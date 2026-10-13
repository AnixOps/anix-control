package model

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageRolloutModelsUseKernelOwnedTables(t *testing.T) {
	tables := map[any]string{
		PackageMigrationRun{}:      "v4_kernel_package_migration_run",
		PackageValidationResult{}:  "v4_kernel_package_validation_result",
		PackageRouteGeneration{}:   "v4_kernel_package_route_generation",
		PackageBackupReference{}:   "v4_kernel_package_backup_reference",
		PackageRolloutLock{}:       "v4_kernel_package_rollout_lock",
		PackageStorage{}:           "v4_kernel_package_storage",
		KernelForwardRoute{}:       "v4_kernel_forward_route",
		KernelForwardAllocation{}:  "v4_kernel_forward_allocation",
		KernelForwardNode{}:        "v4_kernel_forward_node",
		KernelForwardNodeState{}:   "v4_kernel_forward_node_state",
		KernelForwardNodeReport{}:  "v4_kernel_forward_node_report",
		KernelForwardCounter{}:     "v4_kernel_forward_counter",
		KernelForwardTraffic{}:     "v4_kernel_forward_traffic",
		KernelForwardRequest{}:     "v4_kernel_forward_request",
		KernelForwardPlan{}:        "v4_kernel_forward_plan",
		AgentUpgradeCampaign{}:     "v4_kernel_agent_upgrade_campaign",
		AgentUpgradeNode{}:         "v4_kernel_agent_upgrade_node",
		KernelForwardDNSProvider{}: "v4_kernel_forward_dns_provider",
		KernelForwardDNSBinding{}:  "v4_kernel_forward_dns_binding",
		KernelForwardDNSNode{}:     "v4_kernel_forward_dns_node",
		NodeGroup{}:                "v5_node_group",
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

// v5_acme_cache comes from the migration 0003_acme_cache, not from the
// kernel models: they describe the 0001_baseline schema.
func TestACMECacheEntryIsCreatedByAMigration(t *testing.T) {
	require.Equal(t, "v5_acme_cache", ACMECacheEntry{}.TableName())
	for _, value := range KernelModels() {
		_, isCache := value.(*ACMECacheEntry)
		require.False(t, isCache, "v5_acme_cache is created by its migration")
	}
}
