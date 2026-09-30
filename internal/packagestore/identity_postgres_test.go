package packagestore

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	identityplatform "github.com/AnixOps/anix-control/v4/packages/identity-platform"
	"github.com/stretchr/testify/require"
)

// identity-platform's own migrations apply in a real package schema, as the
// module runs them: SQLite tests with a table prefix cannot catch
// PostgreSQL-only mistakes such as a schema-qualified index name.
func TestPostgresIdentityPlatformMigrationsApplyInThePackageSchema(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	leaser := packagestoresdk.LeaserFunc(func(ctx context.Context) (packagebridgesdk.StorageLease, error) {
		lease, err := store.Lease(ctx, Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
			Grants{Storage: true, Views: []string{"kapi_user_directory_v1"}})
		return packagebridgesdk.StorageLease{
			Driver: lease.Driver, DSN: lease.DSN, Schema: lease.Schema, TablePrefix: lease.TablePrefix,
			LeaseGeneration: lease.LeaseGeneration, AdoptedTables: lease.AdoptedTables, Views: lease.Views,
		}, err
	})
	ctx := context.Background()
	pkg, err := packagestoresdk.Open(ctx, leaser)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkg.Close()) })

	result, err := packagestoresdk.RunEmbeddedMigrations(ctx, pkg, identityplatform.Migrations, identityplatform.MigrationIndex)
	require.NoError(t, err)
	require.NotEmpty(t, result.Applied)
	again, err := packagestoresdk.RunEmbeddedMigrations(ctx, pkg, identityplatform.Migrations, identityplatform.MigrationIndex)
	require.NoError(t, err)
	require.Empty(t, again.Applied)

	var index int64
	require.NoError(t, kernel.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname = ? AND indexname = 'account_version'", SchemaName(packageID)).Scan(&index).Error)
	require.EqualValues(t, 1, index)
}
