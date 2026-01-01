package model

import "time"

// Ticket 工单模型
type Ticket struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Subject   string    `gorm:"size:255" json:"subject"`
	Level     int       `gorm:"default:1" json:"level"`  // 0: low, 1: medium, 2: high
	Status    int       `gorm:"default:0" json:"status"` // 0: open, 1: answered, 2: closed
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	User     *User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Messages []TicketMessage `gorm:"foreignKey:TicketID" json:"messages,omitempty"`
}

func (Ticket) TableName() string {
	return "v2_ticket"
}

// TicketMessage 工单消息模型
type TicketMessage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TicketID  uint      `gorm:"index" json:"ticket_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Message   string    `gorm:"type:text" json:"message"`
	IsAdmin   int       `gorm:"default:0" json:"is_admin"` // 0: user, 1: admin
	CreatedAt time.Time `json:"created_at"`
}

func (TicketMessage) TableName() string {
	return "v2_ticket_message"
}
