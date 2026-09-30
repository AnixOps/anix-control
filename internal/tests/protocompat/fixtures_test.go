package protocompat

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "identity", "v1", name))
	require.NoError(t, err)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	require.NoError(t, decoder.Decode(target), name)
}

func TestIdentityAccessTokenFixturesAreConsistent(t *testing.T) {
	var golden struct {
		Header         map[string]any `json:"header"`
		Claims         map[string]any `json:"claims"`
		RequiredClaims []string       `json:"required_claims"`
		Verification   struct {
			Algorithms []string `json:"algorithms"`
			Issuer     string   `json:"issuer"`
			Audience   string   `json:"audience"`
		} `json:"verification"`
	}
	readFixture(t, "access-token-golden.json", &golden)
	require.Equal(t, "EdDSA", golden.Header["alg"])
	require.NotEmpty(t, golden.Header["kid"])
	require.Equal(t, []string{"EdDSA"}, golden.Verification.Algorithms)
	require.Equal(t, golden.Verification.Issuer, golden.Claims["iss"])
	require.Equal(t, golden.Verification.Audience, golden.Claims["aud"])
	for _, claim := range golden.RequiredClaims {
		require.Contains(t, golden.Claims, claim)
	}

	var negative struct {
		Cases []struct {
			Name   string         `json:"name"`
			Header map[string]any `json:"header"`
			Claims map[string]any `json:"claims"`
			Reason string         `json:"reason"`
		} `json:"cases"`
	}
	readFixture(t, "access-token-negative.json", &negative)
	require.NotEmpty(t, negative.Cases)
	seen := map[string]bool{}
	for _, testCase := range negative.Cases {
		require.False(t, seen[testCase.Name], "duplicate case %s", testCase.Name)
		seen[testCase.Name] = true
		require.NotEmpty(t, testCase.Reason, testCase.Name)
		require.True(t, len(testCase.Header) > 0 || len(testCase.Claims) > 0, "case %s changes nothing", testCase.Name)
		for claim := range testCase.Claims {
			require.Contains(t, golden.Claims, claim, "case %s changes an unknown claim", testCase.Name)
		}
	}
}
