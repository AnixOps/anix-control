package packagestoresdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/stretchr/testify/require"
)

func sqliteLeaser(path string) LeaserFunc {
	return func(context.Context) (packagebridgesdk.StorageLease, error) {
		return packagebridgesdk.StorageLease{Driver: "sqlite", DSN: path, TablePrefix: "pkg_knowledge_", LeaseGeneration: 1}, nil
	}
}

func migrationFS(steps map[string]string, index string) fstest.MapFS {
	fsys := fstest.MapFS{"migrations/index.json": {Data: []byte(index)}}
	for name, script := range steps {
		fsys[name] = &fstest.MapFile{Data: []byte(script)}
	}
	return fsys
}

const twoStepIndex = `{"format":"anixops.migrations/v1","package_id":"knowledge","version":"__ANIXOPS_PACKAGE_VERSION__","migrations":[
	{"id":"001_notes","path":"migrations/001_notes.sql"},
	{"id":"002_seed","path":"migrations/002_seed.sql"}]}`

func TestRunEmbeddedMigrationsOnSQLiteAppliesEachStepOnce(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, sqliteLeaser(filepath.Join(t.TempDir(), "kernel.db")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.Equal(t, "pkg_knowledge_notes", store.Table("notes"))

	fsys := migrationFS(map[string]string{
		"migrations/001_notes.sql": "CREATE TABLE __PKG_PREFIX__notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL);\nCREATE INDEX __PKG_PREFIX__notes_body ON __PKG_PREFIX__notes (body);",
		"migrations/002_seed.sql":  "INSERT INTO __PKG_PREFIX__notes (body) VALUES ('first');",
	}, twoStepIndex)
	result, err := RunEmbeddedMigrations(ctx, store, fsys, "migrations/index.json")
	require.NoError(t, err)
	require.Equal(t, []string{"001_notes", "002_seed"}, result.Applied)
	require.Len(t, result.StepsDigest, 64)

	again, err := RunEmbeddedMigrations(ctx, store, fsys, "migrations/index.json")
	require.NoError(t, err)
	require.Empty(t, again.Applied)
	require.Equal(t, result.StepsDigest, again.StepsDigest)
	var count int64
	require.NoError(t, store.DB.Table(store.Table("notes")).Count(&count).Error)
	require.EqualValues(t, 1, count)

	fsys["migrations/002_seed.sql"] = &fstest.MapFile{Data: []byte("INSERT INTO __PKG_PREFIX__notes (body) VALUES ('changed');")}
	_, err = RunEmbeddedMigrations(ctx, store, fsys, "migrations/index.json")
	require.ErrorIs(t, err, ErrMigrationChanged)
}

func TestRunEmbeddedMigrationsRollsBackAFailedStep(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, sqliteLeaser(filepath.Join(t.TempDir(), "kernel.db")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	fsys := migrationFS(map[string]string{
		"migrations/001_notes.sql": "CREATE TABLE __PKG_PREFIX__notes (id INTEGER PRIMARY KEY);",
		"migrations/002_seed.sql":  "INSERT INTO __PKG_PREFIX__notes (id) VALUES (1); INSERT INTO __PKG_PREFIX__missing VALUES (1);",
	}, twoStepIndex)
	result, err := RunEmbeddedMigrations(ctx, store, fsys, "migrations/index.json")
	require.ErrorContains(t, err, "migration 002_seed")
	require.Equal(t, []string{"001_notes"}, result.Applied)
	var count int64
	require.NoError(t, store.DB.Table(store.Table("notes")).Count(&count).Error)
	require.Zero(t, count, "the failed step left nothing behind")
	require.NoError(t, store.DB.Table(store.Table(StateTable)).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMigrationIndexValidation(t *testing.T) {
	script := "CREATE TABLE __PKG_PREFIX__x (id INTEGER);"
	cases := map[string]string{
		"format":         `{"format":"other","migrations":[]}`,
		"unknown field":  `{"format":"anixops.migrations/v1","migrations":[],"extra":1}`,
		"bad id":         `{"format":"anixops.migrations/v1","migrations":[{"id":"../x","path":"migrations/a.sql"}]}`,
		"repeated id":    `{"format":"anixops.migrations/v1","migrations":[{"id":"a","path":"migrations/a.sql"},{"id":"a","path":"migrations/a.sql"}]}`,
		"escaping path":  `{"format":"anixops.migrations/v1","migrations":[{"id":"a","path":"migrations/../a.sql"}]}`,
		"outside":        `{"format":"anixops.migrations/v1","migrations":[{"id":"a","path":"bin/a.sql"}]}`,
		"digest":         `{"format":"anixops.migrations/v1","migrations":[{"id":"a","path":"migrations/a.sql","sha256":"00"}]}`,
		"missing script": `{"format":"anixops.migrations/v1","migrations":[{"id":"a","path":"migrations/b.sql"}]}`,
	}
	for name, index := range cases {
		_, err := loadMigrationSteps(migrationFS(map[string]string{"migrations/a.sql": script}, index), "migrations/index.json")
		require.Error(t, err, name)
	}
}

func TestOpenRejectsBadLeases(t *testing.T) {
	ctx := context.Background()
	for name, lease := range map[string]packagebridgesdk.StorageLease{
		"driver": {Driver: "mysql"},
		"schema": {Driver: "postgres", Schema: "pkg; drop"},
		"prefix": {Driver: "sqlite", DSN: "x.db", TablePrefix: "pkg"},
	} {
		_, err := Open(ctx, LeaserFunc(func(context.Context) (packagebridgesdk.StorageLease, error) { return lease, nil }))
		require.Error(t, err, name)
	}
	refused := errors.New("refused")
	_, err := Open(ctx, LeaserFunc(func(context.Context) (packagebridgesdk.StorageLease, error) {
		return packagebridgesdk.StorageLease{}, refused
	}))
	require.ErrorIs(t, err, refused)
	require.Panics(t, func() { (&Store{}).Table("Bad-Name") })
}

func TestStepsDigestIsOrderIndependentAndLowercase(t *testing.T) {
	want := sha256.Sum256([]byte("001_a:aa\n002_b:bb\n"))
	require.Equal(t, hex.EncodeToString(want[:]), StepsDigest([]MigrationStep{{ID: "002_b", SHA256: "BB"}, {ID: "001_a", SHA256: "aa"}}))
}
