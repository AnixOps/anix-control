// Package agentreports keeps the record of report batches the Agent Control
// stream applied (reports.v1, node-ops-service.md section 5.5), so that a
// batch an agent resends is applied at most once per node.
package agentreports

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Batch kinds.
const (
	KindTraffic = "traffic"
	KindLogs    = "logs"
)

// MaxBatchIDLength bounds a batch id, as the subscriber request ledger
// bounds its request ids.
const MaxBatchIDLength = 128

// Retention is how long a batch id is remembered. Batch ids carry the
// agent's boot id and a sequence, so one never repeats; the record only has
// to outlive the agent's spool, which resends on the stream as soon as it
// reconnects.
const Retention = 7 * 24 * time.Hour

// ErrInvalidBatchID means a batch id is empty or longer than
// MaxBatchIDLength; the batch is refused for good.
var ErrInvalidBatchID = errors.New("batch_id is required and at most 128 bytes")

// ValidateBatchID checks a batch id's bounds.
func ValidateBatchID(batchID string) error {
	if batchID == "" || len(batchID) > MaxBatchIDLength {
		return ErrInvalidBatchID
	}
	return nil
}

// ClaimTx records that this transaction applies batchID for the node and
// reports whether a committed transaction recorded it before (seen). The
// caller applies the batch only when seen is false, in the same tx, so a
// failure rolls the claim back with the batch and a retry applies it. Two
// concurrent deliveries of one batch serialize on the row: the second sees
// the first's claim once it commits.
func ClaimTx(tx *gorm.DB, nodeKind string, nodeID uint, batchID, kind string, now time.Time) (seen bool, err error) {
	if err := ValidateBatchID(batchID); err != nil {
		return false, err
	}
	if nodeID == 0 || nodeKind == "" {
		return false, fmt.Errorf("agent report batch: node is required")
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AgentReportBatch{
		NodeKind: nodeKind, NodeID: nodeID, BatchID: batchID, Kind: kind, CreatedAt: now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 0, nil
}

// Prune deletes batch records older than Retention.
func Prune(db *gorm.DB, now time.Time) (int64, error) {
	result := db.Where("created_at < ?", now.Add(-Retention)).Delete(&model.AgentReportBatch{})
	return result.RowsAffected, result.Error
}
