// Package adminapitoken issues and verifies administrators' personal access
// tokens: long-lived credentials for automation on the administrator APIs
// (docs/reference/admin-api-tokens.md).
//
// A token is "anixadm_" and 43 characters of URL-safe base64 for 32 random
// bytes. The kernel stores only its SHA-256 (v4_kernel_admin_api_token); the
// token is shown once, when it is created. A request names it in
// "Authorization: Bearer <token>" and nowhere else, and the lookup is by
// hash: an attacker cannot learn anything from the comparison without the
// token's preimage.
//
// A token carries no rights of its own beyond a scope (read or admin). The
// owner's rights are read from v2_user on every use, so banning, demoting or
// deleting an administrator ends their tokens at once, and a token used while
// its owner is not an administrator is revoked for good (it does not come
// back when the owner is restored).
package adminapitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// Prefix marks an admin API token. It lets secret scanners and humans
	// recognise one, and the middleware tell it from a JWT.
	Prefix = "anixadm_"
	// secretBytes of randomness: 256 bits.
	secretBytes = 32
	// encodedLength is the length of 32 bytes in unpadded base64.
	encodedLength = 43
	// TokenLength is the length of every token.
	TokenLength = len(Prefix) + encodedLength
	// MaxNameLength bounds a token's name.
	MaxNameLength = 100
	// MaxActivePerUser bounds the unrevoked, unexpired tokens of one owner.
	MaxActivePerUser = 25
	// MaxLifetime bounds an expiry that is set; no expiry stays allowed.
	MaxLifetime = 2 * 365 * 24 * time.Hour
	// ManagementPath is the prefix of the routes that manage tokens. They
	// need a signed-in session: no API token reaches them.
	ManagementPath = "/api/v4/kernel/api-tokens"
	// lastUsedInterval is how often a token's last-used time is written.
	lastUsedInterval = 5 * time.Minute
	// deniedAuditInterval is how often one subject's denials are audited.
	deniedAuditInterval = time.Minute
)

// Request context keys the authentication middleware sets. They are set on
// the server side only; a client cannot send them.
const (
	ContextKeyAuthMethod = "auth_method"
	ContextKeyTokenID    = "api_token_id"
	ContextKeyTokenScope = "api_token_scope"
	// AuthMethodAPIToken is the value of ContextKeyAuthMethod for a request
	// an admin API token authenticated.
	AuthMethodAPIToken = "api_token"
)

// Audit entries (v2_operation_log). Entries carry a token's id, name, scope,
// hint and expiry, never the token or its hash.
const (
	AuditModule       = "admin_api_token"
	AuditActionCreate = "admin_api_token_create"
	// AuditActionCreateDenied is a creation refused for its re-authentication.
	AuditActionCreateDenied = "admin_api_token_create_denied"
	AuditActionRevoke       = "admin_api_token_revoke"
	AuditActionUse          = "admin_api_token_use"
	AuditActionUseDenied    = "admin_api_token_use_denied"
	auditTargetType         = "admin_api_token"
)

// Why an authentication or a use was refused.
var (
	// ErrInvalid covers a malformed, unknown or unusable token.
	ErrInvalid = errors.New("invalid admin API token")
	// ErrRevoked is a revoked token.
	ErrRevoked = errors.New("admin API token revoked")
	// ErrExpired is a token past its expiry.
	ErrExpired = errors.New("admin API token expired")
	// ErrOwnerDenied is a token whose owner is banned, demoted or deleted.
	ErrOwnerDenied = errors.New("admin API token owner is not an active administrator")
	// ErrScopeDenied is a request the token's scope does not allow.
	ErrScopeDenied = errors.New("admin API token scope does not allow this request")
	// ErrInteractiveOnly is a request that needs a signed-in session.
	ErrInteractiveOnly = errors.New("this request needs a signed-in session, not an API token")

	// ErrNotFound is a token that does not exist or is not the caller's.
	ErrNotFound = errors.New("admin API token not found")
	// ErrInvalidRequest wraps every refused creation.
	ErrInvalidRequest = errors.New("invalid admin API token request")
	// ErrTooMany is an owner at MaxActivePerUser.
	ErrTooMany = errors.New("too many active admin API tokens")
	// ErrOwnerNotAdmin is a creation for an account that is not an active
	// administrator.
	ErrOwnerNotAdmin = errors.New("only an active administrator may hold admin API tokens")
)

// Denial is a refused authentication: Err is one of the Err values above,
// the other fields say which token and owner it concerned when they are
// known.
type Denial struct {
	Err     error
	TokenID string
	UserID  uint
	// Detail is a short safe reason for the audit entry.
	Detail string
}

