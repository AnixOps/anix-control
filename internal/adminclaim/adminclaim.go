// Package adminclaim is the first administrator's one-time claim link
// (docs/design/single-binary/README.md, section 3.7, N10).
//
// A fresh panel has no administrator and no password anywhere: the operator
// runs `anix-control admin claim-link` on the machine, which stores the
// SHA-256 of a new token in v5_control_meta (key admin_claim) and prints
// https://<domain>/claim#<token>. The token sits in the URL fragment, so a
// browser never sends it in a request line or a Referer; the claim page
// posts it in the body of POST /api/v5/claim with the administrator's email
// and password. The claim creates the administrator and deletes the token
// in one transaction. Once an administrator exists nothing can be claimed
// and the command refuses.
//
// The token is never stored, logged or answered: only its hash and its
// expiry are kept.
package adminclaim

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
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// TokenPrefix starts every claim token.
	TokenPrefix = "anixclaim_"
	// MetaKey is the v5_control_meta key that holds the pending claim.
	MetaKey = "admin_claim"
	// DefaultTTL is how long a claim link works.
	DefaultTTL = 24 * time.Hour
	// MaxTTL is the longest lifetime the command accepts.
	MaxTTL = DefaultTTL
	// MinTTL is the shortest.
	MinTTL = time.Minute

	tokenBytes = 32
	// tokenLength is the prefix and 32 bytes in unpadded base64url.
	tokenLength = len(TokenPrefix) + 43

	// MinPasswordLength is the first administrator's shortest password, in
	// characters (not bytes): the panel's owner needs more than the 6 an
	// ordinary account does. MaxPasswordBytes is what bcrypt reads.
	MinPasswordLength = 12
	MaxPasswordBytes  = 72
)

// Errors of the claim and of issuing a link.
var (
	// ErrAdministratorExists: the panel has an administrator, so nothing
	// can be claimed (the API answers 404, the command exits 1).
	ErrAdministratorExists = errors.New("the panel already has an administrator")
	// ErrInvalidToken: no pending claim has this token (a wrong or
	// mistyped link, or one a newer link replaced).
	ErrInvalidToken = errors.New("the claim link is not valid")
	// ErrExpired: the token is the pending claim's, but it expired.
	ErrExpired = errors.New("the claim link has expired")
	// ErrInvalidEmail: not an address the sign-in accepts.
	ErrInvalidEmail = errors.New("the email is not valid")
	// ErrWeakPassword and ErrPasswordTooLong break the password rule.
	ErrWeakPassword    = fmt.Errorf("the password must have at least %d characters", MinPasswordLength)
	ErrPasswordTooLong = fmt.Errorf("the password must be at most %d bytes", MaxPasswordBytes)
	// ErrEmailTaken: an account (not an administrator) already has the email.
	ErrEmailTaken = errors.New("an account with this email already exists")
	// ErrIdentityUnavailable: identity holds the credentials and could not
	// be reached; nothing was changed and the link still works.
	ErrIdentityUnavailable = errors.New("the identity module is not reachable")
	// ErrInvalidTTL: a lifetime outside MinTTL..MaxTTL.
	ErrInvalidTTL = fmt.Errorf("the link lifetime must be between %s and %s", MinTTL, MaxTTL)
)

