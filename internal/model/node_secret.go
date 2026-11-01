package model

import "time"

// ProtocolSecret is one secret of a node protocol's settings, a node's raw
// configuration or a WireGuard peer, moved out of the legacy tables by the
// node credential split.
//
// JSONPointer (RFC 6901) names the secret's position in the column's JSON
// document, and Value holds the JSON encoding of the value there. An empty
// JSONPointer stands for the whole column: a WireGuard peer's key, or a
// column whose text is not JSON; Value then holds the text itself.
type ProtocolSecret struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Scope       string `gorm:"size:32;not null;uniqueIndex:idx_v4_kernel_protocol_secret_position,priority:1" json:"scope"`
	OwnerID     uint64 `gorm:"not null;uniqueIndex:idx_v4_kernel_protocol_secret_position,priority:2" json:"owner_id"`
	ColumnName  string `gorm:"size:64;not null;uniqueIndex:idx_v4_kernel_protocol_secret_position,priority:3" json:"column_name"`
	JSONPointer string `gorm:"column:json_pointer;not null;default:'';uniqueIndex:idx_v4_kernel_protocol_secret_position,priority:4" json:"json_pointer"`
	Value       string `gorm:"type:text;not null;default:''" json:"-"`
	Sealed      bool   `gorm:"not null;default:false" json:"sealed"`
	KEKID       string `gorm:"column:kek_id;size:64;not null;default:''" json:"kek_id"`
	// Version counts the changes of the value at this position.
	Version   int       `gorm:"not null;default:1" json:"version"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (ProtocolSecret) TableName() string { return "v4_kernel_protocol_secret" }

// NodeSecretSplit is the state of the credential split of one legacy table:
// its phase (legacy, dual_write, dual_read, finalized), the progress of the
// backfill, and the outcome of the last verification.
type NodeSecretSplit struct {
	Table string `gorm:"column:table_name;primaryKey;size:64" json:"table_name"`
	Phase string `gorm:"size:16;not null" json:"phase"`
	// BackfilledRows counts the legacy rows the last backfill pass copied.
	BackfilledRows int64 `gorm:"not null;default:0" json:"backfilled_rows"`
	// BackfillCursor is the last legacy id of the running backfill pass, so
	// an interrupted pass resumes after it.
	BackfillCursor uint64 `gorm:"not null;default:0" json:"backfill_cursor"`
	// BackfilledAt is when the last backfill pass completed; nil while one
	// runs.
	BackfilledAt *time.Time `json:"backfilled_at"`
	// Digest is the digest of every secret of the table, old and new forms
	// equal, at the last verification that matched (VerifiedAt).
	Digest     string     `gorm:"size:64;not null;default:''" json:"digest"`
	VerifiedAt *time.Time `json:"verified_at"`
	// CheckedAt and Mismatches record the last verification, matched or not.
	CheckedAt   *time.Time `json:"checked_at"`
	Mismatches  int64      `gorm:"not null;default:0" json:"mismatches"`
	FinalizedAt *time.Time `json:"finalized_at"`
	FinalizedBy string     `gorm:"size:128;not null;default:''" json:"finalized_by"`
	UpdatedAt   time.Time  `gorm:"not null" json:"updated_at"`
}

func (NodeSecretSplit) TableName() string { return "v4_kernel_node_secret_split" }
