package agentpki

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Revocation reasons recorded with revoked enrollments and certificates.
const (
	RevokeReasonNodeDisabled          = "node_disabled"
	RevokeReasonNodeDeleted           = "node_deleted"
	RevokeReasonCredentialsReplaced   = "credentials_replaced"
	RevokeReasonCredentialsRevoked    = "credentials_revoked"
	RevokeReasonCredentialsRotated    = "credentials_rotated"
	maxRevocationCacheEntriesBeforeGC = 4096
)

// revocationEpoch changes whenever this process revokes agent
// certificates, so cached answers are dropped at once instead of after the
// cache lifetime.
var revocationEpoch atomic.Uint64

// RevokeNode revokes every certificate and every enrollment (including
// unused enrollment credentials) of node, and its forward link
// certificates, in db, which may be a
// transaction. It is the hook of the kernel paths that revoke, replace or
// disable a node's credentials or delete the node. A database without the
// agent PKI tables has nothing to revoke.
func RevokeNode(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, reason string) error {
	if db == nil || !node.Valid() {
		return nil
	}
	db = db.WithContext(ctx)
	migrator := db.Migrator()
	if !migrator.HasTable(&model.AgentCertificate{}) || !migrator.HasTable(&model.AgentEnrollment{}) {
		return nil
	}
	hasLinkTable := migrator.HasTable(&model.ForwardLinkCertificate{})
	revocationEpoch.Add(1)
	// One transaction (a savepoint inside the caller's), holding the node's
	// row: the transactions that issue a credential of the node hold it too
	// (lockEnabledNode), so this waits for the ones in flight and then sees
	// the certificates and enrollments they recorded, and none can record a
	// live credential after the updates below. A node that is gone or
	// disabled is revoked all the same.
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockNode(tx, node); err != nil && !errors.Is(err, ErrInvalidNode) && !errors.Is(err, ErrNodeDisabled) {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{"revoked_at": now, "revoke_reason": reason}
		if err := tx.Model(&model.AgentCertificate{}).
			Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentEnrollment{}).
			Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		// The node's forward link certificates (link.go) go with its Agent
		// credentials. A database without the link table has none.
		if hasLinkTable {
			return tx.Model(&model.ForwardLinkCertificate{}).
				Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID).
				Updates(updates).Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	// A reader that cached "not revoked" between the first bump and these
	// writes is dropped again.
	revocationEpoch.Add(1)
	return nil
}

// ForgetRevocations drops every cached revocation answer of this process.
// RevokeNode does it too, but inside the caller's transaction: a reader
// between it and the commit caches "not revoked". A caller that revoked in a
// transaction calls this after the commit, so open Agent streams see the
// revocation at their next heartbeat instead of after the cache lifetime.
func ForgetRevocations() { revocationEpoch.Add(1) }

// ActiveCredentials counts what a revocation of a node's Agent credentials
// would revoke: certificates, enrollments (including unused enrollment
// credentials) and forward link certificates that are not revoked yet.
type ActiveCredentials struct {
	Certificates     int64 `json:"certificates"`
	Enrollments      int64 `json:"enrollments"`
	LinkCertificates int64 `json:"link_certificates"`
}

// CountActive counts node's unrevoked Agent credentials in db, which may be
// a transaction. A database without the agent PKI tables has none.
func CountActive(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode) (ActiveCredentials, error) {
	var counts ActiveCredentials
	if db == nil || !node.Valid() {
		return counts, nil
	}
	db = db.WithContext(ctx)
	migrator := db.Migrator()
	if !migrator.HasTable(&model.AgentCertificate{}) || !migrator.HasTable(&model.AgentEnrollment{}) {
		return counts, nil
	}
	count := func(into *int64, table any) error {
		return db.Model(table).Where("node_kind = ? AND node_id = ? AND revoked_at IS NULL", node.Kind, node.ID).Count(into).Error
	}
	if err := count(&counts.Certificates, &model.AgentCertificate{}); err != nil {
		return ActiveCredentials{}, err
	}
	if err := count(&counts.Enrollments, &model.AgentEnrollment{}); err != nil {
		return ActiveCredentials{}, err
	}
	if migrator.HasTable(&model.ForwardLinkCertificate{}) {
		if err := count(&counts.LinkCertificates, &model.ForwardLinkCertificate{}); err != nil {
			return ActiveCredentials{}, err
		}
	}
	return counts, nil
}

// IsRevoked reports whether the agent certificate serial of node may no
// longer authenticate. A serial without a record, or recorded for another
// node, counts as revoked: every agent certificate is recorded when it is
// issued, and records are pruned only after the certificate expired.
// Answers are cached for at most the revocation cache lifetime.
func (s *Service) IsRevoked(ctx context.Context, serial string, node agentcontrol.AgentNode) (bool, error) {
	if revoked, ok := s.revocations.get(serial, node); ok {
		return revoked, nil
	}
	epoch := revocationEpoch.Load()
	var record model.AgentCertificate
	err := s.db.WithContext(ctx).Where("serial = ?", serial).First(&record).Error
	revoked := false
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		revoked = true
	case err != nil:
		return false, err
	default:
		revoked = record.RevokedAt != nil || record.NodeKind != node.Kind || record.NodeID != uint(node.ID) ||
			record.Cluster != s.cluster
	}
	s.revocations.put(serial, node, revoked, epoch)
	return revoked, nil
}

// revocationCache remembers revocation answers per serial and node.
type revocationCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[revocationKey]revocationEntry
}

type revocationKey struct {
	serial string
	node   agentcontrol.AgentNode
}

type revocationEntry struct {
	revoked   bool
	checkedAt time.Time
	epoch     uint64
}

func newRevocationCache(ttl time.Duration, now func() time.Time) *revocationCache {
	return &revocationCache{ttl: ttl, now: now, entries: map[revocationKey]revocationEntry{}}
}

func (c *revocationCache) get(serial string, node agentcontrol.AgentNode) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[revocationKey{serial: serial, node: node}]
	if !ok || entry.epoch != revocationEpoch.Load() || c.now().Sub(entry.checkedAt) >= c.ttl {
		return false, false
	}
	return entry.revoked, true
}

func (c *revocationCache) put(serial string, node agentcontrol.AgentNode, revoked bool, epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.entries) >= maxRevocationCacheEntriesBeforeGC {
		for key, entry := range c.entries {
			if entry.epoch != revocationEpoch.Load() || now.Sub(entry.checkedAt) >= c.ttl {
				delete(c.entries, key)
			}
		}
	}
	c.entries[revocationKey{serial: serial, node: node}] = revocationEntry{revoked: revoked, checkedAt: now, epoch: epoch}
}

// Prune deletes the records of certificates (Agent and, with the link CA,
// forward link certificates) that expired before cutoff and of enrollment
// credentials that expired unused before it.
func (s *Service) Prune(ctx context.Context, cutoff time.Time) error {
	db := s.db.WithContext(ctx)
	if err := db.Where("not_after < ?", cutoff).Delete(&model.AgentCertificate{}).Error; err != nil {
		return err
	}
	if s.link != nil {
		if err := db.Where("not_after < ?", cutoff).Delete(&model.ForwardLinkCertificate{}).Error; err != nil {
			return err
		}
	}
	return db.Where("method = ? AND used_at IS NULL AND expires_at < ?", model.AgentEnrollmentMethodCredential, cutoff).
		Delete(&model.AgentEnrollment{}).Error
}
