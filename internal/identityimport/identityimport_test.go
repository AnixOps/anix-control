package identityimport

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/server"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
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
	store    *account.Store
	identity *flakyIdentity
	importer *Importer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	identityDB := openSQLite(t)
	schema, err := os.ReadFile("../../packages/identity-platform/migrations/003_accounts.sql")
	require.NoError(t, err)
	prefixed := &packagestoresdk.Store{Lease: packagebridgesdk.StorageLease{Driver: "sqlite", TablePrefix: "t_"}}
	require.NoError(t, identityDB.Exec(prefixed.ExpandScript(string(schema))).Error)
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

	return &fixture{store: store, identity: identity, importer: &Importer{
		Connect: func() (grpc.ClientConnInterface, error) { return conn, nil },
	}}
}

func TestImportAccountHandsOneAccountToIdentity(t *testing.T) {
	f := newFixture(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("secret-1"), bcrypt.MinCost)
	require.NoError(t, err)
	user := model.User{ID: 7, Email: "owner@example.test", Password: string(hash), IsAdmin: 1, CreatedAt: time.Now()}
	ctx := context.Background()
	require.NoError(t, f.importer.ImportAccount(ctx, user, AccountUUID(7)))

	accounts, err := f.store.Get(ctx, []uint64{7})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.True(t, accounts[0].IsAdmin)
	require.Equal(t, "owner@example.test", accounts[0].Email)
	require.Equal(t, AccountUUID(7), accounts[0].AccountUUID)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(accounts[0].PasswordHash), []byte("secret-1")))

	// A failed attempt (the identity host still starting) is retried; a
	// request that ended is not.
	f.identity.failOn = f.identity.calls + 1
	require.NoError(t, f.importer.ImportAccount(ctx, model.User{ID: 8, Email: "other@example.test", Password: string(hash), IsAdmin: 1}, AccountUUID(8)))
	ended, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, f.importer.ImportAccount(ended, model.User{ID: 9, Email: "third@example.test", Password: string(hash), IsAdmin: 1}, AccountUUID(9)))
	require.ErrorIs(t, (&Importer{}).ImportAccount(ctx, user, AccountUUID(7)), ErrNotInitialized)
	var none *Importer
	require.ErrorIs(t, none.ImportAccount(ctx, user, AccountUUID(7)), ErrNotInitialized)
}

type endedStream struct{ cause error }

func (s endedStream) CloseAndRecv() (*identityv1.ImportAccountsResponse, error) { return nil, s.cause }

// A Send that fails with a bare EOF means the identity module ended the
// stream: the reason is the status CloseAndRecv returns, and it must be the
// one the checkpoint records.
func TestSendFailureReportsTheStatusBehindAnEOF(t *testing.T) {
	cause := status.Error(codes.Unavailable, "identity restarted")
	err := sendFailure(endedStream{cause: cause}, "send account 3", io.EOF)
	require.ErrorContains(t, err, "send account 3")
	require.ErrorContains(t, err, "identity restarted")
	require.Equal(t, codes.Unavailable, status.Code(errors.Unwrap(err)))

	other := errors.New("connection reset")
	err = sendFailure(endedStream{cause: cause}, "send account 3", other)
	require.ErrorIs(t, err, other, "only EOF hides a status")
	require.NotContains(t, err.Error(), "identity restarted")

	err = sendFailure(endedStream{}, "send account 3", io.EOF)
	require.ErrorIs(t, err, io.EOF, "an EOF without a status stays an EOF")
}
