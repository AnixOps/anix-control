package model

import "time"

// IdentityAccountLink ties an identity-module account to the kernel
// subscriber (v2_user row) it owns. ProjectionVersion is the last account
// version applied to the v2_user identity columns; older ones are ignored.
type IdentityAccountLink struct {
	UserID            uint   `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	AccountUUID       string `gorm:"size:36;not null;uniqueIndex" json:"account_uuid"`
	ProjectionVersion uint64 `gorm:"not null;default:0" json:"projection_version"`
	// TokenVersion is the highest identity token version projected. A
	// session-ending projection revokes by token version only above it.
	TokenVersion uint64    `gorm:"not null;default:0" json:"token_version"`
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (IdentityAccountLink) TableName() string { return "v4_kernel_identity_account" }

// Identity authority states: which side owns credentials.
const (
	// IdentityAuthorityKernel: the kernel's legacy handlers own logins.
	IdentityAuthorityKernel = "kernel"
	// IdentityAuthorityImporting: accounts are being copied to identity.
	IdentityAuthorityImporting = "importing"
	// IdentityAuthorityIdentity: identity owns logins and mirrors
	// credentials back, so switching to legacy is an instant rollback.
	IdentityAuthorityIdentity = "identity"
	// IdentityAuthorityFinalized: legacy credentials are gone; no mirror.
	IdentityAuthorityFinalized = "finalized"
)

// IdentityAuthority is the single row recording the authority state and
// the account import's progress. No row means IdentityAuthorityKernel.
type IdentityAuthority struct {
	ID         uint      `gorm:"primaryKey;autoIncrement:false" json:"id"`
	State      string    `gorm:"size:16;not null" json:"state"`
	ImportID   string    `gorm:"size:64;not null;default:''" json:"import_id"`
	Checkpoint string    `gorm:"type:text;not null;default:''" json:"checkpoint"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
}

func (IdentityAuthority) TableName() string { return "v4_kernel_identity_authority" }

// UnusableLegacyPassword is stored in v2_user.password for accounts whose
// credentials live in identity. It never matches a password.
const UnusableLegacyPassword = "!identity"

// Identity cutover actions, recorded in IdentityCutoverEvent.
const (
	IdentityCutoverActionCutover  = "cutover"
	IdentityCutoverActionAborted  = "cutover_aborted"
	IdentityCutoverActionRollback = "rollback"
	IdentityCutoverActionFinalize = "finalize"
)

// IdentityCutoverEvent records each change of credential authority: the
// cutover to identity, an aborted cutover, a rollback and finalize.
// Finalize waits for a day after the latest cutover.
type IdentityCutoverEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Action    string    `gorm:"size:32;not null;index" json:"action"`
	ActorID   uint      `gorm:"not null;default:0" json:"actor_id"`
	Detail    string    `gorm:"type:text;not null;default:''" json:"detail"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
}

func (IdentityCutoverEvent) TableName() string { return "v4_kernel_identity_cutover" }
