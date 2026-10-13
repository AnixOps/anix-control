package backup

import (
	"context"
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// snapshotFacts are what a manifest records about a snapshot.
type snapshotFacts struct {
	// SchemaVersion is the newest migration recorded in the snapshot's
	// v5_schema_migration; 0 without one (a database from before the
	// migration runner).
	SchemaVersion int
	RowCounts     map[string]int64
}

// openSnapshot opens a snapshot file for reading only (query_only).
func openSnapshot(path string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=query_only(1)"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	return db, func() { _ = sqlDB.Close() }, nil
}

func readSnapshotFacts(ctx context.Context, path string) (snapshotFacts, error) {
	db, closeDB, err := openSnapshot(path)
	if err != nil {
		return snapshotFacts{}, err
	}
	defer closeDB()
	return factsOf(ctx, db)
}

func factsOf(ctx context.Context, db *gorm.DB) (snapshotFacts, error) {
	db = db.WithContext(ctx)
	facts := snapshotFacts{RowCounts: map[string]int64{}}
	if db.Migrator().HasTable(migrations.MigrationTable) {
		var newest *int
		if err := db.Raw("SELECT max(version) FROM " + migrations.MigrationTable).Scan(&newest).Error; err != nil {
			return facts, fmt.Errorf("read %s: %w", migrations.MigrationTable, err)
		}
		if newest != nil {
			facts.SchemaVersion = *newest
		}
	}
	for _, table := range RowCountTables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return facts, fmt.Errorf("count %s: %w", table, err)
		}
		facts.RowCounts[table] = count
	}
	return facts, nil
}
