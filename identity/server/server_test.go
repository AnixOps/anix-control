package server

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type staticKeys []signingkey.Key

func (k staticKeys) Load(context.Context) ([]signingkey.Key, error) { return k, nil }

var policy = signingkey.Policy{RotateAfter: 30 * 24 * time.Hour, PublishAhead: 15 * time.Minute, VerifyAfterRetire: 25 * time.Hour}

func TestGetTokenKeysPublishesLiveKeysWithTheirStates(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, _, err := policy.Advance(nil, now, signingkey.Generate)
	require.NoError(t, err)
	keys, _, err = policy.Advance(keys, now.Add(policy.RotateAfter-policy.PublishAhead), signingkey.Generate)
	require.NoError(t, err)
	keys, _, err = policy.Advance(keys, now.Add(policy.RotateAfter), signingkey.Generate)
	require.NoError(t, err)
	revoked, err := signingkey.Generate(now)
	require.NoError(t, err)
	revoked.State = signingkey.StateRevoked
	keys = append(keys, revoked)

	server := &Server{Keys: staticKeys(keys), Policy: policy, Issuer: "anixops-identity", Audience: "anix-control",
		Now: func() time.Time { return now.Add(policy.RotateAfter + time.Hour) }}
	response, err := server.GetTokenKeys(context.Background(), &identityv1.GetTokenKeysRequest{})
	require.NoError(t, err)
	require.Equal(t, "anixops-identity", response.GetIssuer())
	require.Equal(t, "anix-control", response.GetAudience())
	states := map[identityv1.TokenKeyState]int{}
	for _, key := range response.GetKeys() {
		states[key.GetState()]++
		require.Equal(t, "EdDSA", key.GetAlg())
		require.Len(t, key.GetPublicKey(), 32)
		if key.GetState() == identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED {
			require.Equal(t, now.Add(policy.RotateAfter+policy.VerifyAfterRetire).Unix(), key.GetNotAfterUnix())
		}
	}
	require.Equal(t, map[identityv1.TokenKeyState]int{
		identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE: 1, identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED: 1,
		identityv1.TokenKeyState_TOKEN_KEY_STATE_REVOKED: 1,
	}, states)

	server.Now = func() time.Time { return now.Add(policy.RotateAfter + policy.VerifyAfterRetire) }
	response, err = server.GetTokenKeys(context.Background(), &identityv1.GetTokenKeysRequest{})
	require.NoError(t, err)
	for _, key := range response.GetKeys() {
		require.NotEqual(t, identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED, key.GetState(), "an expired retired key is no longer published")
	}
}

func TestWithoutKeysNothingIsPublished(t *testing.T) {
	_, err := (&Server{}).GetTokenKeys(context.Background(), &identityv1.GetTokenKeysRequest{})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = (&Server{}).BatchGetAccounts(context.Background(), &identityv1.BatchGetAccountsRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err), "accounts arrive with the import")
}
