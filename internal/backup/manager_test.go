package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerCreatesVerifiesAndLists(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	var lines []string
	m.Logf = func(format string, args ...any) { lines = append(lines, format) }

	bundle, row, err := m.Create(ctx, TypeManual)
	require.NoError(t, err)
	require.NotZero(t, row.ID)
	assert.Equal(t, bundle.Name, row.Path)
	assert.Equal(t, model.BackupVerifyPending, row.VerifyStatus)
	assert.Equal(t, bundle.SHA256, row.SHA256)
	assert.Equal(t, bundle.Size, row.Size)
	assert.Equal(t, testVersion, row.ProgramVersion)

	verifiedRow, found, err := m.VerifyNewestUnverified(ctx)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, model.BackupVerifyPassed, verifiedRow.VerifyStatus)
	require.NotNil(t, verifiedRow.VerifiedAt)

	_, found, err = m.VerifyNewestUnverified(ctx)
	require.NoError(t, err)
	assert.False(t, found, "every bundle is verified")

	rows, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.BackupVerifyPassed, rows[0].VerifyStatus)
	assert.Empty(t, rows[0].VerifyError)
	assert.NotEmpty(t, lines)
}

func TestManagerRecordsAFailedVerification(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	bundle, _, err := m.Create(ctx, TypeDaily)
	require.NoError(t, err)
	tamper(t, bundle.Path, func(name string, data []byte) []byte {
		if name == MemberRootKey {
			return nil
		}
		return data
	}, false, nil)

	row, found, err := m.VerifyNewestUnverified(ctx)
	require.True(t, found)
	var failed *VerifyError
	require.True(t, errors.As(err, &failed), "%v", err)
	assert.Equal(t, model.BackupVerifyFailed, row.VerifyStatus)
	assert.Contains(t, row.VerifyError, "member: root.key is missing")
	stored := backupRows(t, p.db)
	require.Len(t, stored, 1)
	assert.Equal(t, model.BackupVerifyFailed, stored[0].VerifyStatus)
	assert.Equal(t, row.VerifyError, stored[0].VerifyError)
}

// The directory is the truth: a bundle written while v5_backup did not
// exist (or by another process) gets its row, the row of a removed bundle
// goes.
func TestManagerKeepsTheTableInStepWithTheDirectory(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	bundle := p.write(t, TypePreMigration, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC))
	require.NoError(t, os.WriteFile(filepath.Join(p.dataDir, "backups", "notes.txt"), []byte("an operator's file"), 0o600))

	rows, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, bundle.Name, rows[0].Path)
	assert.Equal(t, TypePreMigration, rows[0].Type)
	assert.Equal(t, bundle.Manifest.SchemaVersion, rows[0].SchemaVersion)
	assert.Equal(t, testVersion, rows[0].ProgramVersion)
	assert.True(t, rows[0].CreatedAt.Equal(bundle.Manifest.CreatedAt))

	require.NoError(t, os.Remove(bundle.Path))
	rows, err = m.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Empty(t, backupRows(t, p.db))
}

// Without a database (or before v5_backup exists) the directory alone
// answers.
func TestManagerWithoutTheTable(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	m.DB = nil
	bundle, row, err := m.Create(ctx, TypeManual)
	require.NoError(t, err)
	assert.Zero(t, row.ID)
	rows, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, bundle.Name, rows[0].Path)
	verifiedRow, err := m.Verify(ctx, bundle.Name)
	require.NoError(t, err)
	assert.Equal(t, model.BackupVerifyPassed, verifiedRow.VerifyStatus)
}

