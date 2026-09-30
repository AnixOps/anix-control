// Package identitykeys keeps the identity module's token verification keys in
// the kernel: pulled with IdentityService.GetTokenKeys, persisted in
// v4_kernel_identity_token_key so tokens keep verifying while the identity
// module is down, and cached in memory for every request.
package identitykeys

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The kernel verifies only identity tokens issued for it.
const (
	Issuer   = "anixops-identity"
	Audience = "anix-control"
)

// kickInterval bounds refreshes asked for by unknown key ids.
const kickInterval = 10 * time.Second

// ErrNoKeys means the identity module publishes no keys (no KEK yet).
var ErrNoKeys = errors.New("the identity module publishes no token keys")

type cachedKey struct {
	key      identitytoken.Key
	notAfter time.Time
}

// Keys is the kernel's copy of the identity token keys.
type Keys struct {
	db  *gorm.DB
	now func() time.Time

	mu   sync.RWMutex
	keys map[string]cachedKey

	kick     chan struct{}
	kickMu   sync.Mutex
	lastKick time.Time
}

// New returns an empty key cache over db; call Reload to load it.
func New(db *gorm.DB) *Keys {
	return &Keys{db: db, now: time.Now, keys: map[string]cachedKey{}, kick: make(chan struct{}, 1)}
}

// KeySet returns the keys that verify now; revoked keys are kept, marked, so
// their tokens fail.
func (k *Keys) KeySet() identitytoken.KeySet {
	if k == nil {
		return identitytoken.KeySet{}
	}
	now := k.now()
	k.mu.RLock()
	defer k.mu.RUnlock()
	set := make(identitytoken.KeySet, len(k.keys))
	for id, cached := range k.keys {
		if !cached.notAfter.IsZero() && !now.Before(cached.notAfter) {
			continue
		}
		set[id] = cached.key
	}
	return set
}

// Kick asks for a refresh soon, at most every kickInterval; the verifier
// calls it for a token whose key id is unknown.
func (k *Keys) Kick() {
	if k == nil {
		return
	}
	k.kickMu.Lock()
	defer k.kickMu.Unlock()
	if now := k.now(); now.Sub(k.lastKick) >= kickInterval {
		k.lastKick = now
		select {
		case k.kick <- struct{}{}:
		default:
		}
	}
}

// Reload replaces the cache with the persisted keys.
func (k *Keys) Reload(ctx context.Context) error {
	var rows []model.IdentityTokenKey
	if err := k.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return fmt.Errorf("load identity token keys: %w", err)
	}
	keys := make(map[string]cachedKey, len(rows))
	for _, row := range rows {
		public, err := base64.StdEncoding.DecodeString(row.PublicKey)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return fmt.Errorf("identity token key %q is malformed", row.KeyID)
		}
		cached := cachedKey{key: identitytoken.Key{ID: row.KeyID, PublicKey: public, Revoked: row.State == stateRevoked}}
		if row.NotAfter != nil {
			cached.notAfter = *row.NotAfter
		}
		keys[row.KeyID] = cached
	}
	k.mu.Lock()
	k.keys = keys
	k.mu.Unlock()
	return nil
}

const stateRevoked = "revoked"

var states = map[identityv1.TokenKeyState]string{
	identityv1.TokenKeyState_TOKEN_KEY_STATE_NEXT:    "next",
	identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE:  "active",
	identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED: "retired",
	identityv1.TokenKeyState_TOKEN_KEY_STATE_REVOKED: stateRevoked,
}

// Refresh pulls the keys over conn and makes the table and the cache match
// them: new and changed keys are stored, keys the identity module no longer
// lists (retired past their end) are dropped.
func (k *Keys) Refresh(ctx context.Context, conn grpc.ClientConnInterface) error {
	response, err := identityv1.NewIdentityServiceClient(conn).GetTokenKeys(ctx, &identityv1.GetTokenKeysRequest{})
	if status.Code(err) == codes.FailedPrecondition {
		return ErrNoKeys
	}
	if err != nil {
		return fmt.Errorf("get identity token keys: %w", err)
	}
	if response.GetIssuer() != Issuer || response.GetAudience() != Audience {
		return fmt.Errorf("identity token keys are for %q/%q, not %s/%s", response.GetIssuer(), response.GetAudience(), Issuer, Audience)
	}
	now := k.now()
	rows := make([]model.IdentityTokenKey, 0, len(response.GetKeys()))
	seen := map[string]bool{}
	for _, key := range response.GetKeys() {
		state, known := states[key.GetState()]
		switch {
		case key.GetKid() == "" || len(key.GetKid()) > 64 || seen[key.GetKid()]:
			return fmt.Errorf("identity token key id %q is invalid or repeated", key.GetKid())
		case key.GetAlg() != identitytoken.Algorithm || len(key.GetPublicKey()) != ed25519.PublicKeySize || !known:
			return fmt.Errorf("identity token key %q is not a valid Ed25519 key", key.GetKid())
		}
		seen[key.GetKid()] = true
		row := model.IdentityTokenKey{
			KeyID: key.GetKid(), PublicKey: base64.StdEncoding.EncodeToString(key.GetPublicKey()), State: state, UpdatedAt: now,
		}
		if key.GetNotBeforeUnix() > 0 {
			value := time.Unix(key.GetNotBeforeUnix(), 0)
			row.NotBefore = &value
		}
		if key.GetNotAfterUnix() > 0 {
			value := time.Unix(key.GetNotAfterUnix(), 0)
			row.NotAfter = &value
		}
		rows = append(rows, row)
	}
	err = k.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []model.IdentityTokenKey
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		for _, row := range existing {
			if !seen[row.KeyID] {
				if err := tx.Delete(&model.IdentityTokenKey{}, "kid = ?", row.KeyID).Error; err != nil {
					return err
				}
			}
		}
		for _, row := range rows {
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("store identity token keys: %w", err)
	}
	return k.Reload(ctx)
}

// Run refreshes every interval and when kicked, until ctx ends. connect
// returns the current connection to the identity module; failures keep the
// last good keys and go to report.
func (k *Keys) Run(ctx context.Context, interval time.Duration, connect func() (grpc.ClientConnInterface, error), report func(error)) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-k.kick:
		}
		conn, err := connect()
		if err == nil {
			callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = k.Refresh(callCtx, conn)
			cancel()
		}
		if err != nil && ctx.Err() == nil && report != nil {
			report(err)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(interval)
	}
}
