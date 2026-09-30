package identitytoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Contract fixtures shared by the identity module, the kernel and the SDK.
const contractDir = "../../contracts/identity/v1"

type goldenToken struct {
	Header       map[string]any `json:"header"`
	Claims       map[string]any `json:"claims"`
	Verification struct {
		Algorithms    []string `json:"algorithms"`
		Issuer        string   `json:"issuer"`
		Audience      string   `json:"audience"`
		LeewaySeconds int      `json:"leeway_seconds"`
	} `json:"verification"`
}

type negativeCase struct {
	Name   string         `json:"name"`
	Header map[string]any `json:"header"`
	Claims map[string]any `json:"claims"`
}

// Revocation state belongs to the verifier (the kernel checks these cases).
var revocationCases = map[string]bool{
	"stale-token-version": true, "revoked-session": true, "issued-before-not-before": true,
}

func readJSON(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractDir, name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}

func sign(t *testing.T, header, claims map[string]any, key ed25519.PrivateKey) string {
	t.Helper()
	encode := func(value map[string]any) string {
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signingInput := encode(header) + "." + encode(claims)
	switch header["alg"] {
	case "EdDSA":
		return signingInput + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(signingInput)))
	case "HS256":
		signature, err := jwt.SigningMethodHS256.Sign(signingInput, []byte("any-shared-secret"))
		require.NoError(t, err)
		return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	default:
		return signingInput + "."
	}
}

func merge(base, changes map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range changes {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	return merged
}

func fixtureKeys(t *testing.T, golden goldenToken) (KeySet, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	revokedPublic, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	kid := golden.Header["kid"].(string)
	return KeySet{
		kid:           {ID: kid, PublicKey: public},
		"idk-revoked": {ID: "idk-revoked", PublicKey: revokedPublic, Revoked: true},
	}, private
}

func expectationsFor(golden goldenToken) Expectations {
	issuedAt := int64(golden.Claims["iat"].(float64))
	return Expectations{
		Issuer: golden.Verification.Issuer, Audience: golden.Verification.Audience,
		Leeway: time.Duration(golden.Verification.LeewaySeconds) * time.Second,
		Now:    func() time.Time { return time.Unix(issuedAt+600, 0) },
	}
}

func TestGoldenTokenVerifies(t *testing.T) {
	var golden goldenToken
	readJSON(t, "access-token-golden.json", &golden)
	require.Equal(t, []string{Algorithm}, golden.Verification.Algorithms)
	require.Equal(t, int(DefaultLeeway/time.Second), golden.Verification.LeewaySeconds)
	keys, private := fixtureKeys(t, golden)

	claims, err := Verify(sign(t, golden.Header, golden.Claims, private), keys, expectationsFor(golden))
	require.NoError(t, err)
	require.Equal(t, uint64(42), claims.UserID)
	require.Equal(t, "member@example.test", claims.Email)
	require.Equal(t, uint64(3), claims.TokenVersion)
	require.Equal(t, golden.Claims["sid"], claims.SessionID)
	require.Equal(t, golden.Claims["jti"], claims.ID)
}

func TestNegativeTokensAreRefused(t *testing.T) {
	var golden goldenToken
	readJSON(t, "access-token-golden.json", &golden)
	var negative struct {
		Cases []negativeCase `json:"cases"`
	}
	readJSON(t, "access-token-negative.json", &negative)
	keys, private := fixtureKeys(t, golden)

	checked := 0
	for _, testCase := range negative.Cases {
		if revocationCases[testCase.Name] {
			continue
		}
		t.Run(testCase.Name, func(t *testing.T) {
			token := sign(t, merge(golden.Header, testCase.Header), merge(golden.Claims, testCase.Claims), private)
			_, err := Verify(token, keys, expectationsFor(golden))
			require.ErrorIs(t, err, ErrInvalidToken)
		})
		checked++
	}
	require.Equal(t, len(negative.Cases)-len(revocationCases), checked, "every non-revocation case is covered")
}

func TestRequiredClaimsAndExpectations(t *testing.T) {
	var golden goldenToken
	readJSON(t, "access-token-golden.json", &golden)
	keys, private := fixtureKeys(t, golden)
	for _, claim := range []string{"sub", "user_id", "sid", "tv", "jti", "iat", "exp", "aud", "iss"} {
		t.Run("missing "+claim, func(t *testing.T) {
			token := sign(t, golden.Header, merge(golden.Claims, map[string]any{claim: nil}), private)
			_, err := Verify(token, keys, expectationsFor(golden))
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}
	token := sign(t, golden.Header, merge(golden.Claims, map[string]any{"sub": "43"}), private)
	_, err := Verify(token, keys, expectationsFor(golden))
	require.ErrorIs(t, err, ErrInvalidToken, "sub must be the user id")

	_, err = Verify(sign(t, golden.Header, golden.Claims, private), keys, Expectations{Issuer: golden.Verification.Issuer})
	require.ErrorIs(t, err, ErrInvalidToken, "a verifier must say which audience it is")
}

func TestJWKSRoundTrip(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	document, err := json.Marshal(JWKS{Keys: []JWK{NewJWK("idk-a", public)}})
	require.NoError(t, err)
	keys, err := ParseJWKS(document)
	require.NoError(t, err)
	require.Equal(t, public, keys["idk-a"].PublicKey)

	for name, document := range map[string]string{
		"rsa key":   `{"keys":[{"kty":"RSA","kid":"k","n":"x","e":"AQAB"}]}`,
		"short key": `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"k","x":"AAAA"}]}`,
		"duplicate": `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"k","x":"` + base64.RawURLEncoding.EncodeToString(public) + `"},{"kty":"OKP","crv":"Ed25519","kid":"k","x":"` + base64.RawURLEncoding.EncodeToString(public) + `"}]}`,
	} {
		_, err := ParseJWKS([]byte(document))
		require.Error(t, err, name)
	}
}
