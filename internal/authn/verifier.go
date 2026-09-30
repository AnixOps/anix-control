// Package authn verifies the kernel's user access tokens in one place: the
// HTTP middleware, the admin monitor WebSocket and the gRPC interceptor all
// call it. It pins the token algorithm and enforces revocations.
package authn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync/atomic"

	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/utils"
)

var (
	// ErrInvalidToken covers malformed, forged, expired and wrong-algorithm tokens.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrRevokedToken is a valid token whose user or session was revoked.
	ErrRevokedToken = errors.New("token has been revoked")
)

// IdentityKeys supplies the identity module's token keys.
type IdentityKeys interface {
	KeySet() identitytoken.KeySet
	// Kick asks for a refresh: a token named a key the kernel does not know.
	Kick()
}

// The kernel accepts identity tokens with this issuer and audience only.
const (
	IdentityIssuer   = "anixops-identity"
	IdentityAudience = "anix-control"
)

// Verifier checks the kernel's own HS256 tokens (signed with the configured
// secret) and the identity module's EdDSA tokens (signed with a published
// identity key), then the revocation store.
type Verifier struct {
	// Secret returns the current HS256 secret.
	Secret func() string
	// Revocations may be nil, which disables revocation checks (tools and
	// tests without a database).
	Revocations *Store
	// IdentityKeys may be nil: identity tokens are then refused.
	IdentityKeys IdentityKeys
}

// Verify returns the claims of a valid, unrevoked token.
func (v *Verifier) Verify(_ context.Context, token string) (*utils.Claims, error) {
	if v == nil {
		return nil, ErrInvalidToken
	}
	var (
		claims *utils.Claims
		err    error
	)
	if algorithm, kid := tokenHeader(token); algorithm == identitytoken.Algorithm {
		claims, err = v.verifyIdentity(token, kid)
	} else {
		if v.Secret == nil {
			return nil, ErrInvalidToken
		}
		claims, err = utils.ParseTokenWithSecret(token, v.Secret())
	}
	if err != nil {
		return nil, ErrInvalidToken
	}
	if v.Revocations.Revoked(claims) {
		return nil, ErrRevokedToken
	}
	return claims, nil
}

func (v *Verifier) verifyIdentity(token, kid string) (*utils.Claims, error) {
	if v.IdentityKeys == nil {
		return nil, ErrInvalidToken
	}
	keys := v.IdentityKeys.KeySet()
	if _, known := keys[kid]; !known && kid != "" {
		v.IdentityKeys.Kick()
	}
	verified, err := identitytoken.Verify(token, keys, identitytoken.Expectations{Issuer: IdentityIssuer, Audience: IdentityAudience})
	if err != nil {
		return nil, err
	}
	if verified.UserID > math.MaxUint32 {
		return nil, ErrInvalidToken
	}
	return &utils.Claims{
		UserID: uint(verified.UserID), Email: verified.Email, IsAdmin: verified.IsAdmin, // #nosec G115 -- bounded above.
		SessionID: verified.SessionID, TokenVersion: verified.TokenVersion, RegisteredClaims: verified.RegisteredClaims,
	}, nil
}

// tokenHeader reads alg and kid from a compact JWS header without trusting
// it: verification then pins the algorithm either way.
func tokenHeader(token string) (algorithm, kid string) {
	encoded, _, found := strings.Cut(token, ".")
	if !found {
		return "", ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ""
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if json.Unmarshal(raw, &header) != nil {
		return "", ""
	}
	return header.Algorithm, header.KeyID
}

var defaultStore atomic.Pointer[Store]

type identityKeysHolder struct{ keys IdentityKeys }

var defaultIdentityKeys atomic.Pointer[identityKeysHolder]

// SetDefaultIdentityKeys installs the identity keys the default verifier
// uses; the server sets them at start.
func SetDefaultIdentityKeys(keys IdentityKeys) {
	if keys == nil {
		defaultIdentityKeys.Store(nil)
		return
	}
	defaultIdentityKeys.Store(&identityKeysHolder{keys: keys})
}

// DefaultIdentityKeys returns the installed identity keys, or nil.
func DefaultIdentityKeys() IdentityKeys {
	if holder := defaultIdentityKeys.Load(); holder != nil {
		return holder.keys
	}
	return nil
}

// SetDefaultStore installs the revocation store the default verifier and
// Remember use; the server sets it at start.
func SetDefaultStore(store *Store) { defaultStore.Store(store) }

// DefaultStore returns the installed revocation store, or nil.
func DefaultStore() *Store { return defaultStore.Load() }

// Default returns a verifier over the loaded configuration's secret and the
// default revocation store.
func Default() *Verifier {
	return &Verifier{
		Secret: func() string {
			if cfg := config.Get(); cfg != nil {
				return cfg.JWT.Secret
			}
			return ""
		},
		Revocations:  DefaultStore(),
		IdentityKeys: DefaultIdentityKeys(),
	}
}

// Remember applies a committed revocation to the default store, if any.
func Remember(r Revocation) { DefaultStore().Remember(r) }
