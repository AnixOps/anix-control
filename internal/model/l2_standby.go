package model

import "time"

// Tiers of a cross-border standby channel (internal/l2standby, anx
// docs/FUTURE-IMPROVEMENTS.md F-54 item 1).
const (
	// L2StandbyTierReality: VLESS + REALITY + Vision to the exit node's
	// relay port. Quickest to switch to, but the same borrowed-handshake
	// family as the A2 jacket: it covers a fault of A2 itself.
	L2StandbyTierReality = "reality"
	// L2StandbyTierXHTTP: VLESS over XHTTP behind a TLS front (nginx with a
	// real certificate for XHTTPDomain) on the exit node; the relay listens
	// on loopback. A different family from A2.
	L2StandbyTierXHTTP = "xhttp"
)

// L2Standby is one prepared public-protocol standby channel of a
// cross-border (kind anx) transport of the network model, for one pair of
// proxy nodes: the user ingress node the paths through the transport start
// at (SourceNodeID) and the node they leave from (ExitNodeID). While Active,
// both nodes get it in the member "egress" of their desired configuration
// as a catch-all VLESS egress route and relay, so the users' traffic leaves
// by the exit node without the transport.
type L2Standby struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	TransportID  string `gorm:"size:64;not null;index" json:"transport_id"`
	Tier         string `gorm:"size:16;not null" json:"tier"`
	SourceNodeID uint   `gorm:"not null;index" json:"source_node_id"`
	ExitNodeID   uint   `gorm:"not null;index" json:"exit_node_id"`
	// ExitHost is the address the source node dials for the reality tier;
	// the exit node's host when empty. The xhttp tier dials XHTTPDomain.
	ExitHost string `gorm:"size:255;not null;default:''" json:"exit_host"`
	// RelayPort is the exit node's relay port: public for reality, on
	// loopback behind the front for xhttp.
	RelayPort int `gorm:"not null" json:"relay_port"`
	// UUID is the VLESS user of the relay. It is only ever in the two
	// nodes' desired configurations, never in an answer.
	UUID string `gorm:"size:36;not null" json:"-"`
	// RealityPrivateKey (base64url x25519, secret) and RealityPublicKey;
	// RealityShortID is 16 hex digits; RealityServerName and RealityDest
	// are the borrowed site.
	RealityPrivateKey string `gorm:"size:64;not null;default:''" json:"-"`
	RealityPublicKey  string `gorm:"size:64;not null;default:''" json:"reality_public_key"`
	RealityShortID    string `gorm:"size:16;not null;default:''" json:"reality_short_id"`
	RealityServerName string `gorm:"size:255;not null;default:''" json:"reality_server_name"`
	RealityDest       string `gorm:"size:255;not null;default:''" json:"reality_dest"`
	// XHTTPDomain is our own domain on the exit node's front, XHTTPPath
	// the path the front proxies to the relay.
	XHTTPDomain string     `gorm:"column:xhttp_domain;size:255;not null;default:''" json:"xhttp_domain"`
	XHTTPPath   string     `gorm:"column:xhttp_path;size:64;not null;default:''" json:"xhttp_path"`
	Active      bool       `gorm:"not null;default:false" json:"active"`
	ActivatedAt *time.Time `json:"activated_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (L2Standby) TableName() string { return "v5_l2_standby" }
