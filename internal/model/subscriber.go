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

// UserActivity is when a user was last seen online: the latest traffic report
// that carried the user's traffic or alive report that listed one of the
// user's connections, from any node or Agent (subscriber.RecordOnline). It
// is a table of its own because Control keeps no durable value for it:
// v2_user has no such column, the online set is a five-minute cache and
// v2_server_log is purged by the operator. One row per user, written at most
// once per subscriber.ActivityWriteInterval; a user never seen has no row.
type UserActivity struct {
	UserID uint `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	// LastOnlineAt is a Unix time in seconds.
	LastOnlineAt int64 `gorm:"not null;index" json:"last_online_at"`
}

func (UserActivity) TableName() string { return "v4_kernel_user_activity" }
