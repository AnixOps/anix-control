package model

const (
	SpeedLimitStatusDisabled = 0
	SpeedLimitStatusActive   = 1
)

// SpeedLimit mirrors the flux-panel speed-limit resource.
type SpeedLimit struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	CreatedTime int64  `gorm:"index;not null" json:"createdTime"`
	UpdatedTime int64  `gorm:"index;not null" json:"updatedTime"`
	Status      int    `gorm:"default:1" json:"status"`
	Name        string `gorm:"size:100;not null" json:"name"`
	Speed       int64  `gorm:"not null" json:"speed"`
	TunnelID    uint   `gorm:"index;not null" json:"tunnelId"`
	TunnelName  string `gorm:"size:100;not null" json:"tunnelName"`
}

func (SpeedLimit) TableName() string {
	return "v2_speed_limit"
}
