package model

import "time"

// IdentityTokenKey is a public key of the identity module that verifies its
// access tokens, as last pulled with IdentityService.GetTokenKeys. The kernel
// keeps them so it verifies tokens while the identity module is down.
type IdentityTokenKey struct {
	KeyID     string     `gorm:"column:kid;primaryKey;size:64" json:"kid"`
	PublicKey string     `gorm:"type:text;not null" json:"public_key"`
	State     string     `gorm:"size:16;not null" json:"state"`
	NotBefore *time.Time `json:"not_before"`
	NotAfter  *time.Time `json:"not_after"`
	UpdatedAt time.Time  `gorm:"not null" json:"updated_at"`
}

func (IdentityTokenKey) TableName() string { return "v4_kernel_identity_token_key" }
