package model

import "time"

// Knowledge 知识库文章模型
type Knowledge struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Category  string    `gorm:"size:64" json:"category"`
	Title     string    `gorm:"size:255" json:"title"`
	Body      string    `gorm:"type:text" json:"body"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Show      int       `gorm:"default:1" json:"show"` // 0: hidden, 1: visible
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Knowledge) TableName() string {
	return "v2_knowledge"
}
