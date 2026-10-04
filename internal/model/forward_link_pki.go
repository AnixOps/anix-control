package model

import "time"

// Forward link CA states, as the module CA's (ServiceCAState*): the current
// CA signs, a next CA is trusted before it signs, a retired CA stays trusted
// until the link certificates it signed have expired.
const (
	ForwardLinkCAStateCurrent = "current"
	ForwardLinkCAStateNext    = "next"
	ForwardLinkCAStateRetired = "retired"
)

// ForwardLinkCA is a CA of the forward link PKI (owner decision H28,
// forward-sdk.md section 6.2): a self-signed root, separate from the module,
// kernel and Agent CA (ServiceCA), that signs only the link certificates
// forward nodes' engines (gost) present to each other. Its private key is
// sealed with module_runtime.ca_kek, like the module CA's, under its own
// additional data.
type ForwardLinkCA struct {
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

func (ForwardLinkCA) TableName() string { return "v4_kernel_forward_link_ca" }

// ForwardLinkCertificate records every link certificate the forward link CA
// issued: which node holds it, which Agent certificate asked for it, and
// when it was revoked (with the node's Agent credentials,
// agentpki.RevokeNode).
type ForwardLinkCertificate struct {
	Serial   string `gorm:"primaryKey;size:40" json:"serial"`
	NodeKind string `gorm:"size:16;not null;index:idx_v4_kernel_forward_link_certificate_node,priority:1" json:"node_kind"`
	NodeID   uint   `gorm:"not null;index:idx_v4_kernel_forward_link_certificate_node,priority:2" json:"node_id"`
	Cluster  string `gorm:"size:63;not null" json:"cluster"`
	// AgentSerial is the serial of the Agent client certificate the request
	// was authenticated with.
	AgentSerial  string     `gorm:"size:40;not null" json:"agent_serial"`
	IssuerKeyID  string     `gorm:"size:64;not null" json:"issuer_key_id"`
	DNSName      string     `gorm:"size:64;not null" json:"dns_name"`
	SPIFFEID     string     `gorm:"column:spiffe_id;size:255;not null" json:"spiffe_id"`
	NotAfter     time.Time  `gorm:"not null;index" json:"not_after"`
	RevokedAt    *time.Time `json:"revoked_at"`
	RevokeReason string     `gorm:"size:64" json:"revoke_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (ForwardLinkCertificate) TableName() string { return "v4_kernel_forward_link_certificate" }
