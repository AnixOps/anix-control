package identitykeys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

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

type identityStub struct {
	identityv1.UnimplementedIdentityServiceServer
	response *identityv1.GetTokenKeysResponse
	err      error
	calls    int
}

func (s *identityStub) GetTokenKeys(context.Context, *identityv1.GetTokenKeysRequest) (*identityv1.GetTokenKeysResponse, error) {
	s.calls++
	return s.response, s.err
}

func dialStub(t *testing.T, stub *identityStub) grpc.ClientConnInterface {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(server, stub)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///identity", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.IdentityTokenKey{}))
	return db
}

func publicKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return public
}

func tokenKey(kid string, public ed25519.PublicKey, state identityv1.TokenKeyState, notAfter int64) *identityv1.TokenKey {
	return &identityv1.TokenKey{Kid: kid, Alg: "EdDSA", PublicKey: public, State: state, NotBeforeUnix: 1790000000, NotAfterUnix: notAfter}
}

func TestRefreshPersistsKeysAndMatchesTheIdentityModule(t *testing.T) {
	db := testDB(t)
	active, retired, revoked := publicKey(t), publicKey(t), publicKey(t)
	now := time.Unix(1790000000, 0)
	stub := &identityStub{response: &identityv1.GetTokenKeysResponse{Issuer: Issuer, Audience: Audience, Keys: []*identityv1.TokenKey{
		tokenKey("idk-active", active, identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE, 0),
		tokenKey("idk-retired", retired, identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED, now.Add(time.Hour).Unix()),
		tokenKey("idk-revoked", revoked, identityv1.TokenKeyState_TOKEN_KEY_STATE_REVOKED, 0),
	}}}
	keys := New(db)
	keys.now = func() time.Time { return now }
	require.NoError(t, keys.Refresh(context.Background(), dialStub(t, stub)))

	set := keys.KeySet()
	require.Equal(t, active, set["idk-active"].PublicKey)
	require.False(t, set["idk-active"].Revoked)
	require.True(t, set["idk-revoked"].Revoked, "revoked keys are kept so their tokens fail")
	require.Len(t, set.Published().Keys, 2, "revoked keys are not published")

	// A restarted kernel verifies from the table while identity is down.
	restarted := New(db)
	restarted.now = keys.now
	require.NoError(t, restarted.Reload(context.Background()))
	require.Equal(t, set, restarted.KeySet())

	restarted.now = func() time.Time { return now.Add(2 * time.Hour) }
	require.NotContains(t, restarted.KeySet(), "idk-retired", "a retired key stops at its end")

	stub.response.Keys = stub.response.Keys[:1]
	require.NoError(t, keys.Refresh(context.Background(), dialStub(t, stub)))
	var count int64
	require.NoError(t, db.Model(&model.IdentityTokenKey{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "keys identity no longer lists are dropped")
}

func TestRefreshRefusesForeignOrMalformedKeys(t *testing.T) {
	keys := New(testDB(t))
	for name, response := range map[string]*identityv1.GetTokenKeysResponse{
		"other audience": {Issuer: Issuer, Audience: "other-service"},
		"other issuer":   {Issuer: "v2board", Audience: Audience},
		"short key":      {Issuer: Issuer, Audience: Audience, Keys: []*identityv1.TokenKey{tokenKey("idk-a", []byte("short"), identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE, 0)}},
		"repeated kid": {Issuer: Issuer, Audience: Audience, Keys: []*identityv1.TokenKey{
			tokenKey("idk-a", publicKey(t), identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE, 0),
			tokenKey("idk-a", publicKey(t), identityv1.TokenKeyState_TOKEN_KEY_STATE_NEXT, 0),
		}},
	} {
		require.Error(t, keys.Refresh(context.Background(), dialStub(t, &identityStub{response: response})), name)
	}
	err := keys.Refresh(context.Background(), dialStub(t, &identityStub{err: status.Error(codes.FailedPrecondition, "no KEK")}))
	require.True(t, errors.Is(err, ErrNoKeys), err)
	require.Empty(t, keys.KeySet())
}

func TestUnknownKeysAskForARefresh(t *testing.T) {
	db := testDB(t)
	public := publicKey(t)
	stub := &identityStub{response: &identityv1.GetTokenKeysResponse{Issuer: Issuer, Audience: Audience, Keys: []*identityv1.TokenKey{
		tokenKey("idk-new", public, identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE, 0),
	}}}
	conn := dialStub(t, stub)
	keys := New(db)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		keys.Run(ctx, time.Hour, func() (grpc.ClientConnInterface, error) { return conn, nil }, func(err error) { t.Error(err) })
		close(done)
	}()
	require.Eventually(t, func() bool { return len(keys.KeySet()) == 1 }, 5*time.Second, 10*time.Millisecond, "the first refresh runs at once")

	stub.response.Keys = append(stub.response.Keys, tokenKey("idk-next", publicKey(t), identityv1.TokenKeyState_TOKEN_KEY_STATE_NEXT, 0))
	keys.Kick()
	require.Eventually(t, func() bool { return len(keys.KeySet()) == 2 }, 5*time.Second, 10*time.Millisecond, "a kick refreshes before the interval")
	calls := stub.calls
	keys.Kick()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, calls, stub.calls, "kicks are rate limited")
	cancel()
	<-done
}
