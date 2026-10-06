package agentpki

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Audit actions written to the operation log (v2_operation_log).
const (
	auditModule            = "agent_pki"
	AuditActionTokenIssue  = "agent_enrollment_token_issue"
	AuditActionEnroll      = "agent_enroll"
	auditTargetProxyNode   = "proxy_node"
	auditTargetForwardNode = "forward_node"
)

// TokenRequest describes a new one-time enrollment credential.
type TokenRequest struct {
	Node agentcontrol.AgentNode
	// TTL defaults to DefaultEnrollmentCredentialTTL and may not exceed
	// MaxEnrollmentCredentialTTL.
	TTL time.Duration
	// CreatedBy is the administrator's user id; 0 for the command line.
	CreatedBy uint
	// Actor and IP describe the caller in the audit entry.
	Actor string
	IP    string
}

// CreateEnrollmentToken stores a one-time enrollment credential bound to a
// node and returns it. The credential is shown only here; the kernel keeps
// its SHA-256.
func (s *Service) CreateEnrollmentToken(ctx context.Context, request TokenRequest) (string, model.AgentEnrollment, error) {
	var (
		credential string
		row        model.AgentEnrollment
	)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		credential, row, err = s.CreateEnrollmentTokenTx(tx, request)
		return err
	})
	if err != nil {
		return "", model.AgentEnrollment{}, err
	}
	return credential, row, nil
}

// CreateEnrollmentTokenTx is CreateEnrollmentToken in the caller's
// transaction: the credential, its row and its audit entry commit with
// whatever else the caller changed (the credential rotation revokes the
// node's old credentials in the same transaction).
func (s *Service) CreateEnrollmentTokenTx(tx *gorm.DB, request TokenRequest) (string, model.AgentEnrollment, error) {
	if !request.Node.Valid() {
		return "", model.AgentEnrollment{}, fmt.Errorf("%w: node %q", agentcontrol.ErrInvalidAgentIdentity, request.Node.String())
	}
	ttl := request.TTL
	if ttl == 0 {
		ttl = DefaultEnrollmentCredentialTTL
	}
	if ttl < time.Second || ttl > MaxEnrollmentCredentialTTL {
		return "", model.AgentEnrollment{}, fmt.Errorf("enrollment credential lifetime must be between 1s and %s", MaxEnrollmentCredentialTTL)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", model.AgentEnrollment{}, err
	}
	credential := agentcontrol.EnrollmentCredentialPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash := hashSecret(credential)
	now := s.now().UTC()
	expires := now.Add(ttl)
	row := model.AgentEnrollment{
		ID: uuid.NewString(), NodeKind: request.Node.Kind, NodeID: uint(request.Node.ID), Cluster: s.cluster,
		Method: model.AgentEnrollmentMethodCredential, CredentialHash: &hash, ExpiresAt: &expires,
		CreatedBy: request.CreatedBy, CreatedAt: now,
	}
	if err := requireEnabledNode(tx, request.Node); err != nil {
		return "", model.AgentEnrollment{}, err
	}
	if err := tx.Create(&row).Error; err != nil {
		return "", model.AgentEnrollment{}, err
	}
	createdBy := request.CreatedBy
	var userID *uint
	if createdBy != 0 {
		userID = &createdBy
	}
	if err := writeAudit(tx, auditEntry{
		UserID: userID, Actor: request.Actor, Action: AuditActionTokenIssue, Node: request.Node, IP: request.IP,
		Content: map[string]any{
			"node": request.Node.String(), "enrollment_id": row.ID, "expires_at": expires.Format(time.RFC3339),
		},
	}); err != nil {
		return "", model.AgentEnrollment{}, err
	}
	return credential, row, nil
}

// Bootstrap is the credential an agent enrolls with.
type Bootstrap struct {
	// Method is one of model.AgentEnrollmentMethod*.
	Method string
	// Node is the node the agent claims: required with a node API key or a
	// forward token; with an enrollment credential it must match the
	// credential's node when set.
	Node agentcontrol.AgentNode
	// Secret is the API key, the forward token or the enrollment
	// credential. It is never stored or logged.
	Secret string
}

