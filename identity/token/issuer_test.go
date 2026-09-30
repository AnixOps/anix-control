package token

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/stretchr/testify/require"
)

var policy = signingkey.Policy{RotateAfter: 30 * 24 * time.Hour, PublishAhead: 15 * time.Minute, VerifyAfterRetire: 25 * time.Hour}

func TestIssuedTokensVerifyForTheirAudienceOnly(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, _, err := policy.Advance(nil, now, signingkey.Generate)
	require.NoError(t, err)
	active, err := signingkey.Signing(keys)
	require.NoError(t, err)
	issuer := Issuer{Issuer: "anixops-identity", Lifetime: 24 * time.Hour, Now: func() time.Time { return now }}
	session, err := NewSessionID()
	require.NoError(t, err)

	token, issued, err := issuer.Issue(active, Subject{UserID: 42, Email: "member@example.test", TokenVersion: 3}, session, "anix-control")
	require.NoError(t, err)
	published := Published(keys, policy, now)
	verified, err := identitytoken.Verify(token, published, identitytoken.Expectations{
		Issuer: "anixops-identity", Audience: "anix-control", Now: func() time.Time { return now.Add(time.Hour) },
	})
	require.NoError(t, err)
	require.Equal(t, issued.ID, verified.ID)
	require.Equal(t, session, verified.SessionID)

	_, err = identitytoken.Verify(token, published, identitytoken.Expectations{
		Issuer: "anixops-identity", Audience: "another-service", Now: func() time.Time { return now.Add(time.Hour) },
	})
	require.ErrorIs(t, err, identitytoken.ErrInvalidToken, "a token is scoped to one service")

	// The document other services fetch carries the same keys.
	parsed, err := JWKS(published).KeySet()
	require.NoError(t, err)
	_, err = identitytoken.Verify(token, parsed, identitytoken.Expectations{
		Issuer: "anixops-identity", Audience: "anix-control", Now: func() time.Time { return now.Add(time.Hour) },
	})
	require.NoError(t, err)
}

func TestRevokedAndExpiredKeysAreNotPublished(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, _, err := policy.Advance(nil, now, signingkey.Generate)
	require.NoError(t, err)
	active, err := signingkey.Signing(keys)
	require.NoError(t, err)
	issuer := Issuer{Issuer: "anixops-identity", Lifetime: 24 * time.Hour, Now: func() time.Time { return now }}
	token, _, err := issuer.Issue(active, Subject{UserID: 7, TokenVersion: 1}, "s", "anix-control")
	require.NoError(t, err)

	revoked, err := signingkey.Revoke(keys, active.ID)
	require.NoError(t, err)
	require.Empty(t, Published(revoked, policy, now))
	_, err = identitytoken.Verify(token, Published(revoked, policy, now), identitytoken.Expectations{
		Issuer: "anixops-identity", Audience: "anix-control", Now: func() time.Time { return now },
	})
	require.ErrorIs(t, err, identitytoken.ErrInvalidToken)

	_, _, err = issuer.Issue(revoked[0], Subject{UserID: 7, TokenVersion: 1}, "s", "anix-control")
	require.Error(t, err, "only the active key signs")
	_, _, err = issuer.Issue(active, Subject{UserID: 7}, "s", "anix-control")
	require.Error(t, err, "a token needs a token version")
}
