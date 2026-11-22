package model

import "time"

// Service CA states.
const (
	ServiceCAStateNext    = "next"
	ServiceCAStateCurrent = "current"
	ServiceCAStateRetired = "retired"
)

// ServiceCA is one certificate authority of the kernel's module PKI. Its
// private key is stored sealed under the configured key-encryption key.
type ServiceCA struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Cluster        string     `gorm:"size:63;not null;index" json:"cluster"`
	State          string     `gorm:"size:16;not null;index" json:"state"`
	KeyID          string     `gorm:"size:64;not null;uniqueIndex" json:"key_id"`
	CertificatePEM string     `gorm:"type:text;not null" json:"certificate_pem"`
	SealedKey      string     `gorm:"type:text;not null" json:"-"`
	NotBefore      time.Time  `gorm:"not null" json:"not_before"`
	NotAfter       time.Time  `gorm:"not null" json:"not_after"`
	ActivatedAt    *time.Time `json:"activated_at"`
	RetiredAt      *time.Time `json:"retired_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (ServiceCA) TableName() string { return "v4_kernel_service_ca" }
