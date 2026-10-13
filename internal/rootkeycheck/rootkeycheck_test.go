package rootkeycheck_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/rootkeycheck"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "control.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(tables...))
	return db
}

func TestCheckRefusesAKeyThatDoesNotOpenTheCA(t *testing.T) {
	ctx := context.Background()
	right, wrong := bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	db := openDB(t, &model.ServiceCA{}, &model.ForwardLinkCA{}, &model.ACMECacheEntry{}, &model.NodeGroup{})
	require.NoError(t, rootkeycheck.Check(ctx, db, wrong, nil), "nothing sealed yet")

	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "default", KEK: right})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(ctx))
	require.NoError(t, rootkeycheck.Check(ctx, db, right, nil))

	err = rootkeycheck.Check(ctx, db, wrong, nil)
	var mismatch *rootkeycheck.Mismatch
	require.True(t, errors.As(err, &mismatch), "%v", err)
	assert.Equal(t, "module CA key", mismatch.What)
	assert.NotContains(t, err.Error(), string(wrong))
}

// A snapshot taken before a migration may lack later tables: their checks
// are skipped, the others still run.
func TestCheckSkipsTheTablesADatabaseDoesNotHave(t *testing.T) {
	ctx := context.Background()
	right, wrong := bytes.Repeat([]byte{4}, 32), bytes.Repeat([]byte{5}, 32)
	db := openDB(t, &model.ACMECacheEntry{})
	store, err := panelcert.NewStore(db, right, nil)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, "acme-000000000000/panel.example.com", []byte("a certificate and its key")))

	require.NoError(t, rootkeycheck.Check(ctx, db, right, nil))
	err = rootkeycheck.Check(ctx, db, wrong, nil)
	var mismatch *rootkeycheck.Mismatch
	require.True(t, errors.As(err, &mismatch), "%v", err)
	assert.Equal(t, "ACME account or certificate key", mismatch.What)
}
