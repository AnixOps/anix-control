// Package authn verifies the kernel's user access tokens in one place: the
// HTTP middleware, the admin monitor WebSocket and the gRPC interceptor all
// call it. It pins the token algorithm and enforces revocations.
package authn

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/utils"
)

var (
	// ErrInvalidToken covers malformed, forged, expired and wrong-algorithm tokens.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrRevokedToken is a valid token whose user or session was revoked.
	ErrRevokedToken = errors.New("token has been revoked")
)

// Verifier checks HS256 tokens signed with the configured secret against the
// revocation store.
type Verifier struct {
	// Secret returns the current HS256 secret.
	Secret func() string
	// Revocations may be nil, which disables revocation checks (tools and
	// tests without a database).
	Revocations *Store
}

// Verify returns the claims of a valid, unrevoked token.
func (v *Verifier) Verify(_ context.Context, token string) (*utils.Claims, error) {
	if v == nil || v.Secret == nil {
		return nil, ErrInvalidToken
	}
	claims, err := utils.ParseTokenWithSecret(token, v.Secret())
	if err != nil {
		return nil, ErrInvalidToken
	}
	if v.Revocations.Revoked(claims) {
		return nil, ErrRevokedToken
	}
	return claims, nil
}

var defaultStore atomic.Pointer[Store]

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
		Revocations: DefaultStore(),
	}
}

// Remember applies a committed revocation to the default store, if any.
func Remember(r Revocation) { DefaultStore().Remember(r) }
