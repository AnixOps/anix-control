package authn

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/identity/token"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/require"
)

type keySource struct {
	set    identitytoken.KeySet
	kicked int
}

func (k *keySource) KeySet() identitytoken.KeySet { return k.set }
func (k *keySource) Kick()                        { k.kicked++ }

type identityFixture struct {
	key    signingkey.Key
	keys   *keySource
	issuer token.Issuer
}

func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	key, err := signingkey.Generate(time.Now())
	require.NoError(t, err)
	key.State, key.ActivatedAt = signingkey.StateActive, time.Now()
	return &identityFixture{
		key:    key,
		keys:   &keySource{set: identitytoken.KeySet{key.ID: {ID: key.ID, PublicKey: key.PublicKey}}},
		issuer: token.Issuer{Issuer: IdentityIssuer, Lifetime: time.Hour},
	}
}

func (f *identityFixture) issue(t *testing.T, subject token.Subject, session, audience string) string {
	t.Helper()
	signed, _, err := f.issuer.Issue(f.key, subject, session, audience)
	require.NoError(t, err)
	return signed
}

func TestIdentityTokensVerifyAlongsideLegacyTokens(t *testing.T) {
	f := newIdentityFixture(t)
	verifier := &Verifier{Secret: func() string { return testSecret }, IdentityKeys: f.keys}

	claims, err := verifier.Verify(context.Background(), f.issue(t, token.Subject{UserID: 42, Email: "member@example.test", IsAdmin: true, TokenVersion: 3}, "s-1", IdentityAudience))
	require.NoError(t, err)
	require.Equal(t, uint(42), claims.UserID)
	require.True(t, claims.IsAdmin)
	require.Equal(t, "s-1", claims.SessionID)
	require.Equal(t, uint64(3), claims.TokenVersion)

	legacy, err := utils.GenerateToken(7, "legacy@example.test", false, testSecret, 3600)
	require.NoError(t, err)
	_, err = verifier.Verify(context.Background(), legacy)
	require.NoError(t, err, "the kernel's HS256 tokens keep working")

	_, err = verifier.Verify(context.Background(), f.issue(t, token.Subject{UserID: 42, TokenVersion: 1}, "s-1", "other-service"))
	require.ErrorIs(t, err, ErrInvalidToken, "tokens for another service are refused")

	_, err = (&Verifier{Secret: func() string { return testSecret }}).Verify(context.Background(), f.issue(t, token.Subject{UserID: 42, TokenVersion: 1}, "s-1", IdentityAudience))
	require.ErrorIs(t, err, ErrInvalidToken, "without identity keys identity tokens are refused")
}

func TestIdentityTokensWithUnknownOrRevokedKeys(t *testing.T) {
	f := newIdentityFixture(t)
	signed := f.issue(t, token.Subject{UserID: 42, TokenVersion: 1}, "s-1", IdentityAudience)
	verifier := &Verifier{IdentityKeys: f.keys}

	f.keys.set = identitytoken.KeySet{f.key.ID: {ID: f.key.ID, PublicKey: f.key.PublicKey, Revoked: true}}
	_, err := verifier.Verify(context.Background(), signed)
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Zero(t, f.keys.kicked, "a known revoked key needs no refresh")

	f.keys.set = identitytoken.KeySet{}
	_, err = verifier.Verify(context.Background(), signed)
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Equal(t, 1, f.keys.kicked, "an unknown key id asks for a refresh")
}

// The revocation cases of contracts/identity/v1/access-token-negative.json.
func TestIdentityTokenRevocationCases(t *testing.T) {
	f := newIdentityFixture(t)
	store := NewStore(testDB(t), 25*time.Hour)
	verifier := &Verifier{IdentityKeys: f.keys, Revocations: store}
	ctx := context.Background()

	staleVersion := f.issue(t, token.Subject{UserID: 42, TokenVersion: 2}, "s-1", IdentityAudience)
	require.NoError(t, store.Publish(ctx, Revocation{UserID: 42, TokenVersion: 3}))
	_, err := verifier.Verify(ctx, staleVersion)
	require.ErrorIs(t, err, ErrRevokedToken, "stale-token-version")
	_, err = verifier.Verify(ctx, f.issue(t, token.Subject{UserID: 42, TokenVersion: 3}, "s-1", IdentityAudience))
	require.NoError(t, err)

	loggedOut := f.issue(t, token.Subject{UserID: 43, TokenVersion: 1}, "revoked-session", IdentityAudience)
	require.NoError(t, store.Publish(ctx, Revocation{UserID: 43, SessionID: "revoked-session", SessionExpiresAt: time.Now().Add(time.Hour)}))
	_, err = verifier.Verify(ctx, loggedOut)
	require.ErrorIs(t, err, ErrRevokedToken, "revoked-session")

	issuedBefore := f.issue(t, token.Subject{UserID: 44, TokenVersion: 1}, "s-2", IdentityAudience)
	require.NoError(t, store.Publish(ctx, Revocation{UserID: 44, NotBefore: time.Now().Add(time.Second)}))
	_, err = verifier.Verify(ctx, issuedBefore)
	require.ErrorIs(t, err, ErrRevokedToken, "issued-before-not-before")
}
