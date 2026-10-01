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
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/affiliate/native"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// inviteRoute is route with the invite codes and orders the invite routes
// read.
func inviteRoute(t *testing.T, method, pattern, routeID string, legacy func(*handler.InviteHandler, *gin.Context)) packagecompat.Route {
	r := route(t, method, pattern, routeID, legacy)
	r.Models = append(r.Models, &model.InviteCode{}, &model.Order{})
	return r
}

// seedInvites adds invite codes and orders to seed:
//   - alice (2) holds an unused code, a used one, an expired one and one
//     that expires in 2100, so two count against her limit; bob (3) one
//     unused code; one public code;
//   - alice's invitees: bob paid twice, carol (4) has a completed order,
//     which the kernel does not count as paid, and 5 a pending one; bob's
//     invitee 6 paid once.
func seedInvites(config bool, codeCount int) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedWith(config, ptr(settingsValue))(t, db)
		if config && codeCount != 3 {
			require.NoError(t, db.Model(&model.InviteConfig{}).Where("id = 1").Update("code_count", codeCount).Error)
		}
		require.NoError(t, db.Create(&model.Plan{ID: 1, Name: "Basic", CreatedAt: seeded, UpdatedAt: seeded}).Error)
		order := func(id, user uint, status int) model.Order {
			return model.Order{
				ID: id, UserID: user, PlanID: 1, Period: "month_price", TradeNo: fmt.Sprintf("trade-%d", id), TotalAmount: 1000,
				Status: status, CreatedAt: seeded, UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.Order{
			order(1, 3, 1), order(2, 3, 1), order(3, 4, 3), order(4, 5, 0), order(5, 6, 1), order(6, 2, 1),
		}).Error)
		used := seeded.Add(30 * time.Minute)
		expired := seeded.Add(-time.Hour)
		future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
		code := func(id uint, value string, owner *uint, status int, usedBy *uint, usedAt, expiredAt *time.Time) model.InviteCode {
			return model.InviteCode{
				ID: id, Code: value, UserID: owner, Status: status, UsedBy: usedBy, UsedAt: usedAt, ExpiredAt: expiredAt,
				CreatedAt: seeded.Add(time.Duration(id) * time.Minute), UpdatedAt: seeded,
			}
		}
		require.NoError(t, db.Create(&[]model.InviteCode{
			code(1, "a1b2c3d4", ptr(uint(2)), 0, nil, nil, nil),
			code(2, "used0001", ptr(uint(2)), 1, ptr(uint(3)), &used, nil),
			code(3, "expired1", ptr(uint(2)), 0, nil, nil, &expired),
			code(4, "public01", nil, 0, nil, nil, nil),
			code(5, "bobcode1", ptr(uint(3)), 0, nil, nil, nil),
			code(6, "future01", ptr(uint(2)), 0, nil, nil, &future),
		}).Error)
		require.NoError(t, packagestore.EnsureKernelAPIViews(db))
		if db.Name() == "postgres" {
			for _, table := range []string{"v2_plan", "v2_order", "v2_invite_code"} {
				require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)", table).Error)
			}
		}
	}
}

var inviteCodePattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// expiryTime is a code's expiry: masked when the handler set it from its
// clock (some days from now), else as stored.
func expiryTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	until := time.Until(*value)
	days := math.Round(until.Hours() / 24)
	if days >= 1 && math.Abs(until.Hours()-days*24) < 0.2 && !value.Equal(value.Truncate(time.Second)) {
		return fmt.Sprintf("<now+%.0fd>", days)
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// inviteState is everything generating a code can change: the codes (a
// generated code's value checked and masked) and the invite configuration.
func inviteState(t testing.TB, db *gorm.DB) any {
	var codes []model.InviteCode
	require.NoError(t, db.Order("id").Find(&codes).Error)
	rows := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		value := code.Code
		if code.ID > 6 {
			require.Regexp(t, inviteCodePattern, value)
			value = "<generated>"
		}
		rows = append(rows, map[string]any{
			"id": code.ID, "code": value, "user_id": code.UserID, "status": code.Status, "used_by": code.UsedBy,
			"used_at": clockTimePtr(code.UsedAt), "expired_at": expiryTime(code.ExpiredAt),
			"created_at": clockTime(code.CreatedAt), "updated_at": clockTime(code.UpdatedAt),
		})
	}
	all, ok := state(t, db).(map[string]any)
	require.True(t, ok)
	return map[string]any{"codes": rows, "configs": all["configs"]}
}

