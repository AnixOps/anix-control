package model

import (
	"time"
)

// AuditLog records detailed audit trail for admin operations.
// Designed for compliance and security review.
type AuditLog struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	UserID       *uint  `gorm:"index" json:"user_id"`
	Email        string `gorm:"size:255" json:"email"`
	Method       string `gorm:"size:10" json:"method"`        // HTTP method
	Path         string `gorm:"size:500" json:"path"`         // Request path
	Module       string `gorm:"size:100;index" json:"module"` // Extracted module from path
	Action       string `gorm:"size:50;index" json:"action"`  // create/update/delete/etc.
	IP           string `gorm:"size:45" json:"ip"`            // Client IP
	UserAgent    string `gorm:"size:500" json:"user_agent"`
	RequestID    string `gorm:"size:64" json:"request_id"`      // Correlation ID
	RequestBody  string `gorm:"type:text" json:"request_body"`  // Request body (truncated)
	StatusCode   int    `json:"status_code"`                    // HTTP response status
	DurationMS   int64  `json:"duration_ms"`                    // Request duration in milliseconds
	ErrorMessage string `gorm:"type:text" json:"error_message"` // Error if status >= 400

	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName specifies the table name for audit logs.
func (AuditLog) TableName() string {
	return "v2_audit_log"
}
