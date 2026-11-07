package model

import "time"

// UserSS2022Key is a user's random Shadowsocks 2022 key: std base64 of 32
// bytes (the first 16 for the 128-bit ciphers). Like v2_user.uuid it is a
// credential every node must receive, so it is stored in plain text. A user
// gets one at the first subscription pull; the legacy key derived from the
// UUID keeps working next to it.
type UserSS2022Key struct {
	UserID    uint      `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	UserKey   string    `gorm:"size:64;not null" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (UserSS2022Key) TableName() string { return "v5_user_ss2022_key" }
