// Package authntest signs identity access tokens for tests: the kernel
// accepts no other user tokens.
package authntest

import (
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/identity/token"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v5/internal/authn"
)

// Keys is a fixed identity key set.
type Keys identitytoken.KeySet

// KeySet returns the keys.
func (k Keys) KeySet() identitytoken.KeySet { return identitytoken.KeySet(k) }

// Kick does nothing: the set is fixed.
func (Keys) Kick() {}

var (
	once sync.Once
	key  signingkey.Key
	keys Keys
)

func signingKey(t testing.TB) signingkey.Key {
	t.Helper()
	once.Do(func() {
		generated, err := signingkey.Generate(time.Now())
		if err != nil {
			panic(err)
		}
		generated.State, generated.ActivatedAt = signingkey.StateActive, time.Now()
		key = generated
		keys = Keys{key.ID: {ID: key.ID, PublicKey: key.PublicKey}}
	})
	return key
}

// Token returns an identity token (token version 1) for the user and
// installs the test key in the default verifier, so the kernel's middleware
// accepts it.
func Token(t testing.TB, userID uint, email string, isAdmin bool) string {
	t.Helper()
	signer := signingKey(t)
	authn.SetDefaultIdentityKeys(keys)
	session, err := token.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	issuer := token.Issuer{Issuer: authn.IdentityIssuer, Lifetime: time.Hour}
	signed, _, err := issuer.Issue(signer, token.Subject{UserID: uint64(userID), Email: email, IsAdmin: isAdmin, TokenVersion: 1}, session, authn.IdentityAudience)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