func (d *Denial) Error() string { return d.Err.Error() }
func (d *Denial) Unwrap() error { return d.Err }

// Principal is an authenticated token and its owner.
type Principal struct {
	TokenID string
	UserID  uint
	Email   string
	Scope   string
	Name    string
}

// Generate returns a new token, its stored hash and its hint.
func Generate() (token, hash, hint string, err error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", err
	}
	token = Prefix + base64.RawURLEncoding.EncodeToString(raw)
	return token, Hash(token), token[len(token)-4:], nil
}

// Hash is the stored form of a token: its hex SHA-256.
func Hash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// WellFormed reports whether s has the shape of a token, so a request that
// cannot be one is refused without a lookup.
func WellFormed(s string) bool {
	if len(s) != TokenLength || !strings.HasPrefix(s, Prefix) {
		return false
	}
	for _, r := range s[len(Prefix):] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// LooksLikeToken reports whether s starts like a token, however it is
// shaped; the middleware refuses these in a URL.
func LooksLikeToken(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), Prefix) }

// ValidScope reports whether scope is one the kernel knows.
func ValidScope(scope string) bool {
	return scope == model.AdminAPITokenScopeRead || scope == model.AdminAPITokenScopeAdmin
}

// Service issues, lists, revokes and authenticates admin API tokens.
type Service struct {
	db  *gorm.DB
	now func() time.Time

	mu         sync.Mutex
	lastUsed   map[string]time.Time
	lastDenied map[string]time.Time
}

// New returns a service over db.
func New(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now, lastUsed: map[string]time.Time{}, lastDenied: map[string]time.Time{}}
}

// WithClock replaces the clock; for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// CreateInput describes a new token.
type CreateInput struct {
	UserID uint
	Name   string
	Scope  string
	// ExpiresAt ends the token; nil never (allowed, not advised).
	ExpiresAt *time.Time
	// Actor and IP describe the caller in the audit entry.
	Actor string
	IP    string
}

// Create issues a token for an active administrator and returns it: the only
// time it exists in clear. The audit entry is written with the row.
func (s *Service) Create(ctx context.Context, input CreateInput) (string, model.AdminAPIToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > MaxNameLength || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", model.AdminAPIToken{}, fmt.Errorf("%w: a name of 1 to %d printable characters is required", ErrInvalidRequest, MaxNameLength)
	}
	if !ValidScope(input.Scope) {
		return "", model.AdminAPIToken{}, fmt.Errorf("%w: scope must be %q or %q", ErrInvalidRequest, model.AdminAPITokenScopeRead, model.AdminAPITokenScopeAdmin)
	}
	now := s.now().UTC()
	var expires *time.Time
	if input.ExpiresAt != nil {
		at := input.ExpiresAt.UTC()
		if !at.After(now) || at.After(now.Add(MaxLifetime)) {
			return "", model.AdminAPIToken{}, fmt.Errorf("%w: the expiry must be in the future and within %d days", ErrInvalidRequest, int(MaxLifetime.Hours()/24))
		}
		expires = &at
	}
	token, hash, hint, err := Generate()
	if err != nil {
		return "", model.AdminAPIToken{}, err
	}
	row := model.AdminAPIToken{
		ID: uuid.NewString(), UserID: input.UserID, Name: name, TokenHash: hash, Hint: hint, Scope: input.Scope,
		ExpiresAt: expires, CreatedIP: truncate(input.IP, 45), CreatedAt: now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner model.User
		if err := tx.Select("id", "is_admin", "banned").Where("id = ?", input.UserID).Take(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOwnerNotAdmin
			}
			return err
		}
		if owner.IsAdmin != 1 || owner.Banned != 0 {
			return ErrOwnerNotAdmin
		}
		var active int64
		if err := tx.Model(&model.AdminAPIToken{}).
			Where("user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", input.UserID, now).
			Count(&active).Error; err != nil {
			return err
		}
		if active >= MaxActivePerUser {
			return ErrTooMany
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			UserID: input.UserID, Actor: input.Actor, Action: AuditActionCreate, IP: input.IP, Status: 1,
			Content: tokenContent(row, nil),
		})
	})
	if err != nil {
		return "", model.AdminAPIToken{}, err
	}
	return token, row, nil
}

// ListFilter selects tokens.
type ListFilter struct {
	// UserID limits the list to one owner; nil lists every owner's tokens.
	UserID *uint
	// IncludeInactive also lists revoked and expired tokens.
	IncludeInactive bool
}

