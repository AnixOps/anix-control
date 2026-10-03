package model

import "time"

// ShadowMismatchSample is one sanitized shadow-mode mismatch of a package
// route: the kernel stores what package hosts report in their Health
// details (sdk/shadowsample) after sanitizing it again. Rows are kept for
// seven days and at most 100 per route (internal/shadowsamples).
type ShadowMismatchSample struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	PackageID string `gorm:"size:120;not null;uniqueIndex:idx_v4_shadow_sample_key,priority:1;index:idx_v4_shadow_sample_route,priority:1" json:"package_id"`
	// SampleID is the host's id of the sample; the kernel stores each once.
	SampleID       string    `gorm:"size:32;not null;uniqueIndex:idx_v4_shadow_sample_key,priority:2" json:"sample_id"`
	RouteID        string    `gorm:"size:191;not null;index:idx_v4_shadow_sample_route,priority:2" json:"route_id"`
	PackageVersion string    `gorm:"size:64;not null;default:''" json:"package_version"`
	Method         string    `gorm:"size:16;not null;default:''" json:"method"`
	Path           string    `gorm:"size:512;not null;default:''" json:"path"`
	LegacyStatus   int       `gorm:"not null;default:0" json:"legacy_status"`
	NativeStatus   int       `gorm:"not null;default:0" json:"native_status"`
	DiffJSON       string    `gorm:"type:text;not null" json:"-"`
	DiffTruncated  bool      `gorm:"not null;default:false" json:"diff_truncated"`
	RequestID      string    `gorm:"size:128;not null;default:''" json:"request_id"`
	ObservedAt     time.Time `gorm:"not null;index;index:idx_v4_shadow_sample_route,priority:3" json:"observed_at"`
	CreatedAt      time.Time `gorm:"not null" json:"created_at"`
}

func (ShadowMismatchSample) TableName() string { return "v4_kernel_shadow_mismatch_sample" }
