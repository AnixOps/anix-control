package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newBackupServiceForTest(t *testing.T) (*BackupService, string, string) {
	t.Helper()

	root := t.TempDir()
	mainDBPath := filepath.Join(root, "data", "v2board.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(mainDBPath), 0755))

	originCfg := config.Get()
	oldWD, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
		_ = database.Close()
		database.Reset()
		config.Set(originCfg)
	})

	require.NoError(t, os.Chdir(root))
	config.Set(&config.Config{
		Database: config.DatabaseConfig{
			Driver:   "sqlite",
			Database: mainDBPath,
		},
	})

	database.Reset()
	require.NoError(t, database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: mainDBPath,
	}))
	require.NoError(t, database.AutoMigrate(&model.BackupConfig{}, &model.BackupRecord{}))

	return NewBackupService(database.Get()), root, mainDBPath
}

func openSQLiteDB(t *testing.T, path string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	return db
}

func TestBackupServiceDatabaseBackupUsesConfiguredSQLitePath(t *testing.T) {
	svc, root, _ := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "backups")

	require.NoError(t, svc.db.Exec("CREATE TABLE IF NOT EXISTS backup_probe (id INTEGER PRIMARY KEY, value TEXT)").Error)
	require.NoError(t, svc.db.Exec("INSERT INTO backup_probe (id, value) VALUES (?, ?)", 1, "before-backup").Error)
	require.NoError(t, svc.UpdateConfig(&model.BackupConfig{
		StorageType: "local",
		StoragePath: backupDir,
	}))

	record, err := svc.CreateBackup("database", nil)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, 1, record.Status)
	assert.FileExists(t, record.Path)

	backupDB := openSQLiteDB(t, record.Path)
	var restoredValue string
	require.NoError(t, backupDB.Raw("SELECT value FROM backup_probe WHERE id = 1").Scan(&restoredValue).Error)
	assert.Equal(t, "before-backup", restoredValue)
}

func TestBackupServiceFilesBackupCreatesArchive(t *testing.T) {
	svc, root, _ := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "backups")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "config"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "public"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"), []byte("a: 1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "data", "cache.txt"), []byte("cache"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "public", "index.html"), []byte("<html/>"), 0644))

	require.NoError(t, svc.UpdateConfig(&model.BackupConfig{
		StorageType: "local",
		StoragePath: backupDir,
	}))

	record, err := svc.CreateBackup("files", nil)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, 1, record.Status)
	assert.FileExists(t, record.Path)

	reader, err := zip.OpenReader(record.Path)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, reader.Close())
	}()

	names := make(map[string]struct{}, len(reader.File))
	for _, file := range reader.File {
		names[file.Name] = struct{}{}
	}

	_, hasManifest := names["meta/manifest.json"]
	_, hasConfig := names["config/app.yaml"]
	_, hasData := names["data/cache.txt"]
	_, hasPublic := names["public/index.html"]
	_, hasMainDB := names["data/v2board.db"]

	assert.True(t, hasManifest)
	assert.True(t, hasConfig)
	assert.True(t, hasData)
	assert.True(t, hasPublic)
	assert.False(t, hasMainDB)
}

func TestBackupServiceFilesBackupCanRestoreFiles(t *testing.T) {
	svc, root, _ := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "backups")
	targetFile := filepath.Join(root, "config", "runtime.yaml")

	require.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0755))
	require.NoError(t, os.WriteFile(targetFile, []byte("version: one"), 0644))
	require.NoError(t, svc.UpdateConfig(&model.BackupConfig{
		StorageType: "local",
		StoragePath: backupDir,
	}))

	record, err := svc.CreateBackup("files", nil)
	require.NoError(t, err)
	require.NotNil(t, record)

	require.NoError(t, os.WriteFile(targetFile, []byte("version: two"), 0644))
	require.NoError(t, svc.RestoreBackup(record.ID))

	restored, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "version: one", string(restored))
}

