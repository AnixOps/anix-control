package database

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Concurrent transactions that read before they write wait for each other
// instead of failing with "database is locked": a deferred transaction that
// upgrades its read to a write on a stale WAL snapshot fails at once,
// whatever busy_timeout says, so every transaction begins IMMEDIATE.
func TestSQLiteConcurrentReadWriteTransactionsDoNotFailLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.db")
	db, err := gorm.Open(sqlite.Open(sqliteConnectionString(path)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	type counter struct {
		ID    uint
		Value int
	}
	require.NoError(t, db.AutoMigrate(&counter{}))
	require.NoError(t, db.Create(&counter{ID: 1}).Error)

	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				errs <- db.Transaction(func(tx *gorm.DB) error {
					var c counter
					if err := tx.First(&c, 1).Error; err != nil {
						return err
					}
					return tx.Model(&counter{}).Where("id = 1").Update("value", c.Value+1).Error
				})
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var c counter
	require.NoError(t, db.First(&c, 1).Error)
	require.Equal(t, workers*rounds, c.Value, "no update was lost")
}
