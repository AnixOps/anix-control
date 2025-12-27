package model

import "time"

// Event 简单事件表，用于记录管理员操作或异步任务
type Event struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Type        string     `gorm:"size:100;index" json:"type"` // e.g. plan.created, plan.updated, plan.assigned
	Payload     *string    `gorm:"type:text" json:"payload"`
	Status      string     `gorm:"size:20;default:'pending'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at"`
}

func (Event) TableName() string {
	return "v2_event"
}
