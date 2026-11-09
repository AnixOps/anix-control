package model

import "time"

// ClientFailureReport is what one subscriber's clients reported about one
// node in one hour on one kind of network (internal/clientreport, anx
// docs/FUTURE-IMPROVEMENTS.md F-55 item 1): connection outcomes by cause,
// added up over the hour's reports. Counts only, never a destination.
type ClientFailureReport struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;uniqueIndex:idx_v5_client_failure_report_key,priority:1" json:"user_id"`
	Hour           time.Time `gorm:"not null;uniqueIndex:idx_v5_client_failure_report_key,priority:2;index" json:"hour"`
	Node           string    `gorm:"size:128;not null;uniqueIndex:idx_v5_client_failure_report_key,priority:3" json:"node"`
	Network        string    `gorm:"size:16;not null;uniqueIndex:idx_v5_client_failure_report_key,priority:4" json:"network"`
	Server         string    `gorm:"size:255;not null;default:''" json:"server"`
	Proto          string    `gorm:"size:32;not null;default:''" json:"proto"`
	Platform       string    `gorm:"size:16;not null;default:''" json:"platform"`
	Client         string    `gorm:"size:64;not null;default:''" json:"client"`
	OK             int64     `gorm:"not null;default:0" json:"ok"`
	Fallback       int64     `gorm:"not null;default:0" json:"fallback"`
	Timeout        int64     `gorm:"not null;default:0" json:"timeout"`
	Refused        int64     `gorm:"not null;default:0" json:"refused"`
	Reset          int64     `gorm:"not null;default:0" json:"reset"`
	AfterHandshake int64     `gorm:"not null;default:0" json:"after_handshake"`
	KeepaliveLost  int64     `gorm:"not null;default:0" json:"keepalive_lost"`
	Other          int64     `gorm:"not null;default:0" json:"other"`
	// Stopped: the client gave up on the node (F-48).
	Stopped bool `gorm:"not null;default:false" json:"stopped"`
	// ClockSkewSecs is the latest skew the client measured, if any.
	ClockSkewSecs *int64    `json:"clock_skew_secs"`
	UpdatedAt     time.Time `gorm:"not null" json:"updated_at"`
}

func (ClientFailureReport) TableName() string { return "v5_client_failure_report" }
