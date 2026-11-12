package authn

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.IdentityRevocation{}, &model.IdentitySessionRevocation{}))
	return db
}

func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	require.NoError(t, err)
	return token
}

// claimsAt are a verified token's claims: token version 1, issued at
// issuedAt.
func claimsAt(userID uint, issuedAt time.Time) *utils.Claims {
	return &utils.Claims{
		UserID: userID, Email: "user@example.test", SessionID: "session-1", TokenVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IdentityIssuer,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(time.Hour)),
		},
	}
}

func TestVerifyRefusesEveryOtherAlgorithm(t *testing.T) {
	f := newIdentityFixture(t)
	verifier := &Verifier{IdentityKeys: f.keys}
	valid := claimsAt(7, time.Now())
	cases := map[string]string{
		"HS256":    sign(t, jwt.SigningMethodHS256, []byte("a-shared-secret"), valid),
		"alg none": sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid),
		"garbage":  "not-a-token",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := verifier.Verify(context.Background(), token)
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

func TestUserRevocationRejectsOlderTokens(t *testing.T) {
	store := NewStore(testDB(t), 25*time.Hour)
	// The revocation is two seconds old, so the next second is not in the future.
	now := time.Now().Add(-2 * time.Second)
	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, NotBefore: now, Reason: "ban"}))

	require.True(t, store.Revoked(claimsAt(7, now.Add(-time.Minute))))
	require.True(t, store.Revoked(claimsAt(7, now)), "iat has second precision: the revocation's own second is revoked")
	require.False(t, store.Revoked(claimsAt(7, now.Truncate(time.Second).Add(time.Second))), "a token issued in a later second is valid")
	require.False(t, store.Revoked(claimsAt(8, now.Add(-time.Minute))))
}

func TestTokenVersionAndSessionRevocations(t *testing.T) {
	store := NewStore(testDB(t), 25*time.Hour)
	now := time.Now()
	versioned := claimsAt(7, now)
	versioned.TokenVersion = 2

	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, TokenVersion: 2}))
	require.False(t, store.Revoked(versioned), "tv equal to the revoked version is current")
	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, TokenVersion: 3}))
	require.True(t, store.Revoked(versioned))

	require.NoError(t, store.Publish(context.Background(), Revocation{
		UserID: 9, SessionID: "session-1", SessionExpiresAt: now.Add(time.Hour), Reason: "logout",
	}))
	require.True(t, store.Revoked(claimsAt(9, now)))
}

func TestRevocationBoundsOnlyMoveForward(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	require.NoError(t, Write(db, Revocation{UserID: 7, NotBefore: now, TokenVersion: 4}))
	require.NoError(t, Write(db, Revocation{UserID: 7, NotBefore: now.Add(-time.Hour), TokenVersion: 1}))
	var row model.IdentityRevocation
	require.NoError(t, db.Take(&row, "user_id = ?", 7).Error)
	require.WithinDuration(t, now, row.NotBefore, time.Millisecond)
	require.Equal(t, uint64(4), row.TokenVersion)

	require.Error(t, Write(db, Revocation{UserID: 7}), "a revocation needs a bound")
	require.Error(t, Write(db, Revocation{NotBefore: now}), "a revocation needs a user")
}

func TestReloadPicksUpOtherWritersAndPrunes(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	require.NoError(t, Write(db, Revocation{UserID: 7, NotBefore: now}))
	require.NoError(t, Write(db, Revocation{UserID: 8, NotBefore: now.Add(-48 * time.Hour)}))
	require.NoError(t, Write(db, Revocation{UserID: 9, SessionID: "gone", SessionExpiresAt: now.Add(-time.Minute)}))

	store := NewStore(db, 25*time.Hour)
	claims := claimsAt(7, now.Add(-time.Minute))
	require.False(t, store.Revoked(claims), "nothing is loaded before the first reload")

	require.NoError(t, store.Reload(context.Background()))
	require.True(t, store.Revoked(claims))

	var users, sessions int64
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Count(&users).Error)
	require.NoError(t, db.Model(&model.IdentitySessionRevocation{}).Count(&sessions).Error)
	require.Equal(t, int64(1), users, "a revocation older than any token lifetime is pruned")
	require.Zero(t, sessions, "an expired session revocation is pruned")
}

func countDeletes(t *testing.T, db *gorm.DB) *int {
	t.Helper()
	deletes := new(int)
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:count_deletes", func(*gorm.DB) { *deletes++ }))
	return deletes
}

// The tables are read at every reload, but pruned at most every
// PruneInterval, and a prune with nothing to delete writes nothing.
func TestReloadPrunesAtMostEveryPruneInterval(t *testing.T) {
	db := testDB(t)
	clock := time.Now()
	store := NewStore(db, 25*time.Hour)
	store.now = func() time.Time { return clock }
	deletes := countDeletes(t, db)
	ctx := context.Background()

	require.NoError(t, store.Reload(ctx))
	require.Zero(t, *deletes, "an empty table is read and not written")

	// An old revocation appears: the next reload reads it but leaves it.
	require.NoError(t, Write(db, Revocation{UserID: 8, NotBefore: clock.Add(-48 * time.Hour)}))
	require.NoError(t, Write(db, Revocation{UserID: 7, NotBefore: clock}))
	clock = clock.Add(5 * time.Second)
	require.NoError(t, store.Reload(ctx))
	require.Zero(t, *deletes)
	var users int64
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Count(&users).Error)
	require.Equal(t, int64(2), users, "not pruned inside the interval")
	require.True(t, store.Revoked(claimsAt(7, clock.Add(-time.Minute))), "but every reload reads: a revocation of another process applies")

	clock = clock.Add(PruneInterval)
	require.NoError(t, store.Reload(ctx))
	require.Equal(t, 1, *deletes, "one DELETE, for the table that had an expired row")
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Count(&users).Error)
	require.Equal(t, int64(1), users)

	clock = clock.Add(PruneInterval)
	require.NoError(t, store.Reload(ctx))
	require.Equal(t, 1, *deletes, "nothing left to prune: no write")
}

// Tokens follow the token version: one identity issues right after a
// versioned revocation, within the same second, keeps working.
func TestVersionedRevocationsSpareTokensIssuedAfterThem(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := NewStore(db, 25*time.Hour)
	now := time.Now().Truncate(time.Second).Add(500 * time.Millisecond)
	identity := func(tokenVersion uint64, issuedAt time.Time) *utils.Claims {
		claims := claimsAt(7, issuedAt)
		claims.TokenVersion = tokenVersion
		return claims
	}

	require.NoError(t, store.Publish(ctx, Revocation{UserID: 7, TokenVersion: 3, NotBefore: now, Reason: "user is_admin changed"}))
	require.True(t, store.Revoked(identity(2, now.Add(-time.Minute))), "an older token version")
	require.False(t, store.Revoked(identity(3, now)), "issued after the change, in the same second")

	// Without a token version a revocation bounds identity tokens by time.
	later := now.Add(time.Minute)
	require.NoError(t, store.Publish(ctx, Revocation{UserID: 7, NotBefore: later, Reason: "user deleted"}))
	require.True(t, store.Revoked(identity(3, later)))
	require.False(t, store.Revoked(identity(3, later.Add(time.Second))))

	reloaded := NewStore(db, 25*time.Hour)
	require.NoError(t, reloaded.Reload(ctx))
	require.True(t, reloaded.Revoked(identity(2, later.Add(time.Second))), "the token version survives a reload")
	require.True(t, reloaded.Revoked(identity(3, later)), "the identity bound survives a reload")
	require.False(t, reloaded.Revoked(identity(3, later.Add(time.Second))))
}
