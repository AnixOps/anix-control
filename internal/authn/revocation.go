package authn

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Revocation makes tokens stop working. It mirrors
// KernelIdentity.PublishRevocation:
//   - UserID with NotBefore revokes that user's kernel tokens (no tv claim)
//     issued up to NotBefore;
//   - with TokenVersion as well, the user's identity tokens (tv claim) stop
//     working when their tv is below TokenVersion, so a token identity issues
//     afterwards works even within the same second; without it, identity
//     tokens issued up to NotBefore stop working too;
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

// ErrInvalidRevocation wraps every reason a revocation is refused.
var ErrInvalidRevocation = errors.New("invalid revocation")

func (r Revocation) validate() error {
	if r.UserID == 0 {
		return fmt.Errorf("%w: it needs a user", ErrInvalidRevocation)
	}
	if r.SessionID == "" && r.NotBefore.IsZero() && r.TokenVersion == 0 {
		return fmt.Errorf("%w: it needs not_before, a token version or a session", ErrInvalidRevocation)
	}
	if r.SessionID != "" && (len(r.SessionID) > 64 || r.SessionExpiresAt.IsZero()) {
		return fmt.Errorf("%w: a session revocation needs a session id of at most 64 bytes and its expiry", ErrInvalidRevocation)
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
		if bound := r.identityNotBefore(); !bound.IsZero() {
			row.IdentityNotBefore = &bound
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
			if existing.IdentityNotBefore != nil && (row.IdentityNotBefore == nil || existing.IdentityNotBefore.After(*row.IdentityNotBefore)) {
				row.IdentityNotBefore = existing.IdentityNotBefore
			}
			if err := db.Model(&model.IdentityRevocation{}).Where("user_id = ?", r.UserID).Updates(map[string]any{
				"token_version": row.TokenVersion, "not_before": row.NotBefore, "identity_not_before": row.IdentityNotBefore,
				"reason": row.Reason, "updated_at": now,
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

// identityNotBefore is the bound a revocation sets for identity tokens: its
// NotBefore when it carries no token version, else none.
func (r Revocation) identityNotBefore() time.Time {
	if r.TokenVersion > 0 {
		return time.Time{}
	}
	return r.NotBefore
}

type userBounds struct {
	// notBefore bounds kernel tokens, identityNotBefore identity tokens.
	notBefore         time.Time
	identityNotBefore time.Time
	tokenVersion      uint64
}

// PruneInterval is how often Reload deletes the rows that no longer revoke
// any unexpired token. Every reload still reads the tables, so a revocation
// written by another process takes effect within the reload interval; a row
// that is expired only has to go eventually, and each DELETE is a write
// transaction of its own, which a reload every few seconds should not take
// (docs/design/single-binary section 3.4).
const PruneInterval = 5 * time.Minute

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
	// lastPrune is when Reload last pruned the tables; guarded by mu.
	lastPrune time.Time
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
	if claims.TokenVersion > 0 {
		// An identity token: its token version decides, exactly.
		if claims.TokenVersion < bounds.tokenVersion {
			return true
		}
		return issuedBy(claims, bounds.identityNotBefore)
	}
	return issuedBy(claims, bounds.notBefore)
}

// issuedBy reports whether a token was issued up to bound. iat has
// one-second precision, so a token issued in the bound's second counts; one
// issued in a later second does not.
func issuedBy(claims *utils.Claims, bound time.Time) bool {
	if bound.IsZero() {
		return false
	}
	return claims.IssuedAt == nil || !claims.IssuedAt.After(bound.Truncate(time.Second))
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
		if identity := r.identityNotBefore(); identity.After(bounds.identityNotBefore) {
			bounds.identityNotBefore = identity
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

// Reload replaces the cache with the tables. The rows that no longer revoke
// any unexpired token are deleted first, at most every PruneInterval and only
// when there are any.
func (s *Store) Reload(ctx context.Context) error {
	// Anything remembered before this point was committed before the reads
	// below, so they see it.
	started := time.Now()
	now := s.now()
	db := s.db.WithContext(ctx)
	if s.claimPrune(now) {
		if err := s.prune(db, now); err != nil {
			s.mu.Lock()
			s.lastPrune = time.Time{}
			s.mu.Unlock()
			return err
		}
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
		bounds := userBounds{notBefore: row.NotBefore, tokenVersion: row.TokenVersion}
		if row.IdentityNotBefore != nil {
			bounds.identityNotBefore = *row.IdentityNotBefore
		}
		users[row.UserID] = bounds
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

// claimPrune reports whether a prune is due at now, and marks it done: the
// first reload prunes, then one per PruneInterval.
func (s *Store) claimPrune(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastPrune.IsZero() && !now.Before(s.lastPrune) && now.Sub(s.lastPrune) < PruneInterval {
		return false
	}
	s.lastPrune = now
	return true
}

// prune deletes the user revocations older than any token lifetime and the
// expired session revocations. A DELETE that matches no row is a write
// transaction all the same, so each is preceded by a read.
func (s *Store) prune(db *gorm.DB, now time.Time) error {
	if err := deleteWhere(db, &model.IdentityRevocation{}, "token_version = 0 AND not_before < ?", now.Add(-s.maxTokenLifetime)); err != nil {
		return err
	}
	return deleteWhere(db, &model.IdentitySessionRevocation{}, "expires_at < ?", now)
}

func deleteWhere(db *gorm.DB, value any, query string, args ...any) error {
	var found []int
	if err := db.Model(value).Select("1").Where(query, args...).Limit(1).Scan(&found).Error; err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	return db.Where(query, args...).Delete(value).Error
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