// EnrollRequest is an agent's request for its first certificate.
type EnrollRequest struct {
	Bootstrap    Bootstrap
	CSRDER       []byte
	AgentVersion string
	InstanceID   string
	// RemoteAddr is recorded in the audit entry.
	RemoteAddr string
}

// Issued is a signed agent certificate.
type Issued struct {
	modulepki.SignedLeaf
	Identity     agentcontrol.AgentIdentity
	EnrollmentID string
}

// Enroll checks a bootstrap credential and issues the authenticated node's
// first certificate. The CSR's subject and SANs are ignored. A one-time
// credential is consumed atomically; a failed issuance leaves it unused.
func (s *Service) Enroll(ctx context.Context, request EnrollRequest) (Issued, error) {
	bootstrap := request.Bootstrap
	if bootstrap.Secret == "" {
		return Issued{}, ErrEnrollmentRejected
	}
	var issued Issued
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		var (
			node       agentcontrol.AgentNode
			enrollment model.AgentEnrollment
		)
		switch bootstrap.Method {
		case model.AgentEnrollmentMethodCredential:
			consumed, err := s.consumeCredential(tx, bootstrap, now)
			if err != nil {
				return err
			}
			enrollment, node = consumed, agentcontrol.AgentNode{Kind: consumed.NodeKind, ID: uint32(consumed.NodeID)} // #nosec G115 -- node ids are stored from uint32 values.
		case model.AgentEnrollmentMethodNodeAPIKey, model.AgentEnrollmentMethodForwardToken:
			if err := lockEnabledNode(tx, bootstrap.Node); err != nil {
				return ErrEnrollmentRejected
			}
			if err := authenticateNodeCredential(tx, bootstrap); err != nil {
				return err
			}
			node = bootstrap.Node
			enrollment = model.AgentEnrollment{
				ID: uuid.NewString(), NodeKind: node.Kind, NodeID: uint(node.ID), Cluster: s.cluster,
				Method: bootstrap.Method, UsedAt: &now, CreatedAt: now,
			}
		default:
			return ErrEnrollmentRejected
		}
		if err := lockEnabledNode(tx, node); err != nil {
			return ErrEnrollmentRejected
		}
		enrollment.AgentVersion = truncate(request.AgentVersion, 64)
		enrollment.InstanceID = truncate(request.InstanceID, 128)
		if enrollment.Method == model.AgentEnrollmentMethodCredential {
			if err := tx.Model(&model.AgentEnrollment{}).Where("id = ?", enrollment.ID).Updates(map[string]any{
				"agent_version": enrollment.AgentVersion, "instance_id": enrollment.InstanceID,
			}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&enrollment).Error; err != nil {
			return err
		}
		signed, err := s.issue(ctx, tx, node, enrollment.ID, request.CSRDER)
		if err != nil {
			return err
		}
		issued = signed
		return writeAudit(tx, auditEntry{
			Actor: "agent:" + node.String(), Action: AuditActionEnroll, Node: node, IP: request.RemoteAddr,
			Content: map[string]any{
				"node": node.String(), "enrollment_id": enrollment.ID, "method": enrollment.Method,
				"serial": signed.Serial, "not_after": signed.NotAfter.Format(time.RFC3339),
				"agent_version": enrollment.AgentVersion, "instance_id": enrollment.InstanceID,
			},
		})
	})
	return issued, err
}