func TestInviteInfoParity(t *testing.T) {
	read(t, inviteRoute(t, "GET", "/api/v2/user/invite", native.InviteInfoRouteID, (*handler.InviteHandler).GetInviteInfo),
		[]packagecompat.Case{
			{Name: "codes, balance and statistics", Path: "/api/v2/user/invite", Principal: alice, Seed: seedInvites(true, 3)},
			{Name: "an inviter with one paying invitee", Path: "/api/v2/user/invite", Principal: bob, Seed: seedInvites(true, 3)},
			{Name: "no codes and no invitees", Path: "/api/v2/user/invite", Principal: carol, Seed: seedInvites(true, 3)},
			{Name: "the administrator", Path: "/api/v2/user/invite", Principal: admin, Seed: seedInvites(true, 3)},
			{Name: "no orders or codes yet", Path: "/api/v2/user/invite", Principal: alice},
			{Name: "an unknown user", Path: "/api/v2/user/invite", Principal: pluginhostsdk.Principal{ActorID: 99}, Seed: seedInvites(true, 3)},
			{Name: "user zero", Path: "/api/v2/user/invite", Principal: pluginhostsdk.Principal{}, Seed: seedInvites(true, 3)},
			{Name: "no data", Path: "/api/v2/user/invite", Principal: alice, Seed: empty},
		})
}

func TestGenerateInviteCodeParity(t *testing.T) {
	generate := inviteRoute(t, "POST", "/api/v2/user/invite/generate", native.InviteGenerateRouteID, (*handler.InviteHandler).GenerateCode)
	generated := []string{"data.code", "data.created_at", "data.updated_at", "data.expired_at"}
	path := "/api/v2/user/invite/generate"
	for _, c := range []packagecompat.Case{
		{Name: "a code that expires as configured", Path: path, Principal: alice, Mask: generated, Seed: seedInvites(true, 3)},
		{Name: "the limit counts unused unexpired codes", Path: path, Principal: alice, Seed: seedInvites(true, 3),
			Warmup: [][]byte{nil}},
		{Name: "a code under a higher limit", Path: path, Principal: alice, Mask: generated, Seed: seedInvites(true, 4),
			Warmup: [][]byte{nil}},
		{Name: "code count zero is the default limit", Path: path, Principal: alice, Mask: generated, Seed: seedInvites(true, 0),
			Warmup: [][]byte{nil, nil}},
		{Name: "the default limit", Path: path, Principal: alice, Seed: seedInvites(true, 0), Warmup: [][]byte{nil, nil, nil}},
		{Name: "no configuration creates the default", Path: path, Principal: alice, Mask: generated, Seed: seedInvites(false, 0)},
		{Name: "other users' codes and public codes are not counted", Path: path, Principal: carol, Mask: generated,
			Seed: seedInvites(true, 1)},
		{Name: "a user at the limit of one", Path: path, Principal: bob, Seed: seedInvites(true, 1), Warmup: [][]byte{nil}},
		{Name: "a user without codes", Path: path, Principal: carol, Mask: generated, Seed: seedInvites(true, 3)},
		{Name: "an unknown user", Path: path, Principal: pluginhostsdk.Principal{ActorID: 99}, Mask: generated, Seed: seedInvites(true, 3)},
		{Name: "a body is ignored", Path: path, Principal: carol, Mask: generated, Seed: seedInvites(true, 3), Body: []byte(`{"count":9}`)},
	} {
		c.Snapshot = inviteState
		packagecompat.RunWrite(t, generate, c)
	}
}

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
