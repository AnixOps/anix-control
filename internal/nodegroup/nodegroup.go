// Package nodegroup is node onboarding by node group (5.0,
// docs/design/node-onboarding): node groups and their join tokens, the
// protocol templates of their proxy nodes, AgentEnrollment.Join, which turns
// a machine into a node of the group whose token it presents, and the
// approval of the nodes that joined a group requiring it.
//
// A node group (v5_node_group) owns one anixjoin_ token and the defaults of
// the nodes it creates. The token is stored as its SHA-256 and sealed under
// a key derived from module_runtime.ca_kek (token.go), so a super
// administrator can copy the install command again; resetting it
// invalidates every earlier command. Deleting a group is a soft delete,
// refused while the group has live nodes.
package nodegroup

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

var (
	// ErrNotFound: no live node group with that id.
	ErrNotFound = errors.New("node group not found")
	// ErrInvalid: a group or template field is invalid; the error says
	// which.
	ErrInvalid = errors.New("invalid node group")
	// ErrNameTaken: a live node group has the name.
	ErrNameTaken = errors.New("a node group with this name exists")
	// ErrInUse: the group still has live nodes, so it cannot be deleted.
	ErrInUse = errors.New("the node group has live nodes")
	// ErrNoKEK: join tokens are sealed with module_runtime.ca_kek, which is
	// not set (or not a 32-byte key).
	ErrNoKEK = errors.New("node group join tokens are sealed with module_runtime.ca_kek: set it (the built-in agent CA needs it too)")
	// ErrJoinRejected is the single answer to an unusable join token:
	// unknown, of a deleted group, expired, or of a group at its node cap.
	// The audit entry names the reason; the caller never learns it.
	ErrJoinRejected = errors.New("join rejected")
	// ErrInvalidJoin: a join request field is malformed.
	ErrInvalidJoin = errors.New("invalid join request")
	// ErrJoinTemplate: the group's protocol template no longer builds a
	// valid protocol (a subscription group or a setting changed meanwhile).
	ErrJoinTemplate = errors.New("the node group's protocol template is invalid")
	// ErrNodeNotFound: no node with that name.
	ErrNodeNotFound = errors.New("node not found")
	// ErrNotPending: the node is not pending approval.
	ErrNotPending = errors.New("the node is not pending approval")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Audit actions of the operation log (v2_operation_log, module node_group).
// No entry carries a join token.
const (
	auditModule                   = "node_group"
	auditTargetGroup              = "node_group"
	AuditActionCreate             = "node_group_create"
	AuditActionUpdate             = "node_group_update"
	AuditActionDelete             = "node_group_delete"
	AuditActionTokenReset         = "node_group_join_token_reset"  // #nosec G101 -- names an operation log action, not a credential.
	AuditActionTokenReveal        = "node_group_join_token_reveal" // #nosec G101 -- names an operation log action, not a credential.
	AuditActionJoinRejected       = "agent_join_rejected"
	AuditActionNodeApprove        = "node_approve"
	AuditActionNodeReject         = "node_reject"
	auditTargetProxyNode          = "proxy_node"
	auditTargetForwardNode        = "forward_node"
	auditJoinRejectedActorUnknown = "agent:unknown"
)

// Service manages node groups and joins on one database.
type Service struct {
	DB *gorm.DB
	// KEK answers the key-encryption key that seals join tokens;
	// module_runtime.ca_kek of the loaded configuration when nil.
	KEK func() string
	// Now defaults to time.Now.
	Now func() time.Time
	// PKI issues the certificates of joining machines; Join needs it.
	PKI *agentpki.Service
	// OnJoin, when set, is called in its own goroutine after a join
	// committed (the administrators' notification). It must not block the
	// join, and a failure is its own to report.
	OnJoin func(Joined)
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// writeAudit records an entry in the operation log.
func writeAudit(tx *gorm.DB, actor Actor, action, targetType string, targetID uint, content map[string]any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	var target *uint
	if targetID != 0 {
		target = &targetID
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: targetType, TargetID: target, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
