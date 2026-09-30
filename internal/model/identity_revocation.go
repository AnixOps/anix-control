package model

import "time"

// IdentityRevocation makes a user's tokens stop working: those issued before
// NotBefore, and those whose token version (tv claim) is below TokenVersion.
// One row per user; a later revocation only moves the bounds forward.
type IdentityRevocation struct {
	UserID       uint      `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	TokenVersion uint64    `gorm:"not null;default:0" json:"token_version"`
	NotBefore    time.Time `gorm:"not null" json:"not_before"`
	Reason       string    `gorm:"size:64;not null;default:''" json:"reason"`
	UpdatedAt    time.Time `gorm:"not null;index" json:"updated_at"`
}

func (IdentityRevocation) TableName() string { return "v4_kernel_identity_revocation" }

// IdentitySessionRevocation denies one session (sid claim), for example after
// a logout. The row can go once the session's tokens have expired.
type IdentitySessionRevocation struct {
	SessionID string    `gorm:"primaryKey;size:64" json:"session_id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	Reason    string    `gorm:"size:64;not null;default:''" json:"reason"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (IdentitySessionRevocation) TableName() string { return "v4_kernel_identity_session_revocation" }
