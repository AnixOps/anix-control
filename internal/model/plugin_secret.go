package model

import "time"

// PluginSecret is the stable identity used by topology configuration. Secret
// bytes live only in immutable PluginSecretMaterial rows encrypted by a
// deployment-owned key from config.yaml.
type PluginSecret struct {
	ID            string     `gorm:"primaryKey;size:120" json:"id"`
	Name          string     `gorm:"size:160;not null" json:"name"`
	Description   string     `gorm:"type:text" json:"description"`
	ActiveVersion uint64     `gorm:"not null;default:0" json:"active_version"`
	CreatedBy     uint       `gorm:"not null" json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

func (PluginSecret) TableName() string { return "v3_kernel_plugin_secret" }

// PluginSecretVersion is immutable after creation. A new certificate or key
// always creates a new version so in-flight deployment and rollback references
// remain deterministic.
type PluginSecretVersion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SecretID  string    `gorm:"size:120;not null;uniqueIndex:ux_plugin_secret_version" json:"secret_id"`
	Version   uint64    `gorm:"not null;uniqueIndex:ux_plugin_secret_version" json:"version"`
	KeyID     string    `gorm:"size:80;not null" json:"key_id"`
	FileCount int       `gorm:"not null" json:"file_count"`
	CreatedBy uint      `gorm:"not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (PluginSecretVersion) TableName() string { return "v3_kernel_plugin_secret_version" }

// PluginSecretMaterial stores AES-256-GCM ciphertext. Ciphertext and nonce are
// never JSON serializable; callers receive metadata or explicitly decrypted
// dispatch material through the service boundary.
type PluginSecretMaterial struct {
	ID              uint   `gorm:"primaryKey" json:"-"`
	SecretVersionID uint   `gorm:"not null;index;uniqueIndex:ux_plugin_secret_material" json:"-"`
	Name            string `gorm:"size:120;not null;uniqueIndex:ux_plugin_secret_material" json:"name"`
	Ciphertext      []byte `gorm:"type:bytea;not null" json:"-"`
	Nonce           []byte `gorm:"type:bytea;not null" json:"-"`
	SHA256          string `gorm:"size:64;not null" json:"sha256"`
	Size            int64  `gorm:"not null" json:"size"`
}

func (PluginSecretMaterial) TableName() string { return "v3_kernel_plugin_secret_material" }

// PluginSecretAudit contains metadata only. Detail must never contain request
// bodies, decrypted bytes, ciphertext, nonces, or config JSON.
type PluginSecretAudit struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SecretID    string    `gorm:"size:120;not null;index;uniqueIndex:ux_plugin_secret_dispatch,priority:1" json:"secret_id"`
	Version     uint64    `gorm:"not null;default:0;uniqueIndex:ux_plugin_secret_dispatch,priority:2" json:"version"`
	Action      string    `gorm:"size:48;not null;index;uniqueIndex:ux_plugin_secret_dispatch,priority:3" json:"action"`
	ActorID     uint      `gorm:"not null" json:"actor_id"`
	OperationID string    `gorm:"size:64;index;uniqueIndex:ux_plugin_secret_dispatch,priority:4" json:"operation_id,omitempty"`
	NodeID      *uint     `gorm:"index;uniqueIndex:ux_plugin_secret_dispatch,priority:5" json:"node_id,omitempty"`
	Outcome     string    `gorm:"size:32;not null" json:"outcome"`
	Detail      string    `gorm:"size:255" json:"detail"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (PluginSecretAudit) TableName() string { return "v3_kernel_plugin_secret_audit" }
