package identityimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/server"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db
}

// flakyIdentity fails the import call numbered failOn (1-based) once.
type flakyIdentity struct {
	*server.Server
	calls  int
	failOn int
}

func (f *flakyIdentity) ImportAccounts(stream grpc.ClientStreamingServer[identityv1.ImportAccountsRequest, identityv1.ImportAccountsResponse]) error {
	f.calls++
	if f.calls == f.failOn {
		return status.Error(codes.Unavailable, "identity restarted")
	}
	return f.Server.ImportAccounts(stream)
}

type fixture struct {
	kernel   *gorm.DB
	store    *account.Store
	identity *flakyIdentity
	importer *Importer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	kernel := openSQLite(t)
	require.NoError(t, kernel.AutoMigrate(&model.User{}, &model.UserMFA{}, &model.IdentityAccountLink{}, &model.IdentityAuthority{}))

	identityDB := openSQLite(t)
	schema, err := os.ReadFile("../../packages/identity-platform/migrations/003_accounts.sql")
	require.NoError(t, err)
	require.NoError(t, identityDB.Exec(strings.ReplaceAll(string(schema), "__PKG_PREFIX__", "t_")).Error)
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	store := &account.Store{DB: identityDB, Secrets: box, Tables: account.Tables{
		Account: "t_account", MFA: "t_mfa", ImportRun: "t_import_run",
	}}
	identity := &flakyIdentity{Server: &server.Server{Accounts: store}}

	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, identity)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///identity", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return &fixture{kernel: kernel, store: store, identity: identity, importer: &Importer{
		DB: kernel, BatchSize: 2, Connect: func() (grpc.ClientConnInterface, error) { return conn, nil },
	}}
}

func (f *fixture) seed(t *testing.T) {
	t.Helper()
	algo := "md5salt"
	salt := "abc"
	inviter := uint(1)
	users := []model.User{
		{ID: 1, Email: "admin@example.test", Password: "$2a$10$admin", UUID: "u1", Token: "t1", IsAdmin: 1},
		{ID: 2, Email: "member@example.test", Password: "legacy-md5", PasswordAlgo: &algo, PasswordSalt: &salt, UUID: "u2", Token: "t2", InviteUserID: &inviter},
		{ID: 3, Email: "banned@example.test", Password: "$2a$10$banned", UUID: "u3", Token: "t3", Banned: 1},
	}
	require.NoError(t, f.kernel.Create(&users).Error)
	require.NoError(t, f.kernel.Create(&model.UserMFA{UserID: 2, Enabled: true, TOTPSecret: "JBSWY3DPEHPK3PXP", BackupCodes: `["AAAA-BBBB","CCCC-DDDD"]`}).Error)
}

func digest(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func TestFullImportCopiesAccountsAndMFA(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	ctx := context.Background()

	checkpoint, err := f.importer.Run(ctx, false)
	require.NoError(t, err)
	require.NotZero(t, checkpoint.CompletedAt)
	require.Equal(t, uint64(3), checkpoint.Accounts)

	accounts, err := f.store.Get(ctx, []uint64{1, 2, 3})
	require.NoError(t, err)
	require.Len(t, accounts, 3)
	require.True(t, accounts[0].IsAdmin)
	require.Equal(t, "md5salt", accounts[1].PasswordAlgo, "legacy password algorithms travel with the hash")
	require.Equal(t, uint64(1), accounts[1].InviteUserID)
	require.True(t, accounts[1].MFAEnabled)
	require.True(t, accounts[2].Banned)
	require.Equal(t, AccountUUID(2), accounts[1].AccountUUID)

	mfa, keyed, err := f.store.MFA(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, "JBSWY3DPEHPK3PXP", mfa.TOTPSecret)
	require.True(t, f.store.BackupCodeMatches(2, digest("CCCC-DDDD"), keyed))

	var links []model.IdentityAccountLink
	require.NoError(t, f.kernel.Order("user_id").Find(&links).Error)
	require.Len(t, links, 3)
	require.Equal(t, AccountUUID(3), links[2].AccountUUID)

	state, stored, err := Status(ctx, f.kernel)
	require.NoError(t, err)
	require.Equal(t, model.IdentityAuthorityImporting, state)
	require.Equal(t, checkpoint.ImportID, stored.ImportID)
}

func TestAnInterruptedImportResumesFromItsCheckpoint(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	f.identity.failOn = 2
	ctx := context.Background()

	checkpoint, err := f.importer.Run(ctx, false)
	require.Error(t, err)
	require.Equal(t, uint(2), checkpoint.UserCursor, "the first batch was committed")
	require.Contains(t, checkpoint.LastError, "identity restarted")

	resumed, err := f.importer.Run(ctx, false)
	require.NoError(t, err)
	require.Equal(t, checkpoint.ImportID, resumed.ImportID, "the same import continues")
	require.Equal(t, uint64(3), resumed.Accounts)
	require.Empty(t, resumed.LastError)
}

func TestDeltaImportSendsOnlyChanges(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	ctx := context.Background()
	start := time.Now()
	f.importer.Now = func() time.Time { return start }
	_, err := f.importer.Run(ctx, true)
	require.ErrorIs(t, err, ErrNoFullImport)
	full, err := f.importer.Run(ctx, false)
	require.NoError(t, err)

	later := start.Add(time.Minute)
	f.importer.Now = func() time.Time { return later }
	require.NoError(t, f.kernel.Model(&model.User{}).Where("id = ?", 3).
		Updates(map[string]any{"banned": 0, "updated_at": later}).Error)
	require.NoError(t, f.kernel.Model(&model.User{}).Where("id IN ?", []uint{1, 2}).Update("updated_at", start.Add(-time.Hour)).Error)

	delta, err := f.importer.Run(ctx, true)
	require.NoError(t, err)
	require.NotEqual(t, full.ImportID, delta.ImportID)
	require.Equal(t, uint64(1), delta.Accounts)
	accounts, err := f.store.Get(ctx, []uint64{3})
	require.NoError(t, err)
	require.False(t, accounts[0].Banned)
	require.Equal(t, uint64(2), accounts[0].Version)
}

func TestImportsStopOnceIdentityIsAuthoritative(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.kernel.Create(&model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityIdentity, UpdatedAt: time.Now()}).Error)
	_, err := f.importer.Run(context.Background(), false)
	require.True(t, errors.Is(err, ErrNotImportable), err)
	_, err = (&Importer{}).Run(context.Background(), false)
	require.ErrorIs(t, err, ErrNotInitialized)
}

func TestRunnerStartsOneImportAtATime(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	release := make(chan struct{})
	connect := f.importer.Connect
	f.importer.Connect = func() (grpc.ClientConnInterface, error) {
		<-release
		return connect()
	}
	runner := NewRunner(f.importer, t.Logf)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { runner.Serve(ctx); close(served) }()

	require.Eventually(t, func() bool { return runner.Start(false) == nil }, 5*time.Second, 5*time.Millisecond)
	require.ErrorIs(t, runner.Start(true), ErrRunning, "a second import waits for the first")
	close(release)
	require.Eventually(t, func() bool {
		_, checkpoint, err := Status(context.Background(), f.kernel)
		return err == nil && checkpoint.CompletedAt != 0
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-served
	require.ErrorIs(t, (*Runner)(nil).Start(false), ErrNotInitialized)
}
