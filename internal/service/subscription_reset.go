package service

import "time"

// UserSubscriptionReset is a user's reset of their own subscription token,
// for UserSubscriptionResetRequestID.
const UserSubscriptionReset = "user_reset_subscribe"

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

// The answers of a subscription reset that does not reset; identity-platform
// answers the same messages.
const (
	SubscriptionResetRateLimitedMessage = "too many subscription reset attempts, please try again later"
	SubscriptionResetFailedMessage      = "重置订阅失败"
)
