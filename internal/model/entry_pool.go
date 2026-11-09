package model

import "time"

// EntryPoolPolicy is the single row (ID 1) of how node entries are dealt
// out to subscribers (internal/entrypool, anx docs/FUTURE-IMPROVEMENTS.md
// F-55): an entry with a pool is offered only to the subscribers of that
// pool; an entry without one to everyone outside the quarantine pool.
type EntryPoolPolicy struct {
	ID uint `gorm:"primaryKey;autoIncrement:false" json:"-"`
	// Shards deals the subscribers without an assignment into pools
	// shard-0 … shard-(Shards-1) by user id; 0 = no sharding.
	Shards int `gorm:"not null;default:0" json:"shards"`
	// QuarantineDays: accounts younger than this many days, and trial
	// accounts, are in QuarantinePool; 0 = off.
	QuarantineDays int       `gorm:"not null;default:0" json:"quarantine_days"`
	QuarantinePool string    `gorm:"size:32;not null;default:'quarantine'" json:"quarantine_pool"`
	UpdatedAt      time.Time `gorm:"not null" json:"updated_at"`
}

func (EntryPoolPolicy) TableName() string { return "v5_entry_pool_policy" }

// UserEntryPool is an administrator's assignment of a subscriber to a pool,
// which overrides the policy (to isolate a leak, or to split a pool).
type UserEntryPool struct {
	UserID    uint      `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	Pool      string    `gorm:"size:32;not null;index" json:"pool"`
	Reason    string    `gorm:"size:128;not null;default:''" json:"reason"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (UserEntryPool) TableName() string { return "v5_user_entry_pool" }
