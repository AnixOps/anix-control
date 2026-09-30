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

// SubscriberChange is the change log node user lists follow: a row per
// change that can alter what a node serves (entitlements, ban, uuid,
// traffic exhaustion or reset, creation, deletion). ID is the cursor.
// Consumers read the subscriber's current state; rows are kept 7 days.
type SubscriberChange struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	Deleted   bool      `gorm:"not null;default:false" json:"deleted"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
}

func (SubscriberChange) TableName() string { return "v4_kernel_subscriber_change" }
