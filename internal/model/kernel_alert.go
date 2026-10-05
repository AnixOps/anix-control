package model

import "time"

// Kernel alert severities.
const (
	KernelAlertWarning  = "warning"
	KernelAlertCritical = "critical"
)

// KernelAlert is the state of one alert the kernel's alert monitor
// (internal/kernelalerts) raised: a certificate that was not renewed in
// time, a CA close to its end, or a phased process (the node credential
// split, the identity cutover) that has not moved for too long. One row per
// Key (alert kind and subject). The monitor re-evaluates the facts on every
// scan: a row whose finding is gone is resolved (ResolvedAt), and one that
// comes back is reopened, so administrators are told once per subject and
// again only after the re-notification interval or when the severity grows.
//
// A row holds no secret: kinds, node names, dates and counts only.
type KernelAlert struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Key is "<kind>/<subject>", unique.
	Key string `gorm:"column:alert_key;size:191;not null;uniqueIndex" json:"key"`
	// Kind is one of the kernelalerts.Kind* values.
	Kind     string `gorm:"size:48;not null;index" json:"kind"`
	Severity string `gorm:"size:16;not null" json:"severity"`
	// SubjectKind names what the alert is about (node, ca, module,
	// node_secrets, identity) and Subject which one (proxy-12,
	// service_ca:<key id>, a package id, a split table).
	SubjectKind string `gorm:"size:32;not null" json:"subject_kind"`
	Subject     string `gorm:"size:120;not null" json:"subject"`
	// Message is an English sentence for notifications; the admin UI
	// composes its own text from Kind and Detail.
	Message string `gorm:"size:500;not null" json:"message"`
	// Detail is a JSON object of the facts behind the alert.
	Detail string `gorm:"type:text;not null;default:''" json:"-"`
	// ExpiresAt is the end of the certificate the alert is about.
	ExpiresAt   *time.Time `json:"expires_at"`
	FirstSeenAt time.Time  `gorm:"not null" json:"first_seen_at"`
	LastSeenAt  time.Time  `gorm:"not null" json:"last_seen_at"`
	// LastNotifiedAt and NotifiedSeverity record the last notification of
	// this alert; NotifyCount counts them.
	LastNotifiedAt   *time.Time `json:"last_notified_at"`
	NotifiedSeverity string     `gorm:"size:16;not null;default:''" json:"-"`
	NotifyCount      int        `gorm:"not null;default:0" json:"notify_count"`
	ResolvedAt       *time.Time `gorm:"index" json:"resolved_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (KernelAlert) TableName() string { return "v4_kernel_alert" }
