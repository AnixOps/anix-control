package model

import "time"

// PackageReportState is the latest PackageReport (package-reports.v1) a
// node's Agent delivered for one plugin package and report kind: one row per
// node, plugin and kind, replaced by each newer report. The kernel keeps no
// history (docs/architecture/package-reports.md). PayloadJSON is the payload
// as the kind's sanitizer re-encoded it, never the bytes the Agent sent.
// Packages read their own rows through kapi_package_report_v1.
type PackageReportState struct {
	NodeKind    string    `gorm:"primaryKey;size:16" json:"node_kind"`
	NodeID      uint      `gorm:"primaryKey" json:"node_id"`
	PluginID    string    `gorm:"primaryKey;size:120" json:"plugin_id"`
	Kind        string    `gorm:"primaryKey;size:64" json:"kind"`
	Version     string    `gorm:"size:64;not null" json:"version"`
	PayloadJSON string    `gorm:"type:text;not null" json:"payload"`
	ObservedAt  time.Time `gorm:"not null;index" json:"observed_at"`
	ReceivedAt  time.Time `gorm:"not null;index" json:"received_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PackageReportState) TableName() string { return "v4_kernel_package_report_state" }
