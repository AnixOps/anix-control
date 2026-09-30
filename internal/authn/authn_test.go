package authn

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testSecret = "authn-test-secret-0123456789"

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

func legacyClaims(userID uint, issuedAt time.Time) *utils.Claims {
	return &utils.Claims{
		UserID: userID, Email: "user@example.test", SessionID: "session-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    utils.LegacyTokenIssuer,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(time.Hour)),
		},
	}
}

func verifier(store *Store) *Verifier {
	return &Verifier{Secret: func() string { return testSecret }, Revocations: store}
}

func TestVerifyAcceptsTheKernelsHS256Tokens(t *testing.T) {
	token, err := utils.GenerateToken(7, "user@example.test", true, testSecret, 3600)
	require.NoError(t, err)
	claims, err := verifier(nil).Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, uint(7), claims.UserID)
	require.True(t, claims.IsAdmin)
	require.NotEmpty(t, claims.SessionID, "new tokens carry a session id")
}

func TestVerifyPinsTheAlgorithmAndRequiredClaims(t *testing.T) {
	now := time.Now()
	valid := legacyClaims(7, now)
	noExpiry := legacyClaims(7, now)
	noExpiry.ExpiresAt = nil
	noIssuedAt := legacyClaims(7, now)
	noIssuedAt.IssuedAt = nil
	otherIssuer := legacyClaims(7, now)
	otherIssuer.Issuer = "someone-else"
	expired := legacyClaims(7, now.Add(-2*time.Hour))

	cases := map[string]string{
		"HS512 with the same secret": sign(t, jwt.SigningMethodHS512, []byte(testSecret), valid),
		"HS384 with the same secret": sign(t, jwt.SigningMethodHS384, []byte(testSecret), valid),
		"alg none":                   sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid),
		"wrong secret":               sign(t, jwt.SigningMethodHS256, []byte("another-secret"), valid),
		"no exp":                     sign(t, jwt.SigningMethodHS256, []byte(testSecret), noExpiry),
		"no iat":                     sign(t, jwt.SigningMethodHS256, []byte(testSecret), noIssuedAt),
		"other issuer":               sign(t, jwt.SigningMethodHS256, []byte(testSecret), otherIssuer),
		"expired":                    sign(t, jwt.SigningMethodHS256, []byte(testSecret), expired),
		"garbage":                    "not-a-token",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := verifier(nil).Verify(context.Background(), token)
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}

	_, err := (&Verifier{Secret: func() string { return "" }}).Verify(context.Background(),
		sign(t, jwt.SigningMethodHS256, []byte(""), valid))
	require.ErrorIs(t, err, ErrInvalidToken, "an unset secret never verifies")
}

func TestUserRevocationRejectsOlderTokens(t *testing.T) {
	store := NewStore(testDB(t), 25*time.Hour)
	// The revocation is two seconds old, so the next second is not in the future.
	now := time.Now().Add(-2 * time.Second)
	before := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(7, now.Add(-time.Minute)))
	sameSecond := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(7, now))
	nextSecond := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(7, now.Truncate(time.Second).Add(time.Second)))
	otherUser := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(8, now.Add(-time.Minute)))

	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, NotBefore: now, Reason: "ban"}))

	_, err := verifier(store).Verify(context.Background(), before)
	require.ErrorIs(t, err, ErrRevokedToken)
	_, err = verifier(store).Verify(context.Background(), sameSecond)
	require.ErrorIs(t, err, ErrRevokedToken, "iat has second precision: the revocation's own second is revoked")
	_, err = verifier(store).Verify(context.Background(), nextSecond)
	require.NoError(t, err, "a token issued in a later second is valid")
	_, err = verifier(store).Verify(context.Background(), otherUser)
	require.NoError(t, err)
}

func TestTokenVersionAndSessionRevocations(t *testing.T) {
	store := NewStore(testDB(t), 25*time.Hour)
	now := time.Now()
	claims := legacyClaims(7, now)
	claims.TokenVersion = 2
	versioned := sign(t, jwt.SigningMethodHS256, []byte(testSecret), claims)

	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, TokenVersion: 2}))
	_, err := verifier(store).Verify(context.Background(), versioned)
	require.NoError(t, err, "tv equal to the revoked version is current")
	require.NoError(t, store.Publish(context.Background(), Revocation{UserID: 7, TokenVersion: 3}))
	_, err = verifier(store).Verify(context.Background(), versioned)
	require.ErrorIs(t, err, ErrRevokedToken)

	session := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(9, now))
	require.NoError(t, store.Publish(context.Background(), Revocation{
		UserID: 9, SessionID: "session-1", SessionExpiresAt: now.Add(time.Hour), Reason: "logout",
	}))
	_, err = verifier(store).Verify(context.Background(), session)
	require.ErrorIs(t, err, ErrRevokedToken)
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
	token := sign(t, jwt.SigningMethodHS256, []byte(testSecret), legacyClaims(7, now.Add(-time.Minute)))
	_, err := verifier(store).Verify(context.Background(), token)
	require.NoError(t, err, "nothing is loaded before the first reload")

	require.NoError(t, store.Reload(context.Background()))
	_, err = verifier(store).Verify(context.Background(), token)
	require.ErrorIs(t, err, ErrRevokedToken)

	var users, sessions int64
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Count(&users).Error)
	require.NoError(t, db.Model(&model.IdentitySessionRevocation{}).Count(&sessions).Error)
	require.Equal(t, int64(1), users, "a revocation older than any token lifetime is pruned")
	require.Zero(t, sessions, "an expired session revocation is pruned")
}
