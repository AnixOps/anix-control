package agentpki

import (
	"context"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Node group joins (AgentEnrollment.Join, internal/nodegroup).
const (
	// AuditActionJoin is the operation log action of a machine that joined
	// a node group with its join token: as a new node, or rejoining its
	// earlier node. Its entry names the node, the group and the
	// certificate, never the token.
	AuditActionJoin = "agent_join"
	// RevokeReasonRejoined revokes the certificates of a node whose machine
	// rejoined it with the group's join token (it lost its local state).
	RevokeReasonRejoined = "rejoined"
)

// JoinIssue is the certificate of a node that a join token created, or of
// the earlier node a machine rejoined.
type JoinIssue struct {
	Node   agentcontrol.AgentNode
	CSRDER []byte
	// AgentVersion and InstanceID are recorded with the enrollment.
	AgentVersion string
	InstanceID   string
	// RemoteAddr is recorded in the audit entry.
	RemoteAddr string
	// Audit is added to the content of the agent_join entry (the group,
	// whether the node was created). It must not carry the token.
	Audit map[string]any
}

// IssueJoinTx records a join_token enrollment of request.Node and issues
// its certificate exactly as Enroll does, with the agent_join audit entry,
// in the caller's transaction: the node a join creates, its protocols and
// its certificate commit together or not at all. The node's row is locked
// first (lockEnabledNode), so a revocation of the node orders with the
// issuance. ErrInvalidNode reports a node that is gone or disabled.
func (s *Service) IssueJoinTx(ctx context.Context, tx *gorm.DB, request JoinIssue) (Issued, error) {
	node := request.Node
	if !node.Valid() {
		return Issued{}, fmt.Errorf("%w: node %q", agentcontrol.ErrInvalidAgentIdentity, node.String())
	}
	if err := lockEnabledNode(tx, node); err != nil {
		return Issued{}, err
	}
	now := s.now().UTC()
	enrollment := model.AgentEnrollment{
		ID: uuid.NewString(), NodeKind: node.Kind, NodeID: uint(node.ID), Cluster: s.cluster,
		Method: model.AgentEnrollmentMethodJoinToken, UsedAt: &now, CreatedAt: now,
		AgentVersion: truncate(request.AgentVersion, 64), InstanceID: truncate(request.InstanceID, 128),
	}
	if err := tx.Create(&enrollment).Error; err != nil {
		return Issued{}, err
	}
	signed, err := s.issue(ctx, tx, node, enrollment.ID, request.CSRDER)
	if err != nil {
		return Issued{}, err
	}
	content := map[string]any{}
	for key, value := range request.Audit {
		content[key] = value
	}
	content["node"] = node.String()
	content["enrollment_id"] = enrollment.ID
	content["method"] = enrollment.Method
	content["serial"] = signed.Serial
	content["not_after"] = signed.NotAfter.Format(time.RFC3339)
	content["agent_version"] = enrollment.AgentVersion
	content["instance_id"] = enrollment.InstanceID
	if err := writeAudit(tx, auditEntry{
		Actor: "agent:" + node.String(), Action: AuditActionJoin, Node: node, IP: request.RemoteAddr, Content: content,
	}); err != nil {
		return Issued{}, err
	}
	return signed, nil
}
