package model

import "time"

// SubscriberRequest records each applied subscriber write (KernelSubscriber
// and the kernel's own callers) by request id, with its result, so a retried
// request is applied once.
type SubscriberRequest struct {
	RequestID string    `gorm:"primaryKey;size:128" json:"request_id"`
	Method    string    `gorm:"size:32;not null" json:"method"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	Result    string    `gorm:"type:text;not null;default:''" json:"result"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
}

func (SubscriberRequest) TableName() string { return "v4_kernel_subscriber_request" }
