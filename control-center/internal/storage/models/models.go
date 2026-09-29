package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the control center
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Email        string         `gorm:"uniqueIndex;size:255" json:"email"`
	PasswordHash string         `gorm:"size:255" json:"-"`
	Role         string         `gorm:"size:50;default:viewer" json:"role"`
	AuthProvider string         `gorm:"size:50;default:local" json:"auth_provider"`
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	LastLoginAt  *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name
func (User) TableName() string {
	return "users"
}

// APIToken represents an API token
type APIToken struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"index" json:"user_id"`
	Name      string         `gorm:"size:100" json:"name"`
	Token     string         `gorm:"uniqueIndex;size:255" json:"token"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	LastUsed  *time.Time     `json:"last_used,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name
func (APIToken) TableName() string {
	return "api_tokens"
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Action    string    `gorm:"size:100" json:"action"`
	Resource  string    `gorm:"size:100" json:"resource"`
	IP        string    `gorm:"size:50" json:"ip"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	Details   string    `gorm:"type:text" json:"details"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName returns the table name
func (AuditLog) TableName() string {
	return "audit_logs"
}
