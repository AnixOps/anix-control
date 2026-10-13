package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

type splitCounter struct {
	ID    uint `gorm:"primaryKey;autoIncrement:false"`
	Value int
}

type splitEvent struct {
	ID     uint `gorm:"primaryKey"`
	Worker int
	Seq    int
}

func initFileDatabase(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	Reset()
	path := filepath.Join(t.TempDir(), "control.db")
	require.NoError(t, Init(&config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"}))
	t.Cleanup(func() { require.NoError(t, Close()) })
	return Get(), path
}

// pragmaOf runs a pragma through connPool and returns its single value.
func pragmaOf(t *testing.T, connPool *sql.DB, name string) string {
	t.Helper()
	var value string
	require.NoError(t, connPool.QueryRow("PRAGMA "+name).Scan(&value))
	return value
}

func TestInitSQLiteFileDatabaseUsesAWriterAndAReadPool(t *testing.T) {
	db, _ := initFileDatabase(t)

	writer, err := db.DB()
	require.NoError(t, err)
	require.Equal(t, 1, writer.Stats().MaxOpenConnections, "one writer connection")
	require.Len(t, readPools, 1)
	require.Equal(t, SQLiteReadConns, readPools[0].Stats().MaxOpenConnections)

	for name, pool := range map[string]*sql.DB{"writer": writer, "reader": readPools[0]} {
		require.Equal(t, "wal", pragmaOf(t, pool, "journal_mode"), name)
		require.Equal(t, "10000", pragmaOf(t, pool, "busy_timeout"), name)
		require.Equal(t, "0", pragmaOf(t, pool, "foreign_keys"), name+": foreign keys stay as before")
	}
	require.Equal(t, "1", pragmaOf(t, writer, "synchronous"), "NORMAL")
	require.Equal(t, "2", pragmaOf(t, writer, "auto_vacuum"), "INCREMENTAL on a new database")
	require.Equal(t, "0", pragmaOf(t, writer, "query_only"))
	require.Equal(t, "1", pragmaOf(t, readPools[0], "query_only"))
}

func TestInitSQLiteInMemoryDatabaseKeepsOneHandle(t *testing.T) {
	Reset()
	require.NoError(t, Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:", LogLevel: "silent"}))
	t.Cleanup(func() { require.NoError(t, Close()) })
	require.Empty(t, readPools, "each connection of an in-memory database is its own database")
	require.NoError(t, Get().AutoMigrate(&splitCounter{}))
	require.NoError(t, Get().Create(&splitCounter{ID: 1, Value: 7}).Error)
	var counter splitCounter
	require.NoError(t, Get().First(&counter, 1).Error)
	require.Equal(t, 7, counter.Value)
}

// Queries run on the reader, statements and transactions on the writer.
func TestSQLiteRoutesReadsToTheReaderAndWritesToTheWriter(t *testing.T) {
	db, _ := initFileDatabase(t)
	queryOnly := func(handle *gorm.DB) int {
		var value int
		require.NoError(t, handle.Raw("SELECT query_only FROM pragma_query_only").Scan(&value).Error)
		return value
	}
	require.Equal(t, 1, queryOnly(db), "a plain SELECT runs on a read connection")
	require.Equal(t, 1, queryOnly(db.Clauses(dbresolver.Read)))
	require.Equal(t, 0, queryOnly(db.Clauses(dbresolver.Write)), "a read can be forced onto the writer")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.Equal(t, 0, queryOnly(tx), "everything in a transaction runs on the writer")
		return nil
	}))

	require.NoError(t, db.AutoMigrate(&splitCounter{}))
	require.NoError(t, db.Create(&splitCounter{ID: 1, Value: 1}).Error)
	require.NoError(t, db.Model(&splitCounter{}).Where("id = 1").Update("value", gorm.Expr("value + 1")).Error)
	var counter splitCounter
	require.NoError(t, db.First(&counter, 1).Error)
	require.Equal(t, 2, counter.Value, "a read after a committed write sees it")

	err := db.Clauses(dbresolver.Read).Exec("UPDATE split_counters SET value = 0").Error
	require.Error(t, err, "a write that reaches a reader fails instead of queueing")
	require.Contains(t, err.Error(), "readonly")
}

func TestSQLiteTransactionSeesItsOwnWritesAndOthersSeeCommittedOnly(t *testing.T) {
	db, _ := initFileDatabase(t)
	require.NoError(t, db.AutoMigrate(&splitCounter{}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&splitCounter{ID: 5, Value: 50}).Error)
		var inside splitCounter
		require.NoError(t, tx.First(&inside, 5).Error)
		require.Equal(t, 50, inside.Value, "the transaction sees its own write")
		var outside int64
		require.NoError(t, db.Model(&splitCounter{}).Where("id = 5").Count(&outside).Error)
		require.Zero(t, outside, "the root handle reads what was committed, without waiting for the writer")
		return nil
	}))
	var after int64
	require.NoError(t, db.Model(&splitCounter{}).Where("id = 5").Count(&after).Error)
	require.Equal(t, int64(1), after)
}

