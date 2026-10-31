package model

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// Approval states of a node (Node.Approval, ForwardNode.Approval).
const (
	// NodeApprovalApproved is every node an administrator created, every
	// node of a group without require_approval, and a pending node an
	// administrator approved. It is the column's default.
	NodeApprovalApproved = "approved"
	// NodeApprovalPending is a node that joined a group with
	// require_approval: it holds a certificate and connects, but Control
	// sends it no configuration (config.v1) and no users (users.v1).
	NodeApprovalPending = "pending"
	// NodeApprovalRejected is a pending node an administrator rejected: it
	// is disabled and its certificates are revoked. It no longer counts
	// toward its group.
	NodeApprovalRejected = "rejected"
)

// NodeGroupLiveNameIndex is the unique index on the names of live node
// groups. It is partial (WHERE deleted = 0), so a deleted group's name can
// be used again.
const NodeGroupLiveNameIndex = "ux_v5_node_group_live_name"

// NodeGroup owns a join token and the defaults of the nodes the token
// creates (docs/design/node-onboarding, "Node Groups"). Running the install
// command with the group's anixjoin_ token on a machine makes the machine a
// clean node of the group (a v2_node row without protocols).
//
// Rows are never removed. Deleting a group sets Deleted to 1 and DeletedAt;
// gorm.io/plugin/soft_delete then adds "deleted = 0" to every query, update
// and delete of this model, so a deleted group is invisible unless a query
// is Unscoped. A group with live nodes cannot be deleted.
type NodeGroup struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:100;not null;uniqueIndex:ux_v5_node_group_live_name,where:deleted = 0" json:"name"`
	// JoinTokenHash is the hex SHA-256 of the join token, the only value
	// used to find the group of a token.
	JoinTokenHash string `gorm:"size:64;not null;uniqueIndex" json:"-"`
	// JoinTokenSealed is the token sealed with AES-256-GCM under a key
	// derived from module_runtime.ca_kek, so an administrator can copy the
	// install command again (owner decision D8).
	JoinTokenSealed string     `gorm:"type:text;not null" json:"-"`
	TokenResetAt    time.Time  `gorm:"not null" json:"token_reset_at"`
	TokenExpiresAt  *time.Time `json:"token_expires_at"`
	// MaxNodes caps the group's live nodes; 0 is no cap.
	MaxNodes        int  `gorm:"not null;default:0" json:"max_nodes"`
	RequireApproval bool `gorm:"not null;default:false" json:"require_approval"`
	// Rate, Tags, MonthlyLimitBytes, ResetDay and Sort are copied to every
	// node the group creates Moving a node
	// to another group does not apply them again.
	Rate float64 `gorm:"not null;default:1" json:"rate"`
	// Tags is a JSON array of strings.
	Tags string `gorm:"type:text" json:"-"`
	// MonthlyLimitBytes is the monthly traffic limit; 0 is none.
	MonthlyLimitBytes int64                 `gorm:"not null;default:0" json:"monthly_limit_bytes"`
	ResetDay          int                   `gorm:"not null;default:1" json:"reset_day"`
	Sort              int                   `gorm:"not null;default:0" json:"sort"`
	Deleted           soft_delete.DeletedAt `gorm:"column:deleted;not null;default:0;softDelete:flag,DeletedAtField:DeletedAt" json:"-"`
	DeletedAt         *time.Time            `json:"deleted_at,omitempty"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

func (NodeGroup) TableName() string { return "v5_node_group" }
