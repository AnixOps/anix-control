// Package server serves the identity module's IdentityService contract
// (sdk/api/identity/v1). Products add their own adapters around it; the
// service itself is product-neutral.
package server

import (
	"context"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeySource returns the current signing keys.
type KeySource interface {
	Load(ctx context.Context) ([]signingkey.Key, error)
}

// Server implements IdentityService. Keys is nil while no key-encryption key
// is configured: the service then publishes no keys and issues nothing.
type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	Keys   KeySource
	Policy signingkey.Policy
	// Issuer and Audience are the iss and aud claims of the tokens the
	// caller verifies.
	Issuer   string
	Audience string
	// Now defaults to time.Now.
	Now func() time.Time
}

// GetTokenKeys publishes the keys that verify tokens: NEXT, ACTIVE and
// RETIRED keys until they end, and REVOKED keys so verifiers drop them.
func (s *Server) GetTokenKeys(ctx context.Context, _ *identityv1.GetTokenKeysRequest) (*identityv1.GetTokenKeysResponse, error) {
	if s.Keys == nil {
		return nil, status.Error(codes.FailedPrecondition, "identity signing keys are not configured")
	}
	keys, err := s.Keys.Load(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "load signing keys: %v", err)
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	response := &identityv1.GetTokenKeysResponse{Issuer: s.Issuer, Audience: s.Audience}
	for _, key := range keys {
		notAfter := s.Policy.NotAfter(key)
		if !notAfter.IsZero() && !now.Before(notAfter) {
			continue
		}
		published := &identityv1.TokenKey{
			Kid: key.ID, Alg: identitytoken.Algorithm, PublicKey: key.PublicKey, State: keyState(key.State),
			NotBeforeUnix: unixOrZero(key.CreatedAt),
		}
		if !notAfter.IsZero() {
			published.NotAfterUnix = notAfter.Unix()
		}
		response.Keys = append(response.Keys, published)
	}
	return response, nil
}

func keyState(state signingkey.State) identityv1.TokenKeyState {
	switch state {
	case signingkey.StateNext:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_NEXT
	case signingkey.StateActive:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE
	case signingkey.StateRetired:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED
	case signingkey.StateRevoked:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_REVOKED
	default:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_UNSPECIFIED
	}
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
