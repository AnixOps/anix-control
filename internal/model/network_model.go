package model

import "time"

// The device network model (docs/design/network-model,
// internal/networkmodel). New, protected tables written only by
// internal/networkmodel: the model document the administrator declares,
// the ANX-Line link keys Control generates for its transports, sealed
// under Control's key-encryption key, and each device's plan generation
// and reported facts.

// NetworkModel is the model document (networkmodel.Document as JSON), one
// row with ID 1. Revision grows with every write, so a writer can refuse
// to overwrite a document it has not read.
type NetworkModel struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Revision     uint64    `gorm:"not null;default:0" json:"revision"`
	DocumentJSON string    `gorm:"type:text;not null;default:''" json:"-"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (NetworkModel) TableName() string { return "v4_kernel_network_model" }

// NetworkTransportKey is a transport's ANX-Line link key: 32 bytes sealed
// with AES-256-GCM under module_runtime.ca_kek, with additional data
// naming the transport. It never leaves the kernel but in the plans of the
// transport's two devices.
type NetworkTransportKey struct {
	TransportID string    `gorm:"primaryKey;size:64" json:"transport_id"`
	SealedKey   string    `gorm:"type:text;not null" json:"-"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (NetworkTransportKey) TableName() string { return "v4_kernel_network_transport_key" }

// NetworkDevice is what Control keeps per device: the generation and hash
// of its last plan (sdk/network.Stamp), and what its Agent reported
// (network.v1 negotiated, its DeviceCapabilities and its last
// NetworkStatus, as protojson; the public keys by kind as JSON).
type NetworkDevice struct {
	NodeRef          string     `gorm:"primaryKey;size:32" json:"node_ref"`
	Generation       uint64     `gorm:"not null;default:0" json:"generation"`
	PlanHash         string     `gorm:"size:64;not null;default:''" json:"plan_hash"`
	Negotiated       bool       `gorm:"not null;default:false" json:"negotiated"`
	CapabilitiesJSON string     `gorm:"type:text;not null;default:''" json:"-"`
	PublicKeysJSON   string     `gorm:"type:text;not null;default:''" json:"-"`
	StatusJSON       string     `gorm:"type:text;not null;default:''" json:"-"`
	StatusAt         *time.Time `json:"status_at"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updated_at"`
}

func (NetworkDevice) TableName() string { return "v4_kernel_network_device" }

// NetworkModels are the network model's tables.
func NetworkModels() []any { return []any{&NetworkModel{}, &NetworkTransportKey{}, &NetworkDevice{}} }
