package model

import "time"

// IWANLine is a Panabit iWAN line (internal/iwanline): a third-party iWAN
// ingress that subscribers log into with the line's one shared account,
// carrying their traffic over the line (an IPLC, say) to its exit. Control
// runs nothing on it. A proxy node offers itself through a line with an
// entry of kind NodeEntryIWAN: AnixOps Net logs into the line and reaches
// the node through it.
type IWANLine struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:64;not null;uniqueIndex" json:"name"`
	// Host is a domain, an IPv4 or an IPv6 address (without brackets).
	Host string `gorm:"size:255;not null" json:"host"`
	Port int    `gorm:"not null" json:"port"`
	// Username and Password are the line's shared login; the API never
	// answers the password.
	Username string `gorm:"size:128;not null" json:"username"`
	Password string `gorm:"size:128;not null" json:"-"`
	// MTU is the tunnel MTU subscribers use; 0 is the client's default.
	MTU int `gorm:"not null;default:0" json:"mtu"`
	// Encrypt asks for the line's XOR masking (not confidentiality).
	Encrypt   bool      `gorm:"not null;default:false" json:"encrypt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (IWANLine) TableName() string { return "v5_iwan_line" }
