// Package keystore keeps the identity signing keys in a SQL table, private
// halves sealed under the KEK, so every replica of the identity service uses
// the same keys. Rotation runs under a lock, so replicas never race.
//
// The table (created by the product's migrations) is:
//
//	id VARCHAR(64) PRIMARY KEY, state VARCHAR(16) NOT NULL,
//	public_key TEXT NOT NULL, sealed_private_key TEXT NOT NULL,
//	created_at BIGINT NOT NULL, activated_at BIGINT NOT NULL, retired_at BIGINT NOT NULL
//
// with times in Unix seconds and 0 for "not yet".
package keystore

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"gorm.io/gorm"
)

// Store reads and rotates the keys in Table.
type Store struct {
	DB     *gorm.DB
	Table  string
	Sealer *signingkey.Sealer
	Policy signingkey.Policy
	// Now defaults to time.Now.
	Now func() time.Time
	// Generate defaults to signingkey.Generate.
	Generate func(time.Time) (signingkey.Key, error)
}

type row struct {
	ID               string `gorm:"column:id;primaryKey"`
	State            string `gorm:"column:state"`
	PublicKey        string `gorm:"column:public_key"`
	SealedPrivateKey string `gorm:"column:sealed_private_key"`
	CreatedAt        int64  `gorm:"column:created_at"`
	ActivatedAt      int64  `gorm:"column:activated_at"`
	RetiredAt        int64  `gorm:"column:retired_at"`
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) check() error {
	if s == nil || s.DB == nil || s.Table == "" || s.Sealer == nil {
		return errors.New("identity key store is not configured")
	}
	return s.Policy.Validate()
}

// Load returns every key with its private half opened.
func (s *Store) Load(ctx context.Context) ([]signingkey.Key, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	return s.load(s.DB.WithContext(ctx))
}

func (s *Store) load(db *gorm.DB) ([]signingkey.Key, error) {
	var rows []row
	if err := db.Table(s.Table).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load signing keys: %w", err)
	}
	keys := make([]signingkey.Key, 0, len(rows))
	for _, r := range rows {
		public, err := base64.StdEncoding.DecodeString(r.PublicKey)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("signing key %q has a malformed public key", r.ID)
		}
		key := signingkey.Key{
			ID: r.ID, State: signingkey.State(r.State), PublicKey: public,
			CreatedAt: unix(r.CreatedAt), ActivatedAt: unix(r.ActivatedAt), RetiredAt: unix(r.RetiredAt),
		}
		if key.State != signingkey.StateRevoked {
			private, err := s.Sealer.Open(r.ID, r.SealedPrivateKey)
			if err != nil {
				return nil, err
			}
			key.PrivateKey = private
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// Advance applies the rotation due now and returns the resulting keys.
func (s *Store) Advance(ctx context.Context) ([]signingkey.Key, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	generate := s.Generate
	if generate == nil {
		generate = signingkey.Generate
	}
	var result []signingkey.Key
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.lock(tx); err != nil {
			return err
		}
		current, err := s.load(tx)
		if err != nil {
			return err
		}
		next, changed, err := s.Policy.Advance(current, s.now(), generate)
		if err != nil {
			return err
		}
		if changed {
			if err := s.save(tx, current, next); err != nil {
				return err
			}
		}
		result = next
		return nil
	})
	return result, err
}

// Revoke marks a key revoked and forgets its private half.
func (s *Store) Revoke(ctx context.Context, id string) error {
	if err := s.check(); err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.lock(tx); err != nil {
			return err
		}
		result := tx.Table(s.Table).Where("id = ?", id).Updates(map[string]any{"state": string(signingkey.StateRevoked), "sealed_private_key": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("signing key %q not found", id)
		}
		return nil
	})
}

// lock serializes rotation across replicas: an advisory lock on PostgreSQL;
// SQLite serializes writing transactions itself.
func (s *Store) lock(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "identity-keystore:"+s.Table).Error
}

func (s *Store) save(tx *gorm.DB, before, after []signingkey.Key) error {
	remaining := map[string]bool{}
	for _, key := range after {
		remaining[key.ID] = true
	}
	for _, key := range before {
		if !remaining[key.ID] {
			if err := tx.Table(s.Table).Where("id = ?", key.ID).Delete(&row{}).Error; err != nil {
				return err
			}
		}
	}
	existing := map[string]signingkey.Key{}
	for _, key := range before {
		existing[key.ID] = key
	}
	for _, key := range after {
		old, found := existing[key.ID]
		if !found {
			sealed, err := s.Sealer.Seal(key.ID, key.PrivateKey)
			if err != nil {
				return err
			}
			if err := tx.Table(s.Table).Create(&row{
				ID: key.ID, State: string(key.State), PublicKey: base64.StdEncoding.EncodeToString(key.PublicKey),
				SealedPrivateKey: sealed, CreatedAt: seconds(key.CreatedAt), ActivatedAt: seconds(key.ActivatedAt), RetiredAt: seconds(key.RetiredAt),
			}).Error; err != nil {
				return err
			}
			continue
		}
		if old.State != key.State || !old.ActivatedAt.Equal(key.ActivatedAt) || !old.RetiredAt.Equal(key.RetiredAt) {
			if err := tx.Table(s.Table).Where("id = ?", key.ID).Updates(map[string]any{
				"state": string(key.State), "activated_at": seconds(key.ActivatedAt), "retired_at": seconds(key.RetiredAt),
			}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func unix(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(value, 0)
}

func seconds(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
