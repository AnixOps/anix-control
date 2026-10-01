package agentreports_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresDSN enables the PostgreSQL runs; each run gets a throwaway schema.
const postgresDSN = "ANIX_TEST_POSTGRES_DSN"

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// forEachDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN is
// set, on PostgreSQL, each with the batch table.
func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		require.NoError(t, db.AutoMigrate(&model.AgentReportBatch{}))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openPostgres(t)
		require.NoError(t, db.AutoMigrate(&model.AgentReportBatch{}))
		body(t, db)
	})
}

// openPostgres creates a throwaway schema in the test database and returns
// a connection whose search_path is that schema.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(postgresDSN))
	if base == "" {
		t.Skip(postgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "agent_reports_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func claim(t *testing.T, db *gorm.DB, kind string, id uint, batch string) bool {
	t.Helper()
	var seen bool
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		seen, err = agentreports.ClaimTx(tx, kind, id, batch, agentreports.KindTraffic, now)
		return err
	}))
	return seen
}

// A batch id is applied once per node: the first claim is new, a replay is
// seen, and the same id for another node, or the other node kind of the
// same id, is new again.
func TestClaimOncePerNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		const batch = "node:proxy-7:boot-a:1"
		assert.False(t, claim(t, db, "proxy", 7, batch), "first delivery")
		assert.True(t, claim(t, db, "proxy", 7, batch), "replay")
		assert.True(t, claim(t, db, "proxy", 7, batch), "second replay")
		assert.False(t, claim(t, db, "proxy", 8, batch), "another node")
		assert.False(t, claim(t, db, "forward", 7, batch), "a forward node of the same id")
		assert.False(t, claim(t, db, "proxy", 7, "node:proxy-7:boot-a:2"), "the next sequence")

		var rows []model.AgentReportBatch
		require.NoError(t, db.Order("node_kind, node_id, batch_id").Find(&rows).Error)
		require.Len(t, rows, 4)
		assert.Equal(t, agentreports.KindTraffic, rows[0].Kind)
	})
}

// A claim rolls back with the transaction that applies the batch, so a
// failed application is retried, not remembered.
func TestClaimRollsBackWithItsTransaction(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		const batch = "node:proxy-7:boot-a:1"
		err := db.Transaction(func(tx *gorm.DB) error {
			seen, err := agentreports.ClaimTx(tx, "proxy", 7, batch, agentreports.KindLogs, now)
			require.NoError(t, err)
			require.False(t, seen)
			return assert.AnError
		})
		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, claim(t, db, "proxy", 7, batch), "retried after the rollback")
		assert.True(t, claim(t, db, "proxy", 7, batch))
	})
}

// Two concurrent deliveries of one batch: exactly one is new.
func TestClaimConcurrentDeliveries(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		const batch = "node:proxy-7:boot-a:1"
		const deliveries = 8
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			applied int
			errs    []error
		)
		for i := 0; i < deliveries; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := db.Transaction(func(tx *gorm.DB) error {
					seen, err := agentreports.ClaimTx(tx, "proxy", 7, batch, agentreports.KindTraffic, now)
					if err != nil {
						return err
					}
					if !seen {
						mu.Lock()
						applied++
						mu.Unlock()
					}
					return nil
				})
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		// A delivery may fail on a lock (transient, so the agent resends);
		// none may count the batch a second time.
		assert.Equal(t, 1, applied, "errors: %v", errs)
	})
}

func TestClaimValidatesBatchIDAndNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		for name, batch := range map[string]string{"empty": "", "too long": strings.Repeat("x", 129)} {
			t.Run(name, func(t *testing.T) {
				_, err := agentreports.ClaimTx(db, "proxy", 7, batch, agentreports.KindTraffic, now)
				assert.ErrorIs(t, err, agentreports.ErrInvalidBatchID)
			})
		}
		assert.NoError(t, agentreports.ValidateBatchID(strings.Repeat("x", 128)))
		_, err := agentreports.ClaimTx(db, "proxy", 0, "b", agentreports.KindTraffic, now)
		assert.Error(t, err)
		_, err = agentreports.ClaimTx(db, "", 7, "b", agentreports.KindTraffic, now)
		assert.Error(t, err)
		var count int64
		require.NoError(t, db.Model(&model.AgentReportBatch{}).Count(&count).Error)
		assert.Zero(t, count)
	})
}

// Prune drops records older than the retention and keeps the rest; a batch
// id pruned away would apply again, which the retention makes impossible
// for a spool that resends on reconnect.
func TestPrune(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		old := now.Add(-agentreports.Retention - time.Minute)
		recent := now.Add(-agentreports.Retention + time.Minute)
		_, err := agentreports.ClaimTx(db, "proxy", 7, "old", agentreports.KindTraffic, old)
		require.NoError(t, err)
		_, err = agentreports.ClaimTx(db, "proxy", 7, "recent", agentreports.KindTraffic, recent)
		require.NoError(t, err)
		pruned, err := agentreports.Prune(db, now)
		require.NoError(t, err)
		assert.Equal(t, int64(1), pruned)
		assert.True(t, claim(t, db, "proxy", 7, "recent"), "still remembered")
		assert.False(t, claim(t, db, "proxy", 7, "old"), "forgotten")
	})
}
