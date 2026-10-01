package model

import "time"

// Agent enrollment methods: how the agent proved which node it runs on.
const (
	// AgentEnrollmentMethodCredential is a one-time anixagt_ credential
	// bound to a node.
	AgentEnrollmentMethodCredential = "enrollment_credential"
	// AgentEnrollmentMethodNodeAPIKey is a proxy node's API key (agents in
	// the field, and nodes minted by registration keys).
	AgentEnrollmentMethodNodeAPIKey = "node_api_key" // #nosec G101 -- names an enrollment method, not a credential.
	// AgentEnrollmentMethodForwardToken is a forward node's token.
	AgentEnrollmentMethodForwardToken = "forward_token"
)

// AgentEnrollment is one way into the agent PKI for a node: a one-time
// enrollment credential an administrator issued (only its SHA-256 is kept),
// or an enrollment the agent made with its node credential. Every agent
// certificate points at the enrollment it descends from; revoking the
// enrollment stops its renewals.
type AgentEnrollment struct {
	ID       string `gorm:"primaryKey;size:36" json:"id"`
	NodeKind string `gorm:"size:16;not null;index:idx_v4_kernel_agent_enrollment_node,priority:1" json:"node_kind"`
	NodeID   uint   `gorm:"not null;index:idx_v4_kernel_agent_enrollment_node,priority:2" json:"node_id"`
	Cluster  string `gorm:"size:63;not null" json:"cluster"`
	Method   string `gorm:"size:32;not null" json:"method"`
	// CredentialHash is set for AgentEnrollmentMethodCredential only.
	CredentialHash *string    `gorm:"size:64;uniqueIndex" json:"-"`
	ExpiresAt      *time.Time `json:"expires_at"`
	UsedAt         *time.Time `json:"used_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RevokeReason   string     `gorm:"size:64" json:"revoke_reason,omitempty"`
	AgentVersion   string     `gorm:"size:64" json:"agent_version,omitempty"`
	InstanceID     string     `gorm:"size:128" json:"instance_id,omitempty"`
	CreatedBy      uint       `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (AgentEnrollment) TableName() string { return "v4_kernel_agent_enrollment" }

// AgentCertificate records every certificate the agent PKI issued, so the
// agent listener can refuse revoked serials and revoking a node's
// credentials revokes its certificates.
type AgentCertificate struct {
	Serial       string     `gorm:"primaryKey;size:40" json:"serial"`
	NodeKind     string     `gorm:"size:16;not null;index:idx_v4_kernel_agent_certificate_node,priority:1" json:"node_kind"`
	NodeID       uint       `gorm:"not null;index:idx_v4_kernel_agent_certificate_node,priority:2" json:"node_id"`
	Cluster      string     `gorm:"size:63;not null" json:"cluster"`
	EnrollmentID string     `gorm:"size:36;not null;index" json:"enrollment_id"`
	IssuerKeyID  string     `gorm:"size:64;not null" json:"issuer_key_id"`
	NotAfter     time.Time  `gorm:"not null;index" json:"not_after"`
	RevokedAt    *time.Time `json:"revoked_at"`
	RevokeReason string     `gorm:"size:64" json:"revoke_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (AgentCertificate) TableName() string { return "v4_kernel_agent_certificate" }
