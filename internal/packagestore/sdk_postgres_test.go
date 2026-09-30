package packagestore

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/stretchr/testify/require"
)

// The package side of a lease: the SDK connects as the package role, runs
// embedded migrations in the package schema, and cannot reach kernel tables.
func TestPostgresPackageStoreSDKRunsMigrationsInThePackageSchema(t *testing.T) {
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
	require.Equal(t, SchemaName(packageID)+".notes", pkg.Table("notes"))

	fsys := fstest.MapFS{
		"migrations/index.json": {Data: []byte(`{"format":"anixops.migrations/v1","package_id":"x","version":"4.1.0","migrations":[
			{"id":"001_notes","path":"migrations/001_notes.sql"},{"id":"002_directory","path":"migrations/002_directory.sql"}]}`)},
		"migrations/001_notes.sql":     {Data: []byte("CREATE TABLE __PKG_PREFIX__notes (id BIGSERIAL PRIMARY KEY, body TEXT NOT NULL);\nINSERT INTO __PKG_PREFIX__notes (body) VALUES ('a'), ('b');")},
		"migrations/002_directory.sql": {Data: []byte("CREATE VIEW __PKG_PREFIX__admins AS SELECT id, email FROM kapi_user_directory_v1 WHERE is_admin = 1;")},
	}
	result, err := packagestoresdk.RunEmbeddedMigrations(ctx, pkg, fsys, "migrations/index.json")
	require.NoError(t, err)
	require.Equal(t, []string{"001_notes", "002_directory"}, result.Applied)
	again, err := packagestoresdk.RunEmbeddedMigrations(ctx, pkg, fsys, "migrations/index.json")
	require.NoError(t, err)
	require.Empty(t, again.Applied)
	require.Equal(t, result.StepsDigest, again.StepsDigest)

	var tables []string
	require.NoError(t, kernel.Raw("SELECT tablename FROM pg_tables WHERE schemaname = ? ORDER BY tablename", SchemaName(packageID)).Scan(&tables).Error)
	require.Equal(t, []string{"notes", "schema_migrations"}, tables)

	bad := fstest.MapFS{
		"migrations/index.json": {Data: []byte(`{"format":"anixops.migrations/v1","migrations":[{"id":"003_escape","path":"migrations/003.sql"}]}`)},
		"migrations/003.sql":    {Data: []byte("DELETE FROM v2_user;")},
	}
	_, err = packagestoresdk.RunEmbeddedMigrations(ctx, pkg, bad, "migrations/index.json")
	requirePermissionDenied(t, err)
}

// New pool connections fetch the current lease, so a password rotated by
// another lease (a kernel restart, another generation) does not break the
// module's pool.
func TestPostgresPackageStoreSDKFollowsRotatedPasswords(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN, Leases: NewLeaseCache()}
	generation := uint64(1)
	leaser := packagestoresdk.LeaserFunc(func(ctx context.Context) (packagebridgesdk.StorageLease, error) {
		lease, err := store.Lease(ctx, Holder{PackageID: packageID, Version: "4.1.0", Generation: generation}, Grants{Storage: true})
		return packagebridgesdk.StorageLease{Driver: lease.Driver, DSN: lease.DSN, Schema: lease.Schema, LeaseGeneration: lease.LeaseGeneration}, err
	})
	pkg, err := packagestoresdk.Open(context.Background(), leaser)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkg.Close()) })
	sqlDB, err := pkg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(0)
	require.NoError(t, pkg.DB.Exec("SELECT 1").Error)

	// A new generation leases and rotates the role password.
	generation = 2
	_, err = store.Lease(context.Background(), Holder{PackageID: packageID, Version: "4.1.0", Generation: 2}, Grants{Storage: true})
	require.NoError(t, err)
	require.NoError(t, pkg.DB.Exec("SELECT 1").Error, "the next connection uses the rotated password")
}
