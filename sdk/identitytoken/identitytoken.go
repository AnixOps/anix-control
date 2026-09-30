// Package identitytoken is the contract for AnixOps identity access tokens:
// the claims, the published key set (JWKS) and verification. Any AnixOps
// service verifies identity tokens with it, without the Control kernel.
//
// Tokens are compact JWS with alg EdDSA (Ed25519) and a kid header naming a
// published key; see contracts/identity/v1/access-token-golden.json.
// Revocation (token versions, sessions, not-before) is the verifier's own
// state and is not part of this package.
package identitytoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Algorithm is the only signature algorithm identity tokens use.
const Algorithm = "EdDSA"

// DefaultLeeway is the clock skew verification tolerates.
const DefaultLeeway = 60 * time.Second

// Claims are the claims of an identity access token.
type Claims struct {
	UserID       uint64 `json:"user_id"`
	Email        string `json:"email"`
	IsAdmin      bool   `json:"is_admin"`
	SessionID    string `json:"sid"`
	TokenVersion uint64 `json:"tv"`
	jwt.RegisteredClaims
}

// Validate checks the claims every identity token must carry, beyond what
// the JWT parser checks: a subject that is the user id, a session, a token
// version (versions start at 1) and a token id.
func (c *Claims) Validate() error {
	switch {
	case c.UserID == 0:
		return errors.New("user_id is required")
	case c.Subject != strconv.FormatUint(c.UserID, 10):
		return errors.New("sub must be the user id")
	case c.SessionID == "":
		return errors.New("sid is required")
	case c.TokenVersion == 0:
		return errors.New("tv is required")
	case c.ID == "":
		return errors.New("jti is required")
	case c.IssuedAt == nil:
		return errors.New("iat is required")
	}
	return nil
}

// Key is one published verification key.
type Key struct {
	ID        string
	PublicKey ed25519.PublicKey
	// Revoked keys verify nothing, even while still published.
	Revoked bool
}

// KeySet holds the keys a verifier accepts, by key id.
type KeySet map[string]Key

// Expectations are what a verifier requires of a token's issuer and audience.
type Expectations struct {
	Issuer   string
	Audience string
	// Leeway defaults to DefaultLeeway.
	Leeway time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// ErrInvalidToken wraps every reason a token is refused.
var ErrInvalidToken = errors.New("invalid identity token")

// Verify checks signature, algorithm, key id, issuer, audience, time and the
// required claims, and returns the claims.
func Verify(token string, keys KeySet, expect Expectations) (*Claims, error) {
	if expect.Issuer == "" || expect.Audience == "" {
		return nil, fmt.Errorf("%w: issuer and audience expectations are required", ErrInvalidToken)
	}
	leeway := expect.Leeway
	if leeway == 0 {
		leeway = DefaultLeeway
	}
	options := []jwt.ParserOption{
		jwt.WithValidMethods([]string{Algorithm}),
		jwt.WithIssuer(expect.Issuer),
		jwt.WithAudience(expect.Audience),
		jwt.WithLeeway(leeway),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	}
	if expect.Now != nil {
		options = append(options, jwt.WithTimeFunc(expect.Now))
	}
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(parsed *jwt.Token) (any, error) {
		kid, _ := parsed.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("kid is required")
		}
		key, ok := keys[kid]
		if !ok || key.Revoked || len(key.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("key %q is not published", kid)
		}
		return key.PublicKey, nil
	}, options...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if err := claims.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return claims, nil
}

// JWK is one key of a JSON Web Key Set (RFC 8037 OKP Ed25519).
type JWK struct {
	KeyType   string `json:"kty"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	KeyID     string `json:"kid"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
}

// JWKS is a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// NewJWK returns the JWK of an Ed25519 public key.
func NewJWK(id string, publicKey ed25519.PublicKey) JWK {
	return JWK{
		KeyType: "OKP", Curve: "Ed25519", X: base64.RawURLEncoding.EncodeToString(publicKey),
		KeyID: id, Use: "sig", Algorithm: Algorithm,
	}
}

// KeySet returns the verification keys of a JWKS. Keys that are not Ed25519
// signing keys are refused rather than skipped.
func (s JWKS) KeySet() (KeySet, error) {
	keys := make(KeySet, len(s.Keys))
	for _, key := range s.Keys {
		if key.KeyType != "OKP" || key.Curve != "Ed25519" || (key.Algorithm != "" && key.Algorithm != Algorithm) || (key.Use != "" && key.Use != "sig") {
			return nil, fmt.Errorf("key %q is not an Ed25519 signing key", key.KeyID)
		}
		raw, err := base64.RawURLEncoding.DecodeString(key.X)
		if err != nil || len(raw) != ed25519.PublicKeySize || key.KeyID == "" {
			return nil, fmt.Errorf("key %q is malformed", key.KeyID)
		}
		if _, exists := keys[key.KeyID]; exists {
			return nil, fmt.Errorf("key %q is listed twice", key.KeyID)
		}
		keys[key.KeyID] = Key{ID: key.KeyID, PublicKey: ed25519.PublicKey(raw)}
	}
	return keys, nil
}

// Published returns the key set as a JWKS in key id order, leaving out
// revoked keys.
func (s KeySet) Published() JWKS {
	ids := make([]string, 0, len(s))
	for id, key := range s {
		if !key.Revoked {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	document := JWKS{Keys: make([]JWK, 0, len(ids))}
	for _, id := range ids {
		document.Keys = append(document.Keys, NewJWK(id, s[id].PublicKey))
	}
	return document
}

// ParseJWKS decodes a JSON Web Key Set document.
func ParseJWKS(document []byte) (KeySet, error) {
	var set JWKS
	if err := json.Unmarshal(document, &set); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}
	return set.KeySet()
}
