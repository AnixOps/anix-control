package model

import "time"

// EgressRouteProtocol is the protocol of every egress relay, named in the
// member "egress" so that an Agent core can refuse one it does not speak
// and a later protocol needs no new format.
const EgressRouteProtocol = "shadowsocks"

// EgressRouteMethod is the cipher of every egress relay: Shadowsocks 2022
// with a 16-byte key.
const EgressRouteMethod = "2022-blake3-aes-128-gcm"

// EgressRoute sends the traffic a proxy node's users open to some domains
// out through another proxy node (internal/egressroute): the source node's
// Agent routes the domains to a Shadowsocks 2022 relay the exit node's
// Agent serves on RelayPort. Both nodes get the route in the member
// "egress" of their desired configuration.
type EgressRoute struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"size:100;not null" json:"name"`
	SourceNodeID uint   `gorm:"not null;index" json:"source_node_id"`
	ExitNodeID   uint   `gorm:"not null;index" json:"exit_node_id"`
	// ExitHost is the address the source node dials; the exit node's host
	// when empty.
	ExitHost  string `gorm:"size:255;not null;default:''" json:"exit_host"`
	RelayPort int    `gorm:"not null" json:"relay_port"`
	Method    string `gorm:"size:64;not null" json:"method"`
	// RelayKey is the relay's base64 key. It is only ever in the two nodes'
	// desired configurations, never in an answer.
	RelayKey string `gorm:"type:text;not null" json:"-"`
	// Domains is a JSON array of the domains, as the Agent matches them: a
	// bare name is the domain and its subdomains; full:, domain:, keyword:
	// and regexp: entries are kept.
	Domains   string    `gorm:"type:text;not null" json:"-"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (EgressRoute) TableName() string { return "v5_egress_route" }