// consumeCredential marks a one-time credential used. The conditional update
// is the only check that matters under concurrency: of two enrollments with
// one credential, exactly one updates the row.
func (s *Service) consumeCredential(tx *gorm.DB, bootstrap Bootstrap, now time.Time) (model.AgentEnrollment, error) {
	if !strings.HasPrefix(bootstrap.Secret, agentcontrol.EnrollmentCredentialPrefix) {
		return model.AgentEnrollment{}, ErrEnrollmentRejected
	}
	var row model.AgentEnrollment
	if err := tx.Where("credential_hash = ?", hashSecret(bootstrap.Secret)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AgentEnrollment{}, ErrEnrollmentRejected
		}
		return model.AgentEnrollment{}, err
	}
	if row.Method != model.AgentEnrollmentMethodCredential || row.Cluster != s.cluster {
		return model.AgentEnrollment{}, ErrEnrollmentRejected
	}
	if bootstrap.Node != (agentcontrol.AgentNode{}) && (bootstrap.Node.Kind != row.NodeKind || uint(bootstrap.Node.ID) != row.NodeID) {
		return model.AgentEnrollment{}, ErrEnrollmentRejected
	}
	// The node's row first: a revocation of the node then either finished
	// (the update below finds the credential revoked) or waits for this
	// enrollment and revokes what it records.
	if err := lockEnabledNode(tx, agentcontrol.AgentNode{Kind: row.NodeKind, ID: uint32(row.NodeID)}); err != nil { // #nosec G115 -- node ids are stored from uint32 values.
		return model.AgentEnrollment{}, ErrEnrollmentRejected
	}
	result := tx.Model(&model.AgentEnrollment{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", row.ID, now).
		Update("used_at", now)
	if result.Error != nil {
		return model.AgentEnrollment{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.AgentEnrollment{}, ErrEnrollmentRejected
	}
	row.UsedAt = &now
	return row, nil
}

// authenticateNodeCredential checks a proxy node's API key or a forward
// node's token against the node the agent claims, as the legacy transports
// do. Both are read through the node credential split, which applies the
// table's phase and never matches a tombstone or the placeholder.
func authenticateNodeCredential(tx *gorm.DB, bootstrap Bootstrap) error {
	node := bootstrap.Node
	switch {
	case bootstrap.Method == model.AgentEnrollmentMethodNodeAPIKey && node.Kind == agentcontrol.NodeKindProxy && node.ID > 0:
		row, err := nodesecrets.NodeByAPIKey(tx, bootstrap.Secret, false)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnrollmentRejected
			}
			return err
		}
		if row.ID != uint(node.ID) {
			return ErrEnrollmentRejected
		}
		return nil
	case bootstrap.Method == model.AgentEnrollmentMethodForwardToken && node.Kind == agentcontrol.NodeKindForward && node.ID > 0:
		var row model.ForwardNode
		if err := tx.Select("id", "api_token").Where("id = ?", node.ID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnrollmentRejected
			}
			return err
		}
		if !nodesecrets.ForwardNodeTokenMatches(tx, &row, bootstrap.Secret) {
			return ErrEnrollmentRejected
		}
		return nil
	default:
		return ErrEnrollmentRejected
	}
}

// lockEnabledNode is requireEnabledNode that also locks the node's row until
// tx ends (PostgreSQL row lock; SQLite has one writer). Every transaction
// that issues a credential of a node takes it first, and RevokeNode takes it
// too, so a revocation waits for the issuances in flight and sees what they
// recorded, and an issuance that starts later reads the revocation. Without
// it, PostgreSQL's per-statement snapshots let an issuance that began before
// a revocation committed record a live credential after it.
func lockEnabledNode(tx *gorm.DB, node agentcontrol.AgentNode) error {
	return requireEnabledNode(tx.Clauses(clause.Locking{Strength: "UPDATE"}), node)
}

// requireEnabledNode fails unless node exists and is not disabled.
func requireEnabledNode(tx *gorm.DB, node agentcontrol.AgentNode) error {
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		var row model.Node
		if err := tx.Select("id", "status").Where("id = ?", node.ID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidNode
			}
			return err
		}
		if row.Status == model.NodeStatusDisabled {
			return ErrInvalidNode
		}
		return nil
	case agentcontrol.NodeKindForward:
		var row model.ForwardNode
		if err := tx.Select("id", "enabled").Where("id = ?", node.ID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidNode
			}
			return err
		}
		if !row.Enabled {
			return ErrInvalidNode
		}
		return nil
	default:
		return ErrInvalidNode
	}
}