func TestBackupServiceFullBackupCanRestoreDatabase(t *testing.T) {
	svc, root, mainDBPath := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "backups")
	runtimeFile := filepath.Join(root, "config", "runtime.yaml")

	require.NoError(t, os.MkdirAll(filepath.Dir(runtimeFile), 0755))
	require.NoError(t, os.WriteFile(runtimeFile, []byte("mode: original"), 0644))
	require.NoError(t, svc.db.Exec("CREATE TABLE IF NOT EXISTS full_restore_probe (id INTEGER PRIMARY KEY, value TEXT)").Error)
	require.NoError(t, svc.db.Exec("INSERT INTO full_restore_probe (id, value) VALUES (?, ?)", 1, "before-restore").Error)
	require.NoError(t, svc.UpdateConfig(&model.BackupConfig{
		StorageType: "local",
		StoragePath: backupDir,
	}))

	record, err := svc.CreateBackup("full", nil)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, 1, record.Status)

	require.NoError(t, svc.db.Exec("UPDATE full_restore_probe SET value = ? WHERE id = 1", "corrupted").Error)
	require.NoError(t, os.WriteFile(runtimeFile, []byte("mode: changed"), 0644))
	require.NoError(t, svc.RestoreBackup(record.ID))

	restoredDB := openSQLiteDB(t, mainDBPath)
	var restoredValue string
	require.NoError(t, restoredDB.Raw("SELECT value FROM full_restore_probe WHERE id = 1").Scan(&restoredValue).Error)
	assert.Equal(t, "before-restore", restoredValue)

	restoredRuntimeFile, err := os.ReadFile(runtimeFile)
	require.NoError(t, err)
	assert.Equal(t, "mode: original", string(restoredRuntimeFile))
}

func TestBackupServiceCreateBackupUsesPrivateDirectoryPermissions(t *testing.T) {
	svc, root, _ := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "private-backups")

	require.NoError(t, svc.UpdateConfig(&model.BackupConfig{
		StorageType: "local",
		StoragePath: backupDir,
	}))

	record, err := svc.CreateBackup("database", nil)
	require.NoError(t, err)
	require.NotNil(t, record)

	info, err := os.Stat(backupDir)
	require.NoError(t, err)
	assert.Equal(t, backupDirectoryMode, info.Mode().Perm())
}

func TestBackupServiceRestoreArchiveRejectsTraversalEntry(t *testing.T) {
	svc, root, _ := newBackupServiceForTest(t)
	backupDir := filepath.Join(root, "backups")
	require.NoError(t, os.MkdirAll(backupDir, backupDirectoryMode))

	escapePath := filepath.Clean(filepath.Join(root, "..", "escape.txt"))
	if err := os.Remove(escapePath); err != nil && !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	t.Cleanup(func() { _ = os.Remove(escapePath) })

	zipPath := filepath.Join(backupDir, "traversal.zip")
	file, err := os.Create(zipPath)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape.txt")
	require.NoError(t, err)
	_, err = entry.Write([]byte("escaped"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	record := &model.BackupRecord{
		Name:   "traversal",
		Type:   "files",
		Status: 1,
		Path:   zipPath,
	}
	require.NoError(t, svc.db.Create(record).Error)

	err = svc.RestoreBackup(record.ID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid archive entry path")
	assert.NoFileExists(t, escapePath)
}

func TestValidateArchiveFileForRestoreRejectsUnsafeEntries(t *testing.T) {
	oversized := &zip.File{FileHeader: zip.FileHeader{
		Name:               "huge.bin",
		UncompressedSize64: maxBackupArchiveFileBytes + 1,
	}}
	assert.ErrorContains(t, validateArchiveFileForRestore(oversized, nil), "exceeds maximum restore size")

	total := maxBackupArchiveTotalBytes - 5
	tooMuchTotal := &zip.File{FileHeader: zip.FileHeader{
		Name:               "total.bin",
		UncompressedSize64: 6,
	}}
	assert.ErrorContains(t, validateArchiveFileForRestore(tooMuchTotal, &total), "archive exceeds maximum restore size")

	symlinkHeader := zip.FileHeader{
		Name:               "link",
		UncompressedSize64: 1,
	}
	symlinkHeader.SetMode(os.ModeSymlink | 0o777)
	symlink := &zip.File{FileHeader: symlinkHeader}
	assert.ErrorContains(t, validateArchiveFileForRestore(symlink, nil), "unsupported archive entry type")
}
