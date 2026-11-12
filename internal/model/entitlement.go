package model

import "time"

// EntitlementDevice is a device a user registered for a licensed feature
// (internal/entitlement): one row per user, feature and device key. The
// panel signs the device a license while the user's plan includes the
// feature (or the account's trial runs) and a seat is free.
type EntitlementDevice struct {
	ID     uint `gorm:"primaryKey"`
	UserID uint `gorm:"not null;uniqueIndex:idx_v5_entitlement_device_key,priority:1"`
	// Feature is the licensed feature's name, such as "anx".
	Feature string `gorm:"size:32;not null;uniqueIndex:idx_v5_entitlement_device_key,priority:2"`
	// DeviceID is the hex of the feature's device identifier (for anx,
	// SHA-256(alg ‖ public key)[..8], anx spec §2.4).
	DeviceID string `gorm:"size:64;not null;uniqueIndex:idx_v5_entitlement_device_key,priority:3"`
	// Alg is the device key's algorithm ("ed25519", "p256").
	Alg string `gorm:"size:16;not null"`
	// PublicKey is the hex of the device's public key.
	PublicKey string `gorm:"size:255;not null"`
	// Trial marks a device licensed by the account's trial.
	Trial bool `gorm:"not null;default:false"`
	// NotAfter is the end of the last license signed for the device (Unix
	// seconds).
	NotAfter int64 `gorm:"not null;default:0"`
	// RevokedAt is when the administrator revoked the device; a revoked
	// device gets no license and takes no seat.
	RevokedAt *time.Time
	CreatedAt time.Time `gorm:"not null"`
	// LastSeenAt is the last time the device asked for a license.
	LastSeenAt time.Time `gorm:"not null"`
}

// TableName is the table of the entitlement devices.
func (EntitlementDevice) TableName() string { return "v5_entitlement_device" }
