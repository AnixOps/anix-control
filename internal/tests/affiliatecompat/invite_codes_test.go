package affiliatecompat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/affiliate/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The kernel and the package derive the same advisory lock for a user.
func TestInviteCodeLockKeysAreTheKernels(t *testing.T) {
	for _, userID := range []uint{0, 1, 2, 1<<31 - 1, 1 << 31, 1<<32 + 5, math.MaxUint32} {
		kernelClass, kernelKey := service.InviteCodeLockKeys(userID)
		class, key := native.InviteCodeLockKeys(userID)
		require.Equal(t, kernelClass, class)
		require.Equal(t, kernelKey, key, "user %d", userID)
		require.GreaterOrEqual(t, key, int32(0))
	}
}

// Concurrent generations of one user, served by the kernel and the package
// at once on one database, never pass the limit: both count under the
// user's invite code lock on PostgreSQL, and SQLite runs one writer at a
// time.
func TestConcurrentGenerationsStayWithinTheLimit(t *testing.T) {
	for name, open := range sharedDatabases(t) {
		t.Run(name, func(t *testing.T) {
			kernelDB, packageDB := open(t)
			require.NoError(t, kernelDB.AutoMigrate(&model.User{}, &model.InviteConfig{}, &model.InviteCode{}))
			require.NoError(t, kernelDB.Create(&model.InviteConfig{ID: 1, Enabled: true, CodeCount: 3}).Error)
			kernel := service.NewInviteService(kernelDB)
			module := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return packageDB.WithContext(ctx), nil }}
			const rounds, perSide = 4, 6
			for round := 0; round < rounds; round++ {
				userID := uint(100 + round)
				require.NoError(t, kernelDB.Create(&model.User{
					ID: userID, Email: fmt.Sprintf("u%d@example.test", userID), Token: fmt.Sprintf("t%d", userID), UUID: fmt.Sprintf("u%d", userID),
				}).Error)
				var wg sync.WaitGroup
				start := make(chan struct{})
				created := make(chan string, 2*perSide)
				failures := make(chan string, 2*perSide)
				for i := 0; i < perSide; i++ {
					wg.Add(2)
					go func() {
						defer wg.Done()
						<-start
						code, err := kernel.GenerateUserInviteCode(userID)
						switch {
						case err == nil:
							created <- code.Code
						case !errors.Is(err, service.ErrInviteCodeLimit):
							failures <- "kernel: " + err.Error()
						}
					}()
					go func() {
						defer wg.Done()
						<-start
						response, err := module.GenerateInviteCode(context.Background(), pluginhostsdk.NativeRequest{
							RouteID: native.InviteGenerateRouteID, Method: "POST", Principal: pluginhostsdk.Principal{ActorID: userID},
						})
						if err != nil {
							failures <- "package: " + err.Error()
							return
						}
						var answer struct {
							Data  struct{ Code string } `json:"data"`
							Error string                `json:"error"`
						}
						if err := json.Unmarshal(response.Body, &answer); err != nil {
							failures <- "package: " + string(response.Body)
							return
						}
						switch {
						case response.StatusCode == 200:
							created <- answer.Data.Code
						case answer.Error != service.InviteCodeLimitMessage:
							failures <- "package: " + string(response.Body)
						}
					}()
				}
				close(start)
				wg.Wait()
				close(created)
				close(failures)
				for failure := range failures {
					t.Errorf("round %d: %s", round, failure)
				}
				require.Len(t, created, 3, "round %d", round)
				var unused int64
				require.NoError(t, kernelDB.Model(&model.InviteCode{}).Where("user_id = ? AND status = 0", userID).Count(&unused).Error)
				require.EqualValues(t, 3, unused, "round %d", round)
			}
		})
	}
}

