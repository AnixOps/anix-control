package service

import (
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The node operation revision cursor (v3_kernel_node_operation_revision) is
// the one allocator of Agent operation revisions for a proxy node. Durable
// plugin operations take theirs in CreateKernelOperation; the one-off stream
// operations (agent.diagnostic, the forward checks, node.reload, agent.ping,
// users.reload) take theirs from AllocateNodeOperationRevision, so revisions
// on a node are strictly increasing across both kinds, as the Agent requires.

// AllocateNodeOperationRevision reserves the next desired revision of
// nodeID: one above both the stored cursor and floor (the highest revision
// the caller already knows was sent), and stores it.
func AllocateNodeOperationRevision(db *gorm.DB, nodeID uint, floor int64) (int64, error) {
	if db == nil {
		return 0, errors.New("node operation revision allocation requires a database")
	}
	if floor < 0 {
		return 0, errors.New("node operation revision floor is negative")
	}
	var revision int64
	err := db.Transaction(func(tx *gorm.DB) error {
		cursor, err := lockNodeOperationRevisionTx(tx, nodeID)
		if err != nil {
			return err
		}
		revision = max(cursor.DesiredRevision, floor) + 1
		return tx.Model(cursor).Update("desired_revision", revision).Error
	})
	if err != nil {
		return 0, err
	}
	return revision, nil
}

// RaiseNodeOperationRevision raises the stored desired revision of nodeID
// to at least revision, so the next allocation is above it. An Agent that
// connects reports the revision it last applied; after a Control restart it
// can be above the cursor.
func RaiseNodeOperationRevision(db *gorm.DB, nodeID uint, revision int64) error {
	if db == nil {
		return errors.New("node operation revision raise requires a database")
	}
	if revision <= 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		cursor, err := lockNodeOperationRevisionTx(tx, nodeID)
		if err != nil {
			return err
		}
		if cursor.DesiredRevision >= revision {
			return nil
		}
		return tx.Model(cursor).Update("desired_revision", revision).Error
	})
}

// lockNodeOperationRevisionTx seeds the cursor row of nodeID and returns it
// locked for update, as CreateKernelOperation does.
func lockNodeOperationRevisionTx(tx *gorm.DB, nodeID uint) (*model.NodeOperationRevision, error) {
	seed := model.NodeOperationRevision{NodeID: nodeID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return nil, err
	}
	cursor := &model.NodeOperationRevision{}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(cursor, "node_id = ?", nodeID).Error; err != nil {
		return nil, err
	}
	return cursor, nil
}

// RenumberPendingNodeOperations moves the durable operation operationID, which
// the Agent control stream refused because its revision is not above the last
// revision sent on the node, and the pending operations behind it onto
// revisions above both the stored cursor and floor, keeping their order, and
// returns the operation's new revision. A durable operation takes its revision
// when it is created; a one-off stream operation sent after that takes a
// higher one, and the Agent supersedes anything at or below the revision it
// observed, so the older number can never be sent again. The operation goes
// back to pending so the dispatcher sends it with its new revision. Only an
// operation with no earlier active operation on the node qualifies, which
// means every pending operation behind it has never been sent.
func RenumberPendingNodeOperations(db *gorm.DB, nodeID uint, operationID string, floor int64) (int64, error) {
	if db == nil {
		return 0, errors.New("node operation renumbering requires a database")
	}
	if floor < 0 {
		return 0, errors.New("node operation revision floor is negative")
	}
	var revision int64
	err := db.Transaction(func(tx *gorm.DB) error {
		cursor, err := lockNodeOperationRevisionTx(tx, nodeID)
		if err != nil {
			return err
		}
		var stuck model.KernelOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stuck, "id = ? AND node_id = ?", operationID, nodeID).Error; err != nil {
			return err
		}
		if stuck.State != "pending" && stuck.State != "dispatching" {
			return fmt.Errorf("operation %s is %s and cannot be renumbered", operationID, stuck.State)
		}
		var earlierActive int64
		if err := tx.Model(&model.KernelOperation{}).
			Where("node_id = ? AND revision < ? AND state NOT IN ?", nodeID, stuck.Revision,
				[]string{"succeeded", "completed", "failed", "superseded", "cancelled", "timed_out"}).
			Count(&earlierActive).Error; err != nil {
			return err
		}
		if earlierActive > 0 {
			return fmt.Errorf("operation %s has an earlier active operation and cannot be renumbered", operationID)
		}
		var behind []model.KernelOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_id = ? AND state = ? AND revision > ?", nodeID, "pending", stuck.Revision).
			Order("revision, created_at, id").Find(&behind).Error; err != nil {
			return err
		}
		next := max(cursor.DesiredRevision, floor)
		next++
		revision = next
		if err := tx.Model(&stuck).Updates(map[string]any{
			"revision": next, "state": "pending", "session_id": "", "dispatched_at": nil,
		}).Error; err != nil {
			return err
		}
		for _, operation := range behind {
			next++
			if err := tx.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("revision", next).Error; err != nil {
				return err
			}
		}
		return tx.Model(cursor).Update("desired_revision", next).Error
	})
	if err != nil {
		return 0, err
	}
	return revision, nil
}
