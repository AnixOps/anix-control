package model

import "time"

// NodeTemporaryProxy is a SOCKS5 proxy a proxy node serves for a limited
// time on an internal address (internal/tempproxy): devices of that network
// (a mesh's virtual addresses, a LAN) send traffic out through the node
// until ExpiresAt. The node's Agent gets it in the member
// "temporary_proxies" of its desired configuration while it is open, and
// its kernel ends it at ExpiresAt by itself. ClosedAt is set when an
// administrator closed it early.
type NodeTemporaryProxy struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	NodeID uint `gorm:"not null;index" json:"node_id"`
	// Listen is the internal address the node listens on.
	Listen   string `gorm:"size:64;not null" json:"listen"`
	Port     int    `gorm:"not null" json:"port"`
	Username string `gorm:"size:64;not null" json:"username"`
	// Password is only ever in the node's desired configuration and in the
	// answer that created the proxy.
	Password  string     `gorm:"size:64;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	// CreatedBy is the administrator who opened it; nil for an API token
	// without a user.
	CreatedBy *uint     `json:"created_by"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (NodeTemporaryProxy) TableName() string { return "v5_node_temporary_proxy" }
