package model

import "time"

// Admin API token scopes.
const (
	// AdminAPITokenScopeRead lets a token make safe requests (GET and HEAD)
	// on the administrator APIs, except the reads that answer a secret in
	// clear.
	AdminAPITokenScopeRead = "read"
	// AdminAPITokenScopeAdmin lets a token do what its owner may do on the
	// administrator APIs, except manage API tokens.
	AdminAPITokenScopeAdmin = "admin"
)

// AdminAPIToken is an administrator's personal access token for automation.
// Only the SHA-256 of the token is stored: the token itself is shown once, in
// the answer that creates it. The owner's rights are read from v2_user on
// every use, so a banned, demoted or deleted administrator's tokens stop
// working at once.
type AdminAPIToken struct {
	ID     string `gorm:"primaryKey;size:36" json:"id"`
	UserID uint   `gorm:"not null;index" json:"user_id"`
	Name   string `gorm:"size:100;not null" json:"name"`
	// TokenHash is the hex SHA-256 of the token; it is the lookup key.
	TokenHash string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	// Hint is the last four characters of the token, to tell tokens apart.
	Hint       string     `gorm:"size:8;not null" json:"hint"`
	Scope      string     `gorm:"size:16;not null" json:"scope"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	LastUsedIP string     `gorm:"size:45" json:"last_used_ip,omitempty"`
	CreatedIP  string     `gorm:"size:45" json:"created_ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	// RevokeReason says why the token ended: owner_revoked, admin_revoked or
	// owner_not_admin (the owner was banned, demoted or deleted when it was
	// used).
	RevokeReason string `gorm:"size:32" json:"revoke_reason,omitempty"`
}

func (AdminAPIToken) TableName() string { return "v4_kernel_admin_api_token" }
