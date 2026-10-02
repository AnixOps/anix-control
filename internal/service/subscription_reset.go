package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// UserSubscriptionReset is a user's reset of their own subscription token,
// for UserSubscriptionResetRequestID.
const UserSubscriptionReset = "user_reset_subscribe"

// UserSubscriptionResetRequestID names a user's reset of their own
// subscription token in the subscriber request ledger, so a retried
// request is applied once: identity.user_reset_subscribe:<user>:<digest>,
// a digest of the request's Idempotency-Key (else its request id). The
// identity package's native handler derives the same id.
func UserSubscriptionResetRequestID(userID uint, token string) string {
	return AdminUserResetRequestID(UserSubscriptionReset, userID, token)
}

// SubscriptionResetRateLimit bounds a user's subscription resets: every
// attempt that checks a credential counts, successful or not, and the
// third in an hour locks the user's resets for an hour. identity-platform
// applies the same limit (packages/identity-platform/native).
var SubscriptionResetRateLimit = LoginRateLimitOptions{
	Enabled:      true,
	MaxAttempts:  3,
	Window:       time.Hour,
	Lockout:      time.Hour,
	CleanupAfter: 2*time.Hour + 5*time.Minute,
}

// SubscriptionResetRateLimitKey is a user's key in the reset limiter.
func SubscriptionResetRateLimitKey(userID uint) string {
	return fmt.Sprintf("subscription_reset|%d", userID)
}

var globalSubscriptionResetLimiter = NewLoginRateLimiter()

// GetSubscriptionResetLimiter returns the limiter of users' subscription
// resets, kept apart from the login limiter so that neither's cleanup
// forgets the other's keys early.
func GetSubscriptionResetLimiter() *LoginRateLimiter {
	return globalSubscriptionResetLimiter
}

// The answers of a subscription reset that does not reset; identity-platform
// answers the same messages.
const (
	SubscriptionResetRateLimitedMessage = "too many subscription reset attempts, please try again later"
	SubscriptionResetFailedMessage      = "重置订阅失败"
)

// Step-up errors: the re-authentication a user's own subscription reset
// needs. Their messages are the panel answers.
var (
	ErrStepUpPasswordRequired = errors.New("password required")
	ErrStepUpInvalidPassword  = errors.New("invalid password")
	ErrStepUpMFARequired      = errors.New("mfa code required")
	ErrStepUpInvalidMFACode   = errors.New("invalid mfa code")
)

// VerifyStepUp re-authenticates a signed-in user before a sensitive change:
// with a second factor enabled, a TOTP or recovery (backup) code, checked by
// MFAService.Verify, which consumes a used recovery code; otherwise the
// current password. checked reports whether a credential was checked, which
// counts toward the caller's rate limit; a missing credential is not.
func (s *UserService) VerifyStepUp(userID uint, password, code, method string) (bool, error) {
	var user model.User
	if err := s.db.Select("id", "password").Where("id = ?", userID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrUserNotFound
		}
		return false, err
	}
	mfaService := NewMFAService(s.db, nil)
	mfa, err := mfaService.GetUserMFA(userID)
	if err != nil {
		return false, err
	}
	if mfa != nil && mfa.Enabled {
		code = strings.TrimSpace(code)
		if code == "" {
			return false, ErrStepUpMFARequired
		}
		valid, err := mfaService.Verify(userID, code, method)
		if err != nil {
			return true, err
		}
		if !valid {
			return true, ErrStepUpInvalidMFACode
		}
		return true, nil
	}
	if password == "" {
		return false, ErrStepUpPasswordRequired
	}
	if !checkPassword(password, user.Password) {
		return true, ErrStepUpInvalidPassword
	}
	return true, nil
}
