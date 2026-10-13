package service

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// forwardNodeColumns are the v2_forward_node columns added after the table
// was first created, by struct field: metrics_port, and the node group
// columns of 5.0 (internal/nodegroup). The approval column's default
// ('approved') applies to the rows that exist when it is added.
var forwardNodeColumns = []string{"MetricsPort", "NodeGroupID", "MachineFingerprint", "Approval", "LastPublicAddress"}

// forwardNodeIndexes are the indexes of those columns, by struct field.
var forwardNodeIndexes = []string{"NodeGroupID", "MachineFingerprint"}

// EnsureForwardNodeColumns adds the columns of forwardNodeColumns that the
// existing v2_forward_node table lacks, and their indexes. It runs at every
// start (including production) because AutoMigrate only runs in
// development and test; it touches only these columns and indexes, never
// anything else, and does nothing when they exist.
func EnsureForwardNodeColumns(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	migrator := db.Migrator()
	for _, field := range forwardNodeColumns {
		if migrator.HasColumn(&model.ForwardNode{}, field) {
			continue
		}
		if err := migrator.AddColumn(&model.ForwardNode{}, field); err != nil {
			return err
		}
	}
	for _, field := range forwardNodeIndexes {
		if migrator.HasIndex(&model.ForwardNode{}, field) {
			continue
		}
		if err := migrator.CreateIndex(&model.ForwardNode{}, field); err != nil {
			return err
		}
	}
	return nil
}
