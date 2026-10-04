package model

import "time"

// Entry high availability through DNS (forward-sdk.md section 7.4, L2;
// internal/kernelforward). New, protected tables written only by
// internal/kernelforward: DNS provider accounts with their credentials
// sealed under Control's key-encryption key, the routes' bindings to a
// provider zone with what was last published, and each binding's entry
// nodes with their health streaks.

// KernelForwardDNSProvider is a DNS provider account. SealedCredentials is
// the credentials as JSON, sealed with AES-256-GCM under
// module_runtime.ca_kek with additional data naming the row; it never
// leaves the kernel. CredentialNames lists the stored credentials (JSON).
type KernelForwardDNSProvider struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name              string    `gorm:"size:128;not null;uniqueIndex:ux_kernel_forward_dns_provider_name" json:"name"`
	Kind              string    `gorm:"size:32;not null" json:"kind"`
	ConfigJSON        string    `gorm:"type:text;not null;default:''" json:"config"`
	CredentialNames   string    `gorm:"type:text;not null;default:''" json:"credential_names"`
	SealedCredentials string    `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (KernelForwardDNSProvider) TableName() string { return "v4_kernel_forward_dns_provider" }

// KernelForwardDNSBinding binds a route's entry nodes to a name in a
// provider zone, with the controller's state: the values last published
// per record type (PublishedJSON, {"A": [...]}) and with which TTL, whether the last
// evaluation kept them because no entry was healthy (Degraded), and the
// provider failures with the back-off.
type KernelForwardDNSBinding struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RouteID       string     `gorm:"size:64;not null;uniqueIndex:ux_kernel_forward_dns_binding_route" json:"route_id"`
	ProviderID    uint64     `gorm:"not null;index;uniqueIndex:ux_kernel_forward_dns_binding_name" json:"provider_id"`
	Zone          string     `gorm:"size:253;not null;uniqueIndex:ux_kernel_forward_dns_binding_name" json:"zone"`
	RecordName    string     `gorm:"size:253;not null;uniqueIndex:ux_kernel_forward_dns_binding_name" json:"record_name"`
	Mode          string     `gorm:"size:16;not null" json:"mode"`
	RecordTypes   string     `gorm:"size:32;not null" json:"record_types"`
	TTL           uint32     `gorm:"not null" json:"ttl"`
	Paused        bool       `gorm:"not null;default:false" json:"paused"`
	State         string     `gorm:"size:32;not null;default:''" json:"state"`
	PublishedJSON string     `gorm:"type:text;not null;default:''" json:"published"`
	PublishedAt   *time.Time `json:"published_at"`
	EvaluatedAt   *time.Time `json:"evaluated_at"`
	Degraded      bool       `gorm:"not null;default:false" json:"degraded"`
	Failures      uint32     `gorm:"not null;default:0" json:"failures"`
	LastError     string     `gorm:"type:text;not null;default:''" json:"last_error"`
	LastErrorAt   *time.Time `json:"last_error_at"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null" json:"updated_at"`
	PublishedTTL  uint32     `gorm:"not null;default:0" json:"published_ttl"`
	DesiredJSON   string     `gorm:"type:text;not null;default:''" json:"desired"`
}

func (KernelForwardDNSBinding) TableName() string { return "v4_kernel_forward_dns_binding" }

// KernelForwardDNSNode is one entry node of a binding as the controller
// last saw it: whether its addresses are published (InRotation), its
// streaks of good and bad evaluations, and the last evaluation's reason.
type KernelForwardDNSNode struct {
	BindingID   uint64    `gorm:"primaryKey" json:"binding_id"`
	NodeRef     string    `gorm:"primaryKey;size:32" json:"node_ref"`
	InRotation  bool      `gorm:"not null;default:false" json:"in_rotation"`
	Healthy     bool      `gorm:"not null;default:false" json:"healthy"`
	GoodStreak  uint32    `gorm:"not null;default:0" json:"good_streak"`
	BadStreak   uint32    `gorm:"not null;default:0" json:"bad_streak"`
	Reason      string    `gorm:"size:32;not null;default:''" json:"reason"`
	EvaluatedAt time.Time `gorm:"not null" json:"evaluated_at"`
}

func (KernelForwardDNSNode) TableName() string { return "v4_kernel_forward_dns_node" }

// KernelForwardDNSModels are the entry HA tables.
func KernelForwardDNSModels() []any {
	return []any{&KernelForwardDNSProvider{}, &KernelForwardDNSBinding{}, &KernelForwardDNSNode{}}
}
