package agentpki

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AuditActionCredentialsRotate is the operation log action of a credential
// rotation (RotateCredentials). Its entry carries the node, the reason and
// what was revoked; never a credential.
const AuditActionCredentialsRotate = "agent_credentials_rotate"

// MaxRotationReasonLength bounds the free-text reason an administrator
// gives for a rotation.
const MaxRotationReasonLength = 200

// ErrNodeDisabled reports a rotation of a node an administrator disabled:
// disabling already revoked its Agent credentials, and a disabled node is
// not enrolled until it is enabled again.
var ErrNodeDisabled = errors.New("agent node is disabled")

// RotateRequest rotates a node's Agent credentials.
type RotateRequest struct {
	Node agentcontrol.AgentNode
	// TTL is the new enrollment credential's lifetime; as TokenRequest.
	TTL time.Duration
	// CreatedBy is the administrator's user id; Actor and IP describe the
	// caller in the audit entries.
	CreatedBy uint
	Actor     string
	IP        string
	// Reason is recorded in the audit entry (at most MaxRotationReasonLength
	// bytes, checked by the caller).
	Reason string
	// ReplaceNodeKey, when set, runs first in the rotation's transaction and
	// replaces the node's own credential (a proxy node's API key). It must
	// not revoke Agent credentials itself: the rotation does that, once, with
	// its own reason. Its failure rolls the whole rotation back.
	ReplaceNodeKey func(tx *gorm.DB) error
}

// Rotation is what RotateCredentials did.
type Rotation struct {
	// Credential is the new one-time enrollment credential. It is shown
	// once: only its SHA-256 is stored.
	Credential string
	Enrollment model.AgentEnrollment
	// Revoked counts the Agent credentials the rotation revoked.
	Revoked       ActiveCredentials
	APIKeyRotated bool
}

// RotateCredentials rotates a node's Agent credentials in one transaction:
// it optionally replaces the node's API key (req.ReplaceNodeKey), revokes
// every Agent certificate, enrollment (unused enrollment credentials
// included) and forward link certificate of the node (RevokeNode, reason
// credentials_rotated), and then issues a fresh one-time enrollment
// credential, the same code path as the enrollment token administration
// (CreateEnrollmentTokenTx). The revocation comes first because it also
// revokes unused enrollment credentials, and the new one must survive it.
//
// It never dials the node: an offline node is rotated like any other, and
// its Agent finds out when it next presents the revoked certificate. Open
// Agent streams end at their next heartbeat. The node row is locked, so
// concurrent rotations of one node run one after the other, and a repeated
// rotation leaves exactly one valid enrollment credential: the last.
// ErrInvalidNode and ErrNodeDisabled report a node that is gone or disabled;
// nothing is changed then.
func (s *Service) RotateCredentials(ctx context.Context, req RotateRequest) (Rotation, error) {
	if !req.Node.Valid() {
		return Rotation{}, fmt.Errorf("%w: node %q", agentcontrol.ErrInvalidAgentIdentity, req.Node.String())
	}
	var rotation Rotation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockNode(tx, req.Node); err != nil {
			return err
		}
		revoked, err := CountActive(ctx, tx, req.Node)
		if err != nil {
			return err
		}
		if req.ReplaceNodeKey != nil {
			if err := req.ReplaceNodeKey(tx); err != nil {
				return err
			}
		}
		if err := RevokeNode(ctx, tx, req.Node, RevokeReasonCredentialsRotated); err != nil {
			return err
		}
		credential, enrollment, err := s.CreateEnrollmentTokenTx(tx, TokenRequest{
			Node: req.Node, TTL: req.TTL, CreatedBy: req.CreatedBy, Actor: req.Actor, IP: req.IP,
		})
		if err != nil {
			return err
		}
		var userID *uint
		if req.CreatedBy != 0 {
			createdBy := req.CreatedBy
			userID = &createdBy
		}
		content := map[string]any{
			"node": req.Node.String(), "enrollment_id": enrollment.ID, "api_key_rotated": req.ReplaceNodeKey != nil,
			"revoked_certificates": revoked.Certificates, "revoked_enrollments": revoked.Enrollments,
			"revoked_link_certificates": revoked.LinkCertificates, "reason": strings.TrimSpace(req.Reason),
		}
		if err := writeAudit(tx, auditEntry{
			UserID: userID, Actor: req.Actor, Action: AuditActionCredentialsRotate, Node: req.Node, IP: req.IP, Content: content,
		}); err != nil {
			return err
		}
		rotation = Rotation{Credential: credential, Enrollment: enrollment, Revoked: revoked, APIKeyRotated: req.ReplaceNodeKey != nil}
		return nil
	})
	if err != nil {
		return Rotation{}, err
	}
	ForgetRevocations()
	return rotation, nil
}

// lockNode reads node's row for update (PostgreSQL row lock; SQLite has one
// writer) and fails with ErrInvalidNode when it does not exist and with
// ErrNodeDisabled when it is disabled.
func lockNode(tx *gorm.DB, node agentcontrol.AgentNode) error {
	lock := clause.Locking{Strength: "UPDATE"}
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		var row model.Node
		if err := tx.Clauses(lock).Select("id", "status").Where("id = ?", node.ID).First(&row).Error; err != nil {
			return missingNode(err)
		}
		if row.Status == model.NodeStatusDisabled {
			return ErrNodeDisabled
		}
		return nil
	case agentcontrol.NodeKindForward:
		var row model.ForwardNode
		if err := tx.Clauses(lock).Select("id", "enabled").Where("id = ?", node.ID).First(&row).Error; err != nil {
			return missingNode(err)
		}
		if !row.Enabled {
			return ErrNodeDisabled
		}
		return nil
	default:
		return ErrInvalidNode
	}
}

func missingNode(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrInvalidNode
	}
	return err
}
