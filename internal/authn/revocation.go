package authn

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Revocation makes tokens stop working. It mirrors
// KernelIdentity.PublishRevocation:
//   - UserID with NotBefore and/or TokenVersion revokes that user's tokens
//     issued before NotBefore or carrying a smaller tv claim;
//   - SessionID revokes one session until SessionExpiresAt.
type Revocation struct {
	UserID           uint
	TokenVersion     uint64
	NotBefore        time.Time
	SessionID        string
	SessionExpiresAt time.Time
	Reason           string
}

// UserRevocation revokes every token of a user issued before now.
func UserRevocation(userID uint, reason string) Revocation {
	return Revocation{UserID: userID, NotBefore: time.Now(), Reason: reason}
}

func (r Revocation) validate() error {
	if r.UserID == 0 {
		return errors.New("revocation needs a user")
	}
	if r.SessionID == "" && r.NotBefore.IsZero() && r.TokenVersion == 0 {
		return errors.New("revocation needs not_before, a token version or a session")
	}
	if r.SessionID != "" && (len(r.SessionID) > 64 || r.SessionExpiresAt.IsZero()) {
		return errors.New("session revocation needs a session id of at most 64 bytes and its expiry")
	}
	return nil
}

// Write records a revocation with db, which may be a transaction, so that
// the change that caused it and the revocation commit together. Bounds only
// move forward. Call Store.Remember after the commit to apply it at once.
func Write(db *gorm.DB, r Revocation) error {
	if err := r.validate(); err != nil {
		return err
	}
	now := time.Now()
	if !r.NotBefore.IsZero() || r.TokenVersion > 0 {
		row := model.IdentityRevocation{
			UserID: r.UserID, TokenVersion: r.TokenVersion, NotBefore: r.NotBefore, Reason: r.Reason, UpdatedAt: now,
		}
		if row.NotBefore.IsZero() {
			row.NotBefore = time.Unix(0, 0)
		}
		var existing model.IdentityRevocation
		err := db.Where("user_id = ?", r.UserID).Take(&existing).Error
		switch {
		case err == nil:
			if existing.NotBefore.After(row.NotBefore) {
				row.NotBefore = existing.NotBefore
			}
			if existing.TokenVersion > row.TokenVersion {
				row.TokenVersion = existing.TokenVersion
			}
			if err := db.Model(&model.IdentityRevocation{}).Where("user_id = ?", r.UserID).Updates(map[string]any{
				"token_version": row.TokenVersion, "not_before": row.NotBefore, "reason": row.Reason, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := db.Create(&row).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	if r.SessionID != "" {
		row := model.IdentitySessionRevocation{
			SessionID: r.SessionID, UserID: r.UserID, ExpiresAt: r.SessionExpiresAt, Reason: r.Reason, CreatedAt: now,
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

type userBounds struct {
	notBefore    time.Time
	tokenVersion uint64
}

// Store answers revocation checks from memory. It loads the tables at start,
// applies revocations this process makes at once (Remember), and reloads
// periodically to pick up revocations written elsewhere.
type Store struct {
	db *gorm.DB
	// maxTokenLifetime bounds how long a user revocation matters: after it,
	// every token issued before NotBefore has expired anyway.
	maxTokenLifetime time.Duration
	now              func() time.Time

	mu       sync.RWMutex
	users    map[uint]userBounds
	sessions map[string]time.Time
	// recent holds revocations remembered while a reload may be reading the
	// tables, so the reload's swap cannot drop them.
	recent []remembered
}

type remembered struct {
	at         time.Time
	revocation Revocation
}

// NewStore returns a store over db. maxTokenLifetime is the longest token
// lifetime the kernel accepts (jwt.expire plus leeway).
func NewStore(db *gorm.DB, maxTokenLifetime time.Duration) *Store {
	return &Store{
		db: db, maxTokenLifetime: maxTokenLifetime, now: time.Now,
		users: map[uint]userBounds{}, sessions: map[string]time.Time{},
	}
}

// Revoked reports whether claims belong to a revoked token.
func (s *Store) Revoked(claims *utils.Claims) bool {
	if s == nil || claims == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if claims.SessionID != "" {
		if expires, ok := s.sessions[claims.SessionID]; ok && s.now().Before(expires) {
			return true
		}
	}
	bounds, ok := s.users[claims.UserID]
	if !ok {
		return false
	}
	if claims.TokenVersion < bounds.tokenVersion {
		return true
	}
	// iat has one-second precision, so a token issued in the revocation's
	// second is revoked too; one issued in a later second is valid.
	return claims.IssuedAt == nil || !claims.IssuedAt.After(bounds.notBefore.Truncate(time.Second))
}

// Publish writes a revocation and applies it at once.
func (s *Store) Publish(ctx context.Context, r Revocation) error {
	if err := Write(s.db.WithContext(ctx), r); err != nil {
		return err
	}
	s.Remember(r)
	return nil
}

// Remember applies a revocation that was written and committed.
func (s *Store) Remember(r Revocation) {
	if s == nil || r.validate() != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent = append(s.recent, remembered{at: time.Now(), revocation: r})
	s.applyLocked(r)
}

func (s *Store) applyLocked(r Revocation) {
	if !r.NotBefore.IsZero() || r.TokenVersion > 0 {
		bounds := s.users[r.UserID]
		if r.NotBefore.After(bounds.notBefore) {
			bounds.notBefore = r.NotBefore
		}
		if r.TokenVersion > bounds.tokenVersion {
			bounds.tokenVersion = r.TokenVersion
		}
		s.users[r.UserID] = bounds
	}
	if r.SessionID != "" && r.SessionExpiresAt.After(s.sessions[r.SessionID]) {
		s.sessions[r.SessionID] = r.SessionExpiresAt
	}
}

// Reload replaces the cache with the tables, after deleting rows that no
// longer revoke any unexpired token.
func (s *Store) Reload(ctx context.Context) error {
	// Anything remembered before this point was committed before the reads
	// below, so they see it.
	started := time.Now()
	now := s.now()
	db := s.db.WithContext(ctx)
	if err := db.Where("token_version = 0 AND not_before < ?", now.Add(-s.maxTokenLifetime)).
		Delete(&model.IdentityRevocation{}).Error; err != nil {
		return err
	}
	if err := db.Where("expires_at < ?", now).Delete(&model.IdentitySessionRevocation{}).Error; err != nil {
		return err
	}
	var userRows []model.IdentityRevocation
	if err := db.Find(&userRows).Error; err != nil {
		return err
	}
	var sessionRows []model.IdentitySessionRevocation
	if err := db.Find(&sessionRows).Error; err != nil {
		return err
	}
	users := make(map[uint]userBounds, len(userRows))
	for _, row := range userRows {
		users[row.UserID] = userBounds{notBefore: row.NotBefore, tokenVersion: row.TokenVersion}
	}
	sessions := make(map[string]time.Time, len(sessionRows))
	for _, row := range sessionRows {
		sessions[row.SessionID] = row.ExpiresAt
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users, s.sessions = users, sessions
	kept := s.recent[:0]
	for _, entry := range s.recent {
		if !entry.at.Before(started) {
			kept = append(kept, entry)
			s.applyLocked(entry.revocation)
		}
	}
	s.recent = kept
	return nil
}

// Run reloads every interval until ctx ends; errors go to report.
func (s *Store) Run(ctx context.Context, interval time.Duration, report func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(ctx); err != nil && ctx.Err() == nil && report != nil {
				report(err)
			}
		}
	}
}
