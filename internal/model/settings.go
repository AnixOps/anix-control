package model

import "time"

// SettingsRequest records each applied KernelSettings write by request id,
// with its namespace, method, calling package and result, so a retried
// write is applied once. Rows are kept for 90 days.
type SettingsRequest struct {
	RequestID string    `gorm:"primaryKey;size:128" json:"request_id"`
	Namespace string    `gorm:"size:64;not null" json:"namespace"`
	Method    string    `gorm:"size:32;not null" json:"method"`
	PackageID string    `gorm:"size:128;not null;default:''" json:"package_id"`
	Result    string    `gorm:"type:text;not null;default:''" json:"result"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
}

func (SettingsRequest) TableName() string { return "v4_kernel_settings_request" }