// List returns tokens, newest first. A token's secret is never in a row.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]model.AdminAPIToken, error) {
	query := s.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(500)
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if !filter.IncludeInactive {
		query = query.Where("revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", s.now().UTC())
	}
	rows := []model.AdminAPIToken{}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Actor is the administrator who revokes a token.
type Actor struct {
	UserID uint
	Name   string
	IP     string
	// Super lets the actor revoke other administrators' tokens.
	Super bool
}

// Revoke ends a token. An administrator revokes their own; a super
// administrator revokes anyone's. A token that is not the actor's, or does
// not exist, is ErrNotFound. Revoking a revoked token changes nothing and
// reports changed false.
func (s *Service) Revoke(ctx context.Context, id string, actor Actor) (model.AdminAPIToken, bool, error) {
	var row model.AdminAPIToken
	changed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.UserID != actor.UserID && !actor.Super {
			return ErrNotFound
		}
		if row.RevokedAt != nil {
			return nil
		}
		now := s.now().UTC()
		reason := "owner_revoked"
		if row.UserID != actor.UserID {
			reason = "admin_revoked"
		}
		result := tx.Model(&model.AdminAPIToken{}).Where("id = ? AND revoked_at IS NULL", id).
			Updates(map[string]any{"revoked_at": now, "revoke_reason": reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		row.RevokedAt, row.RevokeReason, changed = &now, reason, true
		return writeAudit(tx, auditEntry{
			UserID: actor.UserID, Actor: actor.Name, Action: AuditActionRevoke, IP: actor.IP, Status: 1,
			Content: tokenContent(row, map[string]any{"owner_id": row.UserID, "reason": reason}),
		})
	})
	if err != nil {
		return model.AdminAPIToken{}, false, err
	}
	return row, changed, nil
}

// Authenticate resolves a presented token to its owner and scope. A refusal
// is a *Denial (use errors.Is with the Err values). ip is recorded as the
// token's last use, at most every few minutes.
func (s *Service) Authenticate(ctx context.Context, presented, ip string) (*Principal, error) {
	if !WellFormed(presented) {
		return nil, &Denial{Err: ErrInvalid, Detail: "malformed"}
	}
	hash := Hash(presented)
	db := s.db.WithContext(ctx)
	var row model.AdminAPIToken
	if err := db.Where("token_hash = ?", hash).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &Denial{Err: ErrInvalid, Detail: "unknown"}
		}
		return nil, err
	}
	// The lookup is by hash; compare once more in constant time so no
	// later change to it can turn into a comparison that leaks.
	if subtle.ConstantTimeCompare([]byte(row.TokenHash), []byte(hash)) != 1 {
		return nil, &Denial{Err: ErrInvalid, Detail: "unknown"}
	}
	denied := func(err error, detail string) error {
		return &Denial{Err: err, TokenID: row.ID, UserID: row.UserID, Detail: detail}
	}
	now := s.now().UTC()
	if row.RevokedAt != nil {
		return nil, denied(ErrRevoked, "revoked")
	}
	if row.ExpiresAt != nil && !now.Before(*row.ExpiresAt) {
		return nil, denied(ErrExpired, "expired")
	}
	if !ValidScope(row.Scope) {
		return nil, denied(ErrInvalid, "unknown scope")
	}
	var owner model.User
	err := db.Select("id", "email", "is_admin", "banned").Where("id = ?", row.UserID).Take(&owner).Error
	detail := ""
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		detail = "owner deleted"
	case err != nil:
		return nil, err
	case owner.Banned != 0:
		detail = "owner banned"
	case owner.IsAdmin != 1:
		detail = "owner not an administrator"
	}
	if detail != "" {
		// A token whose owner stopped being an administrator ends for good:
		// restoring the owner later does not bring back credentials that
		// were out there while they were not trusted.
		if err := db.Model(&model.AdminAPIToken{}).Where("id = ? AND revoked_at IS NULL", row.ID).
			Updates(map[string]any{"revoked_at": now, "revoke_reason": "owner_not_admin"}).Error; err != nil {
			return nil, err
		}
		return nil, denied(ErrOwnerDenied, detail)
	}
	s.touch(ctx, row.ID, ip, now)
	return &Principal{TokenID: row.ID, UserID: row.UserID, Email: owner.Email, Scope: row.Scope, Name: row.Name}, nil
}

