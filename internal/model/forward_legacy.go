package model

import "time"

// The v4.2 forwarding upgrade (forward-sdk.md section 10, F5c) records
// its three steps in tables of its own: the archives of the flux
// forwarding data, each forward node's cleanliness check, and the drop of
// the flux tables. They are new tables; no existing table is altered.

// ForwardLegacyArchive is one archive file of the flux forwarding data.
type ForwardLegacyArchive struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Path is the archive file's absolute path on the Control host.
	Path      string `gorm:"size:1024;not null" json:"path"`
	SHA256    string `gorm:"size:64;not null" json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	// Schema is the archive's schema version.
	Schema string `gorm:"size:64;not null" json:"schema"`
	// Trigger is "startup" (the first v4.2 start) or "cli".
	Trigger string `gorm:"size:16;not null" json:"trigger"`
	Actor   string `gorm:"size:255" json:"actor"`
	// RowCounts is a JSON object of each archived table's row count; a
	// table that did not exist is -1.
	RowCounts string    `gorm:"type:text" json:"row_counts"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName keeps the upgrade's tables with the other v4 tables.
func (ForwardLegacyArchive) TableName() string { return "v4_forward_legacy_archive" }

// ForwardLegacyNode is the latest cleanliness check of one forward node
// (v2_forward_node), and its abandonment.
type ForwardLegacyNode struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	NodeID   uint   `gorm:"not null;uniqueIndex" json:"node_id"`
	NodeRef  string `gorm:"size:64;not null" json:"node_ref"`
	NodeName string `gorm:"size:255" json:"node_name"`
	// State is the latest check's result: clean, dirty or unreachable.
	State string `gorm:"size:16;not null;index" json:"state"`
	// Checks is a JSON array of the check's paths (agent, nodex, ansible)
	// with their results.
	Checks    string     `gorm:"type:text" json:"checks"`
	Detail    string     `gorm:"type:text" json:"detail"`
	CheckedAt *time.Time `json:"checked_at"`
	// AbandonedAt is set when an administrator accepted the node as
	// unreachable for good; the drop then no longer waits for it.
	AbandonedAt   *time.Time `json:"abandoned_at"`
	AbandonedBy   string     `gorm:"size:255" json:"abandoned_by"`
	AbandonReason string     `gorm:"type:text" json:"abandon_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName keeps the upgrade's tables with the other v4 tables.
func (ForwardLegacyNode) TableName() string { return "v4_forward_legacy_node" }

// ForwardLegacyDrop records the drop of the flux forwarding tables. Its
// presence is what keeps Control from creating them again.
type ForwardLegacyDrop struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	ArchiveID     uint   `json:"archive_id"`
	ArchivePath   string `gorm:"size:1024" json:"archive_path"`
	ArchiveSHA256 string `gorm:"size:64" json:"archive_sha256"`
	// Backup is a JSON object naming the database backup the drop
	// accepted (source, path, size, sha256).
	Backup string `gorm:"type:text" json:"backup"`
	// Dropped and AlreadyMissing are JSON arrays of table names.
	Dropped        string    `gorm:"type:text" json:"dropped"`
	AlreadyMissing string    `gorm:"type:text" json:"already_missing"`
	Actor          string    `gorm:"size:255" json:"actor"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName keeps the upgrade's tables with the other v4 tables.
func (ForwardLegacyDrop) TableName() string { return "v4_forward_legacy_drop" }

// ForwardLegacyModels are the upgrade's tables.
func ForwardLegacyModels() []any {
	return []any{&ForwardLegacyArchive{}, &ForwardLegacyNode{}, &ForwardLegacyDrop{}}
}
