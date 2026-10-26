package model

import "time"

// Node entry kinds (NodeEntry.Kind).
const (
	// NodeEntryDirect: subscribers dial the address on the protocol's own
	// port, for example a native IPv6 address of one of the machine's lines.
	NodeEntryDirect = "direct"
	// NodeEntryNAT: the provider maps a public address's ports to the
	// machine's (a NAT IPv4 entry); PortMap says which public port reaches
	// which protocol port.
	NodeEntryNAT = "nat"
)

// NodeEntry is one address subscribers reach a node at (internal/nodeentry),
// for machines with several lines or address families: a Shanghai relay with
// a CN2 and a 9929 line, each with a native IPv6 address and a provider NAT
// IPv4 address. A node without entries is reached at its host; a node with
// enabled entries is offered once per entry and protocol instead, the
// protocol's name followed by the entry's label.
type NodeEntry struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	NodeID uint   `gorm:"not null;index" json:"node_id"`
	Label  string `gorm:"size:64;not null" json:"label"`
	Kind   string `gorm:"size:16;not null" json:"kind"`
	// Host is a domain, an IPv4 or an IPv6 address (without brackets).
	Host string `gorm:"size:255;not null" json:"host"`
	// PortMap is a JSON array of nodeentry.PortRange, for a NAT entry: the
	// protocol ports from..to are reached at external..external+(to-from).
	// Empty for a direct entry.
	PortMap   string    `gorm:"type:text;not null;default:''" json:"-"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (NodeEntry) TableName() string { return "v5_node_entry" }
