package model

import "time"

// KernelNodeDesiredConfig is a node's desired configuration
// (docs/architecture/node-ops-service.md section 5.5): the configuration
// the kernel built from the node's rows, with the revision and hash that
// every push and every agent's ConfigStatus name. SyncNode and the kernel
// writes that change what a node runs rebuild it. The revision grows only
// when the hash changes, so the same configuration keeps its revision.
//
// The document holds the node's runtime secrets (protocol keys): the table
// is protected (service.protectedTables), and no contract call answers it.
type KernelNodeDesiredConfig struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"-"`
	// NodeKind is proxy (v2_node) or forward (v2_forward_node); ids of the
	// two tables overlap.
	NodeKind string `gorm:"size:16;not null;uniqueIndex:idx_kernel_node_desired_config_node,priority:1" json:"node_kind"`
	NodeID   uint64 `gorm:"not null;uniqueIndex:idx_kernel_node_desired_config_node,priority:2" json:"node_id"`
	// Revision is monotonic per node; it starts at 1.
	Revision uint64 `gorm:"not null;default:0" json:"revision"`
	// ConfigHash is the SHA-256 (hex) of the canonical document.
	ConfigHash string `gorm:"size:64;not null" json:"config_hash"`
	// Format names the document's schema, kernelnodeops.DesiredConfigFormat.
	Format string `gorm:"size:40;not null" json:"format"`
	// ConfigJSON is the canonical document (sorted keys, no whitespace).
	ConfigJSON string `gorm:"type:text;not null" json:"-"`
	// ExcludedProtocols counts the protocols left out because they failed
	// validation (zero while validation is report-only, section 3.8).
	ExcludedProtocols uint32 `gorm:"not null;default:0" json:"excluded_protocols"`
	// BuiltAt is when the document was last rebuilt, changed or not.
	BuiltAt   time.Time `gorm:"not null" json:"built_at"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (KernelNodeDesiredConfig) TableName() string { return "v4_kernel_node_desired_config" }