// pending is the value of the admin_claim row: the token's hash and when it
// expires, never the token.
type pending struct {
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// NewToken returns a claim token: TokenPrefix and 32 bytes from random in
// unpadded base64url (256 bits).
func NewToken(random io.Reader) (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := io.ReadFull(random, buf); err != nil {
		return "", fmt.Errorf("claim token: %w", err)
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// WellFormed reports whether token has the shape NewToken gives.
func WellFormed(token string) bool {
	if len(token) != tokenLength || !strings.HasPrefix(token, TokenPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token[len(TokenPrefix):])
	return err == nil && len(decoded) == tokenBytes
}

// Hash is the stored form of a token: its SHA-256 in hex.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HasAdministrator reports whether any account is an administrator. The
// kernel's v2_user is the source in every identity authority state: while
// identity holds the credentials it projects is_admin into it. A banned
// administrator still counts: the link is only for a panel nobody owns yet.
func HasAdministrator(ctx context.Context, db *gorm.DB) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.User{}).Where("is_admin = ?", 1).Count(&count).Error; err != nil {
		return false, fmt.Errorf("look for an administrator: %w", err)
	}
	return count > 0, nil
}

// Issued is a new claim link's token and expiry. The token exists only
// here: print it and forget it.
type Issued struct {
	Token     string
	ExpiresAt time.Time
}

// Issue makes a new claim token valid for ttl, replacing (and so
// invalidating) the previous one. It refuses with ErrAdministratorExists
// when the panel has an administrator.
func Issue(ctx context.Context, db *gorm.DB, ttl time.Duration, now time.Time) (Issued, error) {
	if ttl < MinTTL || ttl > MaxTTL {
		return Issued{}, ErrInvalidTTL
	}
	token, err := NewToken(rand.Reader)
	if err != nil {
		return Issued{}, err
	}
	now = now.UTC().Truncate(time.Second)
	record := pending{SHA256: Hash(token), ExpiresAt: now.Add(ttl), CreatedAt: now}
	value, err := json.Marshal(record)
	if err != nil {
		return Issued{}, err
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exists, err := HasAdministrator(ctx, tx)
		if err != nil {
			return err
		}
		if exists {
			return ErrAdministratorExists
		}
		entry := migrations.MetaEntry{Key: MetaKey, Value: string(value), UpdatedAt: now}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&entry).Error
	})
	if err != nil {
		return Issued{}, err
	}
	return Issued{Token: token, ExpiresAt: record.ExpiresAt}, nil
}

// Claimable reports whether the panel can be claimed: it has no
// administrator. GET /api/v5/claim/status answers it and nothing else, so
// the page tells an expired link from a used one only after a POST with the
// token.
func Claimable(ctx context.Context, db *gorm.DB) (bool, error) {
	exists, err := HasAdministrator(ctx, db)
	return !exists, err
}

// loadPending reads the pending claim; nil when there is none.
func loadPending(tx *gorm.DB) (*pending, string, error) {
	var entries []migrations.MetaEntry
	if err := tx.Where("key = ?", MetaKey).Limit(1).Find(&entries).Error; err != nil {
		return nil, "", fmt.Errorf("read the claim: %w", err)
	}
	if len(entries) == 0 {
		return nil, "", nil
	}
	var record pending
	if err := json.Unmarshal([]byte(entries[0].Value), &record); err != nil || record.SHA256 == "" {
		// An unreadable row matches no token; issuing a new link replaces it.
		return nil, "", nil
	}
	return &record, entries[0].Value, nil
}

// check verifies token against the pending claim: ErrInvalidToken when it
// is not the pending one, ErrExpired when it is but expired. It returns the
// stored value, which the claim deletes only if it is still the same.
func check(tx *gorm.DB, token string, now time.Time) (string, error) {
	record, stored, err := loadPending(tx)
	if err != nil {
		return "", err
	}
	// Hash first and compare in constant time even for a malformed token
	// or no pending claim, so the answer's timing says nothing.
	want := Hash(token)
	have := strings.Repeat("0", len(want))
	if record != nil && len(record.SHA256) == len(want) {
		have = record.SHA256
	}
	matches := subtle.ConstantTimeCompare([]byte(want), []byte(have)) == 1
	if record == nil || !matches || !WellFormed(token) {
		return "", ErrInvalidToken
	}
	if !now.Before(record.ExpiresAt) {
		return "", ErrExpired
	}
	return stored, nil
}

// emailRule is the sign-in's check of an address (model.LoginRequest), so
// the administrator can sign in with what they claimed with.
type emailRule struct {
	Email string `binding:"required,email,max=255"`
}

// NormalizeEmail trims email and checks it as the sign-in does.
func NormalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if err := binding.Validator.ValidateStruct(&emailRule{Email: email}); err != nil {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// CheckPassword applies the first administrator's password rule: at least
// MinPasswordLength characters and at most MaxPasswordBytes bytes.
func CheckPassword(password string) error {
	switch {
	case utf8.RuneCountInString(password) < MinPasswordLength:
		return ErrWeakPassword
	case len(password) > MaxPasswordBytes:
		return ErrPasswordTooLong
	}
	return nil
}
