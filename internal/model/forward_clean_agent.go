package model

import "time"

const (
	ForwardCleanAgentStatusRevoked = -1
	ForwardCleanAgentStatusOffline = 0
	ForwardCleanAgentStatusOnline  = 1
)

// ForwardCleanAgent is the clean-room runtime worker used by the forward subsystem.
type ForwardCleanAgent struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	NodeID       *uint      `gorm:"index" json:"nodeId"`
	Name         string     `gorm:"size:100;not null" json:"name"`
	Token        string     `gorm:"size:160;not null;uniqueIndex" json:"-"`
	Version      string     `gorm:"size:50" json:"version"`
	Hostname     string     `gorm:"size:255" json:"hostname"`
	OS           string     `gorm:"size:50" json:"os"`
	Arch         string     `gorm:"size:50" json:"arch"`
	Kernel       string     `gorm:"size:120" json:"kernel"`
	PublicIP     string     `gorm:"size:64" json:"publicIp"`
	PrivateIP    string     `gorm:"size:64" json:"privateIp"`
	Capabilities string     `gorm:"type:text" json:"capabilities"`
	Status       int        `gorm:"default:0;index" json:"status"`
	LastSeen     *time.Time `gorm:"index" json:"lastSeen"`
	LastError    string     `gorm:"type:text" json:"lastError"`
	RevokedAt    *time.Time `json:"revokedAt"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (ForwardCleanAgent) TableName() string {
	return "v2_forward_clean_agent"
}
