package model

import "time"

// AgentTransportMTLSStream is the Agent Control stream authenticated by a
// client certificate, the only transport an Agent connects on.
const AgentTransportMTLSStream = "mtls-stream"

// AgentTransport records when a node was last seen on one transport. The
// kernel keeps the live value in memory and writes a row at most about once
// a minute per node and transport (internal/agenttransport), so the table
// costs little however often agents poll.
type AgentTransport struct {
	NodeKind  string `gorm:"primaryKey;size:16" json:"node_kind"`
	NodeID    uint   `gorm:"primaryKey;autoIncrement:false" json:"node_id"`
	Transport string `gorm:"primaryKey;size:32" json:"transport"`
	// AgentVersion is what the agent reported on this transport (the
	// stream's Hello), empty where the transport carries none.
	AgentVersion string `gorm:"size:64;not null;default:''" json:"agent_version"`
	// Identity is the agent's SPIFFE ID; CertSerial and CertNotAfter
	// describe its certificate.
	Identity     string     `gorm:"size:255;not null;default:''" json:"identity"`
	CertSerial   string     `gorm:"size:40;not null;default:''" json:"cert_serial,omitempty"`
	CertNotAfter *time.Time `json:"cert_not_after,omitempty"`
	FirstSeenAt  time.Time  `gorm:"not null" json:"first_seen_at"`
	LastSeenAt   time.Time  `gorm:"not null;index" json:"last_seen_at"`
}

func (AgentTransport) TableName() string { return "v4_kernel_agent_transport" }
