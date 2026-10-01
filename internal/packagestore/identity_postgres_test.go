package packagestore

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/v4/internal/model"
	identityplatform "github.com/AnixOps/anix-control/v4/packages/identity-platform"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
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

// identity-platform's user directory joins its own account table with the
// subscriber and plan name views it is granted, in one read-only
// repeatable-read transaction, as the package's least-privilege role: the
// role can run the search and the counts, and they answer identity's account
// fields over Control's projection and the plan's id and name.
func TestPostgresIdentityDirectoryJoinsAccountsWithSubscriberViews(t *testing.T) {
	kernel, kernelDSN := openPostgresKernel(t)
	packageID := "pkgtest-" + randomSuffix(t)
	t.Cleanup(func() { dropPackageStorage(t, kernel, packageID) })
	suffix := randomSuffix(t)
	require.NoError(t, kernel.AutoMigrate(&model.Plan{}))
	require.NoError(t, EnsureKernelAPIViews(kernel))
	plan := &model.Plan{Name: "Directory " + suffix}
	require.NoError(t, kernel.Create(plan).Error)
	t.Cleanup(func() { kernel.Delete(&model.Plan{}, plan.ID) })
	future := time.Now().Add(240 * time.Hour).Unix()
	member := &model.User{Email: "dir-member-" + suffix + "@example.test", Password: "x", Token: "dir-t1-" + suffix, UUID: "dir-u1-" + suffix,
		TransferEnable: 1000, U: 10, D: 20, ExpiredAt: &future, PlanID: &plan.ID}
	unlinked := &model.User{Email: "dir-unlinked-" + suffix + "@example.test", Password: "x", Token: "dir-t2-" + suffix, UUID: "dir-u2-" + suffix, Banned: 1}
	require.NoError(t, kernel.Create(member).Error)
	require.NoError(t, kernel.Create(unlinked).Error)
	t.Cleanup(func() { kernel.Delete(&model.User{}, []uint{member.ID, unlinked.ID}) })

	store := Store{DB: kernel, Driver: "postgres", DSN: kernelDSN}
	leaser := packagestoresdk.LeaserFunc(func(ctx context.Context) (packagebridgesdk.StorageLease, error) {
		lease, err := store.Lease(ctx, Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
			Grants{Storage: true, Views: []string{"kapi_user_directory_v1", "kapi_subscriber_entitlement_v1", "kapi_plan_name_v1"}})
		return packagebridgesdk.StorageLease{
			Driver: lease.Driver, DSN: lease.DSN, Schema: lease.Schema, TablePrefix: lease.TablePrefix,
			LeaseGeneration: lease.LeaseGeneration, AdoptedTables: lease.AdoptedTables, Views: lease.Views,
		}, err
	})
	ctx := context.Background()
	pkg, err := packagestoresdk.Open(ctx, leaser)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkg.Close()) })
	_, err = packagestoresdk.RunEmbeddedMigrations(ctx, pkg, identityplatform.Migrations, identityplatform.MigrationIndex)
	require.NoError(t, err)
	renamed := "dir-identity-" + suffix + "@example.test"
	require.NoError(t, pkg.DB.Exec("INSERT INTO "+pkg.Table("account")+
		" (user_id, account_uuid, email, password_hash, password_algo, password_salt, is_admin, is_staff, banned, token_version, version, invite_user_id, created_at, updated_at)"+
		" VALUES (?, ?, ?, '', 'bcrypt', '', 0, 1, 1, 1, 1, 0, 0, 0)", member.ID, "acct-"+suffix, renamed).Error)

	directory := native.UserDirectory{DB: pkg.DB, Accounts: pkg.Table("account")}
	total, users, err := directory.Search(ctx, native.UserQuery{Email: suffix, Now: time.Now(), Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)
	byID := map[uint64]native.DirectoryUser{users[0].ID: users[0], users[1].ID: users[1]}
	require.Equal(t, renamed, byID[uint64(member.ID)].Email, "identity's account")
	require.Equal(t, 1, byID[uint64(member.ID)].Banned)
	require.Equal(t, 1, byID[uint64(member.ID)].IsStaff)
	require.EqualValues(t, 1000, byID[uint64(member.ID)].TransferEnable, "Control's entitlements")
	require.Equal(t, &native.DirectoryPlan{ID: uint64(plan.ID), Name: plan.Name}, byID[uint64(member.ID)].Plan)
	require.Nil(t, byID[uint64(unlinked.ID)].Plan)
	require.Equal(t, unlinked.Email, byID[uint64(unlinked.ID)].Email, "Control's projection without an account")

	total, _, err = directory.Search(ctx, native.UserQuery{Email: "dir-member-" + suffix, Now: time.Now(), Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total, "the projected e-mail is not identity's")
	total, users, err = directory.Search(ctx, native.UserQuery{Email: suffix, Status: native.UserStatusBanned, Now: time.Now(), Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)

	counts, err := directory.Count(ctx, time.Now(), time.Now().Truncate(24*time.Hour))
	require.NoError(t, err)
	require.GreaterOrEqual(t, counts.Banned, int64(2))
	require.GreaterOrEqual(t, counts.Total, counts.Banned)

	var tokens []string
	requirePermissionDenied(t, pkg.DB.Raw("SELECT token FROM v2_user").Scan(&tokens).Error)
}
