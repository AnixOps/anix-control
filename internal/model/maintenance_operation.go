package model

import "time"

// MaintenanceChange binds approval to an immutable, bounded operation description.
// Config values stay private; public review includes their canonical hash.
type MaintenanceChange struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	RequestKey       string     `gorm:"size:80;not null;uniqueIndex" json:"request_key"`
	RequestedBy      uint       `gorm:"not null;index" json:"requested_by"`
	ApprovedBy       uint       `json:"approved_by"`
	NodeIDsJSON      string     `gorm:"type:text;not null" json:"node_ids_json"`
	PluginID         string     `gorm:"size:120;not null;index" json:"plugin_id"`
	TargetVersion    string     `gorm:"size:64;not null" json:"target_version"`
	Kind             string     `gorm:"size:32;not null" json:"kind"`
	ConfigJSON       string     `gorm:"type:text;not null" json:"-"`
	ConfigHash       string     `gorm:"size:64;not null" json:"config_hash"`
	BindingHash      string     `gorm:"size:64;not null" json:"binding_hash"`
	Status           string     `gorm:"size:24;not null;index" json:"status"`
	OperationIDsJSON string     `gorm:"type:text" json:"operation_ids_json"`
	CreatedAt        time.Time  `json:"created_at"`
	ApprovedAt       *time.Time `json:"approved_at"`
	QueuedAt         *time.Time `json:"queued_at"`
}

func (MaintenanceChange) TableName() string { return "v3_maintenance_change" }

// MaintenanceChangeAudit is append-only and survives the detailed event window.
type MaintenanceChangeAudit struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ChangeID    uint      `gorm:"not null;index" json:"change_id"`
	ActorID     uint      `gorm:"not null" json:"actor_id"`
	Action      string    `gorm:"size:32;not null" json:"action"`
	BindingHash string    `gorm:"size:64;not null" json:"binding_hash"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (MaintenanceChangeAudit) TableName() string { return "v3_maintenance_change_audit" }

// MaintenancePluginOverride pins explicit node-specific maintenance intent. A
// newer assignment generation supersedes it; reconciliation may not silently
// undo an operator's rollback or repeat a failed maintenance operation.
type MaintenancePluginOverride struct {
	NodeID     uint      `gorm:"primaryKey" json:"node_id"`
	PluginID   string    `gorm:"primaryKey;size:120" json:"plugin_id"`
	ChangeID   uint      `gorm:"not null;index" json:"change_id"`
	Generation int64     `gorm:"not null" json:"generation"`
	Version    string    `gorm:"size:64;not null" json:"version"`
	ConfigJSON string    `gorm:"type:text;not null" json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

func (MaintenancePluginOverride) TableName() string { return "v3_maintenance_plugin_override" }

func MaintenanceOperationModels() []any {
	return []any{&MaintenanceChange{}, &MaintenanceChangeAudit{}, &MaintenancePluginOverride{}}
}
