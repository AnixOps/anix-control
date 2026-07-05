package model

import "time"

// NodeLog stores runtime logs reported by V2bX parent nodes over gRPC.
type NodeLog struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	NodeID     uint       `gorm:"index;not null" json:"node_id"`
	Level      string     `gorm:"size:20;index;not null" json:"level"`
	Source     string     `gorm:"size:120;index" json:"source"`
	Message    string     `gorm:"type:text;not null" json:"message"`
	TraceID    string     `gorm:"size:120;index" json:"trace_id"`
	FieldsJSON string     `gorm:"type:text" json:"fields_json"`
	LoggedAt   *time.Time `gorm:"index" json:"logged_at"`
	CreatedAt  time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (NodeLog) TableName() string {
	return "v2_node_log"
}