// Renew issues a new certificate to the holder of a valid agent
// certificate. peer must already be verified (VerifyPeer).
func (s *Service) Renew(ctx context.Context, peer *x509.Certificate, csrDER []byte) (Issued, error) {
	identity, err := agentcontrol.AgentIdentityFromCertificate(peer)
	if err != nil {
		return Issued{}, err
	}
	if identity.Cluster != s.cluster {
		return Issued{}, ErrCertificateRevoked
	}
	var issued Issued
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The node's row first (see lockEnabledNode): the records below are
		// read after any revocation in flight has committed.
		if err := lockEnabledNode(tx, identity.Node); err != nil {
			if errors.Is(err, ErrInvalidNode) {
				return ErrCertificateRevoked
			}
			return err
		}
		var record model.AgentCertificate
		if err := tx.First(&record, "serial = ?", modulepki.SerialString(peer.SerialNumber)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCertificateRevoked
			}
			return err
		}
		if record.RevokedAt != nil || record.NodeKind != identity.Node.Kind || record.NodeID != uint(identity.Node.ID) ||
			record.Cluster != s.cluster {
			return ErrCertificateRevoked
		}
		var enrollment model.AgentEnrollment
		if err := tx.First(&enrollment, "id = ?", record.EnrollmentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCertificateRevoked
			}
			return err
		}
		if enrollment.RevokedAt != nil {
			return ErrCertificateRevoked
		}
		signed, err := s.issue(ctx, tx, identity.Node, record.EnrollmentID, csrDER)
		if err != nil {
			return err
		}
		issued = signed
		return nil
	})
	return issued, err
}

// issue signs and records a certificate for node.
func (s *Service) issue(ctx context.Context, tx *gorm.DB, node agentcontrol.AgentNode, enrollmentID string, csrDER []byte) (Issued, error) {
	identity, err := agentcontrol.NewAgentIdentity(s.cluster, node)
	if err != nil {
		return Issued{}, err
	}
	leaf, err := s.authority.SignLeaf(ctx, tx, identity.URL(), s.lifetime, csrDER)
	if err != nil {
		return Issued{}, err
	}
	if err := tx.Create(&model.AgentCertificate{
		Serial: leaf.Serial, NodeKind: node.Kind, NodeID: uint(node.ID), Cluster: s.cluster, EnrollmentID: enrollmentID,
		IssuerKeyID: leaf.IssuerKeyID, NotAfter: leaf.NotAfter, CreatedAt: s.now().UTC(),
	}).Error; err != nil {
		return Issued{}, err
	}
	return Issued{SignedLeaf: leaf, Identity: identity, EnrollmentID: enrollmentID}, nil
}

type auditEntry struct {
	UserID  *uint
	Actor   string
	Action  string
	Node    agentcontrol.AgentNode
	IP      string
	Content map[string]any
}

// writeAudit records an agent PKI event in the operation log. Entries never
// carry credentials, keys or certificates.
func writeAudit(tx *gorm.DB, entry auditEntry) error {
	content, err := json.Marshal(entry.Content)
	if err != nil {
		return err
	}
	targetType := auditTargetProxyNode
	if entry.Node.Kind == agentcontrol.NodeKindForward {
		targetType = auditTargetForwardNode
	}
	targetID := uint(entry.Node.ID)
	return tx.Create(&model.OperationLog{
		UserID: entry.UserID, Username: truncate(entry.Actor, 100), Action: entry.Action, Module: auditModule,
		TargetType: targetType, TargetID: &targetID, Content: string(content), IP: truncate(entry.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// CheckNodeEnabled fails with ErrInvalidNode unless node exists and is not
// disabled.
func CheckNodeEnabled(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode) error {
	return requireEnabledNode(db.WithContext(ctx), node)
}
