package keystore

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const table = "pkg_identity_signing_key"

// Schema is the product's migration of the key table.
const Schema = `CREATE TABLE ` + table + ` (id VARCHAR(64) PRIMARY KEY, state VARCHAR(16) NOT NULL,
	public_key TEXT NOT NULL, sealed_private_key TEXT NOT NULL,
	created_at BIGINT NOT NULL, activated_at BIGINT NOT NULL, retired_at BIGINT NOT NULL)`

var policy = signingkey.Policy{RotateAfter: 30 * 24 * time.Hour, PublishAhead: 15 * time.Minute, VerifyAfterRetire: 25 * time.Hour}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newStore(t *testing.T, kek string) (*Store, *clock, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec(Schema).Error)
	raw, err := signingkey.ParseKEK(kek)
	require.NoError(t, err)
	sealer, err := signingkey.NewSealer(raw)
	require.NoError(t, err)
	c := &clock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	return &Store{DB: db, Table: table, Sealer: sealer, Policy: policy, Now: c.Now}, c, db
}

const kek = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestAdvancePersistsSealedKeysAndRotates(t *testing.T) {
	store, clock, db := newStore(t, kek)
	ctx := context.Background()
	keys, err := store.Advance(ctx)
	require.NoError(t, err)
	first, err := signingkey.Signing(keys)
	require.NoError(t, err)

	var sealed string
	require.NoError(t, db.Table(table).Select("sealed_private_key").Where("id = ?", first.ID).Scan(&sealed).Error)
	require.NotEmpty(t, sealed)
	require.NotContains(t, sealed, string(first.PrivateKey.Seed()), "private keys are stored sealed")

	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.True(t, first.PrivateKey.Equal(loaded[0].PrivateKey), "another replica opens the same key")

	clock.now = clock.now.Add(policy.RotateAfter - policy.PublishAhead)
	keys, err = store.Advance(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	clock.now = clock.now.Add(policy.PublishAhead)
	keys, err = store.Advance(ctx)
	require.NoError(t, err)
	second, err := signingkey.Signing(keys)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)

	loaded, err = store.Load(ctx)
	require.NoError(t, err)
	states := map[string]signingkey.State{}
	for _, key := range loaded {
		states[key.ID] = key.State
	}
	require.Equal(t, map[string]signingkey.State{first.ID: signingkey.StateRetired, second.ID: signingkey.StateActive}, states)
}

func TestRevokedKeysLoseTheirPrivateHalf(t *testing.T) {
	store, _, db := newStore(t, kek)
	ctx := context.Background()
	keys, err := store.Advance(ctx)
	require.NoError(t, err)
	require.NoError(t, store.Revoke(ctx, keys[0].ID))
	var sealed string
	require.NoError(t, db.Table(table).Select("sealed_private_key").Where("id = ?", keys[0].ID).Scan(&sealed).Error)
	require.Empty(t, sealed)
	keys, err = store.Advance(ctx)
	require.NoError(t, err, "a replacement key is activated")
	_, err = signingkey.Signing(keys)
	require.NoError(t, err)
	require.Error(t, store.Revoke(ctx, "idk-missing"))
}

func TestAnotherKEKCannotOpenTheKeys(t *testing.T) {
	store, _, db := newStore(t, kek)
	_, err := store.Advance(context.Background())
	require.NoError(t, err)
	raw, err := signingkey.ParseKEK("ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100")
	require.NoError(t, err)
	other, err := signingkey.NewSealer(raw)
	require.NoError(t, err)
	wrong := &Store{DB: db, Table: table, Sealer: other, Policy: policy}
	_, err = wrong.Load(context.Background())
	require.Error(t, err)
}
