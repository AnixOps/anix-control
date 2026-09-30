package signingkey

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var policy = Policy{RotateAfter: 30 * 24 * time.Hour, PublishAhead: 15 * time.Minute, VerifyAfterRetire: 25 * time.Hour}

func states(keys []Key) map[string]State {
	out := map[string]State{}
	for _, key := range keys {
		out[key.ID] = key.State
	}
	return out
}

func TestKeysArePublishedBeforeTheySignAndVerifyAfterRetiring(t *testing.T) {
	start := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, changed, err := policy.Advance(nil, start, Generate)
	require.NoError(t, err)
	require.True(t, changed)
	first, err := Signing(keys)
	require.NoError(t, err, "the first key signs at once")

	keys, changed, err = policy.Advance(keys, start.Add(24*time.Hour), Generate)
	require.NoError(t, err)
	require.False(t, changed, "nothing is due during the key's term")

	nextDue := start.Add(policy.RotateAfter - policy.PublishAhead)
	keys, changed, err = policy.Advance(keys, nextDue, Generate)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, keys, 2)
	signing, err := Signing(keys)
	require.NoError(t, err)
	require.Equal(t, first.ID, signing.ID, "a NEXT key is published but does not sign")

	rotated := start.Add(policy.RotateAfter)
	keys, _, err = policy.Advance(keys, rotated, Generate)
	require.NoError(t, err)
	signing, err = Signing(keys)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, signing.ID)
	require.Equal(t, StateRetired, states(keys)[first.ID])

	keys, _, err = policy.Advance(keys, rotated.Add(policy.VerifyAfterRetire-time.Second), Generate)
	require.NoError(t, err)
	require.Contains(t, states(keys), first.ID, "a retired key verifies until its tokens expired")
	keys, _, err = policy.Advance(keys, rotated.Add(policy.VerifyAfterRetire), Generate)
	require.NoError(t, err)
	require.NotContains(t, states(keys), first.ID)
}

func TestANextKeyWaitsForItsPublicationTime(t *testing.T) {
	start := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, _, err := policy.Advance(nil, start, Generate)
	require.NoError(t, err)
	// The kernel was down: the rotation is overdue but no NEXT key exists.
	late := start.Add(policy.RotateAfter + time.Hour)
	keys, _, err = policy.Advance(keys, late, Generate)
	require.NoError(t, err)
	active, err := Signing(keys)
	require.NoError(t, err)
	require.Equal(t, start, active.ActivatedAt, "a key created now is not trusted by verifiers yet")
	keys, _, err = policy.Advance(keys, late.Add(policy.PublishAhead), Generate)
	require.NoError(t, err)
	promoted, err := Signing(keys)
	require.NoError(t, err)
	require.NotEqual(t, active.ID, promoted.ID)
}

func TestRevokingTheActiveKeyActivatesAnother(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	keys, _, err := policy.Advance(nil, now, Generate)
	require.NoError(t, err)
	active, err := Signing(keys)
	require.NoError(t, err)
	keys, err = Revoke(keys, active.ID)
	require.NoError(t, err)
	_, err = Signing(keys)
	require.Error(t, err)
	keys, _, err = policy.Advance(keys, now.Add(time.Minute), Generate)
	require.NoError(t, err)
	replacement, err := Signing(keys)
	require.NoError(t, err)
	require.NotEqual(t, active.ID, replacement.ID)
	require.Equal(t, StateRevoked, states(keys)[active.ID], "revoked keys stay recorded")
	_, err = Revoke(keys, "idk-missing")
	require.Error(t, err)
}

func TestPolicyValidation(t *testing.T) {
	require.Error(t, Policy{RotateAfter: time.Hour, PublishAhead: 2 * time.Hour, VerifyAfterRetire: time.Hour}.Validate())
	require.Error(t, Policy{}.Validate())
	require.NoError(t, policy.Validate())
}

func TestSealedKeysOpenOnlyForTheirID(t *testing.T) {
	kek, err := ParseKEK("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	require.NoError(t, err)
	sealer, err := NewSealer(kek)
	require.NoError(t, err)
	key, err := Generate(time.Now())
	require.NoError(t, err)

	sealed, err := sealer.Seal(key.ID, key.PrivateKey)
	require.NoError(t, err)
	require.NotContains(t, sealed, string(key.PrivateKey.Seed()))
	opened, err := sealer.Open(key.ID, sealed)
	require.NoError(t, err)
	require.True(t, key.PrivateKey.Equal(opened))
	require.Equal(t, key.PublicKey, opened.Public().(ed25519.PublicKey))

	_, err = sealer.Open("idk-other", sealed)
	require.Error(t, err, "a sealed key cannot be moved to another row")
	otherKEK, err := ParseKEK("ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100")
	require.NoError(t, err)
	other, err := NewSealer(otherKEK)
	require.NoError(t, err)
	_, err = other.Open(key.ID, sealed)
	require.Error(t, err, "another KEK cannot open it")

	_, err = ParseKEK("too-short")
	require.Error(t, err)
}