// Readers keep answering while a write transaction is open and a second
// writer waits: the reason for the split.
func TestSQLiteReadsDoNotWaitForTheWriter(t *testing.T) {
	db, _ := initFileDatabase(t)
	require.NoError(t, db.AutoMigrate(&splitCounter{}))
	require.NoError(t, db.Create(&splitCounter{ID: 1, Value: 1}).Error)

	holding, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&splitCounter{}).Where("id = 1").Update("value", 2).Error; err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	var counter splitCounter
	started := time.Now()
	require.NoError(t, db.First(&counter, 1).Error)
	require.Equal(t, 1, counter.Value, "the reader sees the last commit")
	require.Less(t, time.Since(started), time.Second)
	close(release)
	require.NoError(t, <-done)
}

func TestCloseClosesTheReadPool(t *testing.T) {
	Reset()
	path := filepath.Join(t.TempDir(), "control.db")
	require.NoError(t, Init(&config.DatabaseConfig{Driver: "sqlite", Database: path, LogLevel: "silent"}))
	require.Len(t, readPools, 1)
	reader := readPools[0]
	require.NoError(t, Close())
	require.Error(t, reader.PingContext(context.Background()), "the read pool is closed with the writer")
	require.Empty(t, readPools)
}

// Kernel writers (immediate read-modify-write transactions and plain
// statements) and readers on the handle, plus a second process's connections
// to the same file (a package host: its own pool, up to 4 connections),
// for a few seconds: nothing fails with "database is locked" and no update
// is lost.
func TestSQLiteConcurrentWritersAndReadersDoNotFailLocked(t *testing.T) {
	db, path := initFileDatabase(t)
	require.NoError(t, db.AutoMigrate(&splitCounter{}, &splitEvent{}))
	require.NoError(t, db.Create(&splitCounter{ID: 1}).Error)
	require.NoError(t, db.Create(&splitCounter{ID: 2}).Error)

	// What a package host opens: the same file, its own pool.
	host, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	hostSQL, err := host.DB()
	require.NoError(t, err)
	hostSQL.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = hostSQL.Close() })

	const kernelWriters, hostWriters, readers = 4, 3, 3
	var (
		stop     atomic.Bool
		wg       sync.WaitGroup
		failures = make(chan error, 64)
		kernelTx atomic.Int64
		hostTx   atomic.Int64
		inserts  atomic.Int64
		reads    atomic.Int64
	)
	fail := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	for worker := range kernelWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := 0; !stop.Load(); seq++ {
				// A read-modify-write transaction, as the kernel's services do.
				err := db.Transaction(func(tx *gorm.DB) error {
					var counter splitCounter
					if err := tx.First(&counter, 1).Error; err != nil {
						return err
					}
					return tx.Model(&splitCounter{}).Where("id = 1").Update("value", counter.Value+1).Error
				})
				if err != nil {
					fail(err)
					return
				}
				kernelTx.Add(1)
				// A plain statement and an insert, each its own transaction.
				if err := db.Create(&splitEvent{Worker: worker, Seq: seq}).Error; err != nil {
					fail(err)
					return
				}
				inserts.Add(1)
			}
		}()
	}
	for range hostWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				err := host.Transaction(func(tx *gorm.DB) error {
					var counter splitCounter
					if err := tx.First(&counter, 2).Error; err != nil {
						return err
					}
					return tx.Model(&splitCounter{}).Where("id = 2").Update("value", counter.Value+1).Error
				})
				if err != nil {
					fail(err)
					return
				}
				hostTx.Add(1)
			}
		}()
	}
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				var counters []splitCounter
				if err := db.Order("id").Find(&counters).Error; err != nil {
					fail(err)
					return
				}
				var events int64
				if err := db.Model(&splitEvent{}).Count(&events).Error; err != nil {
					fail(err)
					return
				}
				reads.Add(1)
			}
		}()
	}
	time.Sleep(2 * time.Second)
	stop.Store(true)
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
		require.False(t, errors.Is(err, context.Canceled))
		require.False(t, strings.Contains(err.Error(), "locked"), err.Error())
	}

	var one, two splitCounter
	require.NoError(t, db.First(&one, 1).Error)
	require.NoError(t, db.First(&two, 2).Error)
	require.Equal(t, int(kernelTx.Load()), one.Value, "no kernel update was lost")
	require.Equal(t, int(hostTx.Load()), two.Value, "no host update was lost")
	var events int64
	require.NoError(t, db.Model(&splitEvent{}).Count(&events).Error)
	require.Equal(t, inserts.Load(), events)
	require.Positive(t, kernelTx.Load())
	require.Positive(t, hostTx.Load())
	require.Positive(t, reads.Load())
}
