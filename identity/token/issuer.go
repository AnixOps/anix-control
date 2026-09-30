// Package token issues identity access tokens: compact JWS signed with the
// active Ed25519 key, with the claims of sdk/identitytoken. Every token names
// one audience, the service it is for.
package token

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/golang-jwt/jwt/v5"
)

// Issuer signs access tokens.
type Issuer struct {
	// Issuer is the iss claim, for example "anixops-identity".
	Issuer string
	// Lifetime is how long a token is valid.
	Lifetime time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// Subject is what a token says about its account.
type Subject struct {
	UserID       uint64
	Email        string
	IsAdmin      bool
	TokenVersion uint64
}

// Issue signs a token for subject and session, scoped to audience.
func (i Issuer) Issue(key signingkey.Key, subject Subject, sessionID, audience string) (string, *identitytoken.Claims, error) {
	switch {
	case i.Issuer == "" || i.Lifetime <= 0:
		return "", nil, errors.New("token issuer is not configured")
	case audience == "":
		return "", nil, errors.New("a token needs an audience")
	case key.State != signingkey.StateActive || key.PrivateKey == nil:
		return "", nil, errors.New("tokens are signed only with the active key")
	case subject.UserID == 0 || subject.TokenVersion == 0 || sessionID == "":
		return "", nil, errors.New("a token needs a user, a token version and a session")
	}
	now := time.Now
	if i.Now != nil {
		now = i.Now
	}
	issuedAt := now().Truncate(time.Second)
	tokenID := make([]byte, 16)
	if _, err := rand.Read(tokenID); err != nil {
		return "", nil, err
	}
	claims := &identitytoken.Claims{
		UserID: subject.UserID, Email: subject.Email, IsAdmin: subject.IsAdmin,
		SessionID: sessionID, TokenVersion: subject.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: i.Issuer, Subject: strconv.FormatUint(subject.UserID, 10), Audience: jwt.ClaimStrings{audience},
			ID: hex.EncodeToString(tokenID), IssuedAt: jwt.NewNumericDate(issuedAt), ExpiresAt: jwt.NewNumericDate(issuedAt.Add(i.Lifetime)),
		},
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	unsigned.Header["kid"] = key.ID
	signed, err := unsigned.SignedString(key.PrivateKey)
	if err != nil {
		return "", nil, err
	}
	return signed, claims, nil
}

// NewSessionID returns a random session id.
func NewSessionID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// Published returns the keys verifiers accept: NEXT, ACTIVE and RETIRED keys
// until their end, never REVOKED ones.
func Published(keys []signingkey.Key, policy signingkey.Policy, now time.Time) identitytoken.KeySet {
	set := identitytoken.KeySet{}
	for _, key := range keys {
		if key.State == signingkey.StateRevoked {
			continue
		}
		if end := policy.NotAfter(key); !end.IsZero() && !now.Before(end) {
			continue
		}
		set[key.ID] = identitytoken.Key{ID: key.ID, PublicKey: key.PublicKey}
	}
	return set
}

// JWKS renders the published keys as a JSON Web Key Set, in key id order.
func JWKS(set identitytoken.KeySet) identitytoken.JWKS {
	document := identitytoken.JWKS{Keys: []identitytoken.JWK{}}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		document.Keys = append(document.Keys, identitytoken.NewJWK(id, set[id].PublicKey))
	}
	return document
}