// touch records a use at most every lastUsedInterval per token, in this
// process and, with the conditional update, across processes.
func (s *Service) touch(ctx context.Context, id, ip string, now time.Time) {
	s.mu.Lock()
	if last, ok := s.lastUsed[id]; ok && now.Sub(last) < lastUsedInterval {
		s.mu.Unlock()
		return
	}
	if len(s.lastUsed) > 4096 {
		s.lastUsed = map[string]time.Time{}
	}
	s.lastUsed[id] = now
	s.mu.Unlock()
	// A failed write only loses a timestamp.
	_ = s.db.WithContext(ctx).Model(&model.AdminAPIToken{}).
		Where("id = ? AND (last_used_at IS NULL OR last_used_at < ?)", id, now.Add(-lastUsedInterval)).
		Updates(map[string]any{"last_used_at": now, "last_used_ip": truncate(ip, 45)}).Error
}

// Event is a request an API token made or was refused.
type Event struct {
	TokenID string
	UserID  uint
	Actor   string
	IP      string
	Method  string
	Path    string
	// Reason is a short safe reason: a Denial's Detail or the error.
	Reason string
	Status int
}

// RecordDenied audits a refused use: an unusable token (Reason says why), a
// scope the request is outside of, or a request that needs a session. The
// same subject is audited at most once a minute, so a script that keeps
// failing cannot fill the log; the log line always says which token and
// owner, never the token.
func (s *Service) RecordDenied(ctx context.Context, event Event) {
	key := event.TokenID + "|" + event.Reason
	if event.TokenID == "" {
		key = "ip:" + event.IP + "|" + event.Reason
	}
	now := s.now()
	s.mu.Lock()
	if last, ok := s.lastDenied[key]; ok && now.Sub(last) < deniedAuditInterval {
		s.mu.Unlock()
		return
	}
	if len(s.lastDenied) > 4096 {
		s.lastDenied = map[string]time.Time{}
	}
	s.lastDenied[key] = now
	s.mu.Unlock()
	s.record(ctx, AuditActionUseDenied, event, 2)
}

// RecordCreateDenied audits a creation refused because the re-authentication
// failed or was missing, at most once a minute per administrator. The entry
// carries no password and no code.
func (s *Service) RecordCreateDenied(ctx context.Context, event Event) {
	key := fmt.Sprintf("create|%d|%s", event.UserID, event.Reason)
	now := s.now()
	s.mu.Lock()
	if last, ok := s.lastDenied[key]; ok && now.Sub(last) < deniedAuditInterval {
		s.mu.Unlock()
		return
	}
	if len(s.lastDenied) > 4096 {
		s.lastDenied = map[string]time.Time{}
	}
	s.lastDenied[key] = now
	s.mu.Unlock()
	s.record(ctx, AuditActionCreateDenied, event, 2)
}

// RecordUse audits a write an API token made, so the audit log can tell a
// token's requests from the owner's own. The audit log of the administrator
// APIs records the request itself; this entry names the token.
func (s *Service) RecordUse(ctx context.Context, event Event) {
	s.record(ctx, AuditActionUse, event, 1)
}

func (s *Service) record(ctx context.Context, action string, event Event, status int) {
	content := map[string]any{"token_id": event.TokenID, "method": event.Method, "path": event.Path, "reason": event.Reason}
	if event.Status != 0 {
		content["status_code"] = event.Status
	}
	if event.UserID != 0 {
		content["owner_id"] = event.UserID
	}
	// An audit entry that cannot be written must not change the answer.
	_ = writeAudit(s.db.WithContext(ctx), auditEntry{
		UserID: event.UserID, Actor: event.Actor, Action: action, IP: event.IP, Status: status, Content: content,
	})
}

type auditEntry struct {
	UserID  uint
	Actor   string
	Action  string
	IP      string
	Status  int
	Content map[string]any
}

// writeAudit records an event in the operation log.
func writeAudit(tx *gorm.DB, entry auditEntry) error {
	content, err := json.Marshal(entry.Content)
	if err != nil {
		return err
	}
	var userID *uint
	if entry.UserID != 0 {
		id := entry.UserID
		userID = &id
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(entry.Actor, 100), Action: entry.Action, Module: AuditModule,
		TargetType: auditTargetType, Content: string(content), IP: truncate(entry.IP, 45), Status: entry.Status,
	}).Error
}

// tokenContent is what an audit entry says about a token.
func tokenContent(row model.AdminAPIToken, extra map[string]any) map[string]any {
	content := map[string]any{"token_id": row.ID, "name": row.Name, "scope": row.Scope, "hint": row.Hint, "owner_id": row.UserID}
	if row.ExpiresAt != nil {
		content["expires_at"] = row.ExpiresAt.UTC().Format(time.RFC3339)
	}
	for key, value := range extra {
		content[key] = value
	}
	return content
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