func TestPruneRemovesWhatTheRetentionDropsUnlessItIsRead(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	m.Now = func() time.Time { return now }
	var names []string
	for i := 0; i < 5; i++ {
		names = append(names, p.write(t, TypeManual, now.Add(-time.Duration(5-i)*time.Hour)).Name)
	}
	// The two oldest manual bundles are outside the retention; the
	// oldest is being read.
	reader, err := openShared(filepath.Join(p.dataDir, "backups", names[0]))
	require.NoError(t, err)

	decisions, err := m.Prune(ctx)
	require.NoError(t, err)
	assert.Len(t, decisions, 5)
	remaining := ls(t, filepath.Join(p.dataDir, "backups"))
	assert.Contains(t, remaining, names[0], "a bundle being read is not removed")
	assert.NotContains(t, remaining, names[1])
	assert.Len(t, remaining, 4)
	assert.Len(t, backupRows(t, p.db), 4)

	require.NoError(t, reader.Close())
	_, err = m.Prune(ctx)
	require.NoError(t, err)
	assert.NotContains(t, ls(t, filepath.Join(p.dataDir, "backups")), names[0])
	assert.Len(t, backupRows(t, p.db), 3)
}

func TestPruneRemovesWhatAnInterruptedBackupLeft(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	backups, scratch := filepath.Join(p.dataDir, "backups"), filepath.Join(p.dataDir, "tmp")
	require.NoError(t, os.MkdirAll(backups, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(scratch, "backup-123"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(scratch, "verify-new"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(backups, ".partial-1.tar"), []byte("half"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "import-report.json"), []byte("{}"), 0o600))
	old := time.Now().Add(-48 * time.Hour)
	for _, path := range []string{filepath.Join(scratch, "backup-123"), filepath.Join(backups, ".partial-1.tar"), filepath.Join(scratch, "import-report.json")} {
		require.NoError(t, os.Chtimes(path, old, old))
	}
	_, err := m.Prune(ctx)
	require.NoError(t, err)
	assert.Empty(t, ls(t, backups))
	assert.ElementsMatch(t, []string{"verify-new", "import-report.json"}, ls(t, scratch), "a recent scratch directory and other files stay")
}

// On PostgreSQL every operation answers ErrNeedsSQLite and nothing is
// written.
func TestPostgreSQLNeedsSQLite(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	m := p.manager()
	m.Driver = "postgres"
	_, _, err := m.Create(ctx, TypeManual)
	require.ErrorIs(t, err, ErrNeedsSQLite)
	_, err = m.List(ctx)
	require.ErrorIs(t, err, ErrNeedsSQLite)
	_, _, err = m.VerifyNewestUnverified(ctx)
	require.ErrorIs(t, err, ErrNeedsSQLite)
	_, err = m.Prune(ctx)
	require.ErrorIs(t, err, ErrNeedsSQLite)
	_, err = m.Find(ctx, 1)
	require.ErrorIs(t, err, ErrNeedsSQLite)
	assert.Empty(t, ls(t, filepath.Join(p.dataDir, "backups")))
	assert.True(t, strings.HasPrefix(ErrNeedsSQLite.Error(), "backups need SQLite"))

	var lines []string
	m.Logf = func(format string, args ...any) { lines = append(lines, format) }
	(&Worker{Manager: m}).Run(ctx)
	require.Len(t, lines, 1, "the worker says why and stops")
}

// One operation at a time: a request while another runs is told so.
func TestManagerAnswersBusy(t *testing.T) {
	p := newPanel(t)
	m := p.manager()
	m.mu.Lock()
	_, _, err := m.Create(context.Background(), TypeManual)
	m.mu.Unlock()
	require.ErrorIs(t, err, ErrBusy)
}

// Two processes may record one bundle (a backup command's create and the
// server's directory sync): one row, whoever comes first.
func TestRecordingABundleTwiceKeepsOneRow(t *testing.T) {
	ctx := context.Background()
	p := newPanel(t)
	bundle := p.write(t, TypeManual, time.Now())
	server, command := p.manager(), p.manager()
	_, err := server.List(ctx)
	require.NoError(t, err)
	row, err := command.record(ctx, bundle)
	require.NoError(t, err)
	require.NotZero(t, row.ID)
	rows := backupRows(t, p.db)
	require.Len(t, rows, 1)
	assert.Equal(t, rows[0].ID, row.ID)
}
