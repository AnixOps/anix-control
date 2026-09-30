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

// ModuleEnrollment is a credential a module presents once (or, when
// Reusable, until revoked or expired) to obtain its first certificate. Only
// the SHA-256 of the credential is stored.
type ModuleEnrollment struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	PackageID      string     `gorm:"size:120;not null;index" json:"package_id"`
	Cluster        string     `gorm:"size:63;not null" json:"cluster"`
	CredentialHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	Reusable       bool       `gorm:"not null" json:"reusable"`
	ExpiresAt      time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt         *time.Time `json:"used_at"`
	UseCount       int64      `gorm:"not null;default:0" json:"use_count"`
	RevokedAt      *time.Time `json:"revoked_at"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (ModuleEnrollment) TableName() string { return "v4_kernel_module_enrollment" }

// ModuleCertificate records every certificate the module PKI issued, so a
// revoked enrollment stops renewals and the listener can reject revoked
// serials.
type ModuleCertificate struct {
	Serial       string     `gorm:"primaryKey;size:40" json:"serial"`
	PackageID    string     `gorm:"size:120;not null;index" json:"package_id"`
	Cluster      string     `gorm:"size:63;not null" json:"cluster"`
	EnrollmentID string     `gorm:"size:36;not null;index" json:"enrollment_id"`
	IssuerKeyID  string     `gorm:"size:64;not null" json:"issuer_key_id"`
	NotAfter     time.Time  `gorm:"not null;index" json:"not_after"`
	RevokedAt    *time.Time `json:"revoked_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (ModuleCertificate) TableName() string { return "v4_kernel_module_certificate" }
