package service

import (
	"errors"

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