// sharedDatabases opens, per backend, one database through two separate
// connection pools: the kernel's and the package's.
func sharedDatabases(t *testing.T) map[string]func(*testing.T) (*gorm.DB, *gorm.DB) {
	quiet := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	backends := map[string]func(*testing.T) (*gorm.DB, *gorm.DB){
		"sqlite": func(t *testing.T) (*gorm.DB, *gorm.DB) {
			path := filepath.Join(t.TempDir(), "shared.db") + "?_pragma=busy_timeout(5000)"
			kernelDB, err := gorm.Open(sqlite.Open(path), quiet)
			require.NoError(t, err)
			packageDB, err := gorm.Open(sqlite.Open(path), quiet)
			require.NoError(t, err)
			closeOnCleanup(t, kernelDB, packageDB)
			return kernelDB, packageDB
		},
	}
	base := strings.TrimSpace(os.Getenv(packagecompat.PostgresDSNEnvironment))
	if base == "" {
		return backends
	}
	backends["postgres"] = func(t *testing.T) (*gorm.DB, *gorm.DB) {
		admin, err := gorm.Open(postgres.Open(base), quiet)
		require.NoError(t, err)
		closeOnCleanup(t, admin)
		var databaseName string
		require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
		if !strings.Contains(strings.ToLower(databaseName), "test") {
			t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
		}
		suffix := make([]byte, 4)
		_, err = rand.Read(suffix)
		require.NoError(t, err)
		schema := "affiliatecompat_lock_" + hex.EncodeToString(suffix)
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
		kernelDB, err := gorm.Open(postgres.Open(base+" search_path="+schema), quiet)
		require.NoError(t, err)
		packageDB, err := gorm.Open(postgres.Open(base+" search_path="+schema), quiet)
		require.NoError(t, err)
		closeOnCleanup(t, kernelDB, packageDB)
		return kernelDB, packageDB
	}
	return backends
}

func closeOnCleanup(t *testing.T, databases ...*gorm.DB) {
	t.Cleanup(func() {
		for _, db := range databases {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
}

// The package's own PostgreSQL role, as a storage lease provisions it,
// takes the user's invite code lock with no grant beyond the lease, and the
// lock the kernel holds in a transaction keeps it out until that ends.
func TestPackageRoleWaitsForTheKernelsInviteCodeLock(t *testing.T) {
	base := strings.TrimSpace(os.Getenv(packagecompat.PostgresDSNEnvironment))
	if base == "" {
		t.Skip(packagecompat.PostgresDSNEnvironment + " is not set")
	}
	quiet := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	kernelDB, _ := sharedDatabases(t)["postgres"](t)
	require.NoError(t, kernelDB.AutoMigrate(&model.PackageStorage{}, &model.InviteCode{}))
	suffix := make([]byte, 4)
	_, err := rand.Read(suffix)
	require.NoError(t, err)
	packageID := "lockcheck-" + hex.EncodeToString(suffix)
	role, schema := packagestore.RoleName(packageID), packagestore.SchemaName(packageID)
	t.Cleanup(func() {
		_ = kernelDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		_ = kernelDB.Exec(`DROP OWNED BY "` + role + `"`).Error
		_ = kernelDB.Exec(`DROP ROLE IF EXISTS "` + role + `"`).Error
	})
	lease, err := packagestore.Store{DB: kernelDB, Driver: "postgres", DSN: base}.Lease(context.Background(),
		packagestore.Holder{PackageID: packageID, Version: "4.1.0", Generation: 1},
		packagestore.Grants{Storage: true, AdoptTables: []string{"v2_invite_code"}})
	require.NoError(t, err)
	packageDB, err := gorm.Open(postgres.Open(lease.DSN), quiet)
	require.NoError(t, err)
	closeOnCleanup(t, packageDB)
	var user string
	require.NoError(t, packageDB.Raw("SELECT current_user").Scan(&user).Error)
	require.Equal(t, role, user)

	tryLock := func() bool {
		var taken bool
		require.NoError(t, packageDB.Transaction(func(tx *gorm.DB) error {
			class, key := native.InviteCodeLockKeys(7)
			return tx.Raw("SELECT pg_try_advisory_xact_lock(CAST(? AS integer), CAST(? AS integer))", class, key).Scan(&taken).Error
		}))
		return taken
	}
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- kernelDB.Transaction(func(tx *gorm.DB) error {
			if err := service.LockUserInviteCodes(tx, 7); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	require.False(t, tryLock(), "the package role takes the lock while the kernel holds it")
	close(release)
	require.NoError(t, <-done)
	require.True(t, tryLock(), "the package role cannot take the lock")
	require.NoError(t, packageDB.Transaction(func(tx *gorm.DB) error { return native.LockUserInviteCodes(tx, 7) }))
}
