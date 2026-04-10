package service

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

type BackupService struct {
	db     *gorm.DB
	config *model.BackupConfig
}

type sqliteDatabaseEntry struct {
	Name string `gorm:"column:name"`
	File string `gorm:"column:file"`
}

type backupArchiveSource struct {
	SourcePath  string
	ArchivePath string
}

type backupArchiveManifest struct {
	Type          string    `json:"type"`
	CreatedAt     time.Time `json:"created_at"`
	DatabaseEntry string    `json:"database_entry,omitempty"`
}

func NewBackupService(db *gorm.DB) *BackupService {
	return &BackupService{db: db}
}

func (s *BackupService) GetConfig() (*model.BackupConfig, error) {
	if s.config != nil {
		return s.config, nil
	}

	var cfg model.BackupConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = model.BackupConfig{
			Enabled:        false,
			AutoBackup:     false,
			RetentionDays:  7,
			BackupDatabase: true,
			StorageType:    "local",
			StoragePath:    "backups",
		}
		if createErr := s.db.Create(&cfg).Error; createErr != nil {
			return nil, createErr
		}
		err = nil
	}
	if err != nil {
		return nil, err
	}

	s.config = &cfg
	return &cfg, nil
}

func (s *BackupService) UpdateConfig(cfg *model.BackupConfig) error {
	if err := s.db.Save(cfg).Error; err != nil {
		return err
	}
	s.config = cfg
	return nil
}

func (s *BackupService) CreateBackup(backupType string, createdBy *uint) (*model.BackupRecord, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}

	normalizedType := normalizeBackupType(backupType)
	record := &model.BackupRecord{
		Name:      fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405")),
		Type:      normalizedType,
		Status:    0,
		Auto:      false,
		CreatedBy: createdBy,
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	if normalizeBackupStorageType(cfg.StorageType) != "local" {
		backupErr := fmt.Errorf("backup storage type %q is not implemented yet; only local storage is currently supported", cfg.StorageType)
		now := time.Now()
		record.Status = 2
		record.Error = backupErr.Error()
		record.CompletedAt = &now
		_ = s.db.Save(record).Error
		return record, backupErr
	}

	backupDir, err := s.resolveBackupDirectory(cfg.StoragePath)
	if err != nil {
		now := time.Now()
		record.Status = 2
		record.Error = err.Error()
		record.CompletedAt = &now
		_ = s.db.Save(record).Error
		return record, err
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		now := time.Now()
		record.Status = 2
		record.Error = err.Error()
		record.CompletedAt = &now
		_ = s.db.Save(record).Error
		return record, err
	}

	var backupPath string
	var backupSize int64
	var backupErr error
	switch normalizedType {
	case "database":
		backupPath, backupSize, backupErr = s.backupDatabase(backupDir, record.Name)
	case "files":
		backupPath, backupSize, backupErr = s.backupFiles(backupDir, record.Name)
	case "full":
		backupPath, backupSize, backupErr = s.backupFull(backupDir, record.Name)
	default:
		backupErr = errors.New("unknown backup type")
	}

	now := time.Now()
	if backupErr != nil {
		record.Status = 2
		record.Error = backupErr.Error()
	} else {
		record.Status = 1
		record.Path = backupPath
		record.Size = backupSize
	}
	record.CompletedAt = &now
	_ = s.db.Save(record).Error

	return record, backupErr
}

func (s *BackupService) backupDatabase(backupDir, name string) (string, int64, error) {
	backupPath := filepath.Join(backupDir, name+".db")
	if err := s.createSQLiteSnapshot(backupPath); err != nil {
		return "", 0, err
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return "", 0, err
	}
	return backupPath, info.Size(), nil
}

func (s *BackupService) backupFiles(backupDir, name string) (string, int64, error) {
	backupPath := filepath.Join(backupDir, name+".zip")
	appRoot, err := s.resolveAppRoot()
	if err != nil {
		return "", 0, err
	}

	sqlitePath, err := s.resolveOptionalSQLiteDatabasePath()
	if err != nil {
		return "", 0, err
	}

	sources, err := s.collectRuntimeFileSources(appRoot, backupDir, sqlitePath)
	if err != nil {
		return "", 0, err
	}

	manifest := backupArchiveManifest{
		Type:      "files",
		CreatedAt: time.Now().UTC(),
	}
	return s.writeBackupArchive(backupPath, sources, manifest)
}

func (s *BackupService) backupFull(backupDir, name string) (string, int64, error) {
	backupPath := filepath.Join(backupDir, name+".zip")
	appRoot, err := s.resolveAppRoot()
	if err != nil {
		return "", 0, err
	}

	dbPath, err := s.resolveSQLiteDatabasePath()
	if err != nil {
		return "", 0, err
	}

	sqliteSnapshot, cleanup, err := s.createTemporarySQLiteSnapshot(backupDir)
	if err != nil {
		return "", 0, err
	}
	defer cleanup()

	sources, err := s.collectRuntimeFileSources(appRoot, backupDir, dbPath)
	if err != nil {
		return "", 0, err
	}
	sources = append(sources, backupArchiveSource{
		SourcePath:  sqliteSnapshot,
		ArchivePath: filepath.ToSlash(filepath.Join("database", filepath.Base(dbPath))),
	})

	manifest := backupArchiveManifest{
		Type:          "full",
		CreatedAt:     time.Now().UTC(),
		DatabaseEntry: filepath.ToSlash(filepath.Join("database", filepath.Base(dbPath))),
	}
	return s.writeBackupArchive(backupPath, sources, manifest)
}

func (s *BackupService) ListBackups(page, pageSize int) ([]model.BackupRecord, int64, error) {
	var records []model.BackupRecord
	var total int64

	s.db.Model(&model.BackupRecord{}).Count(&total)
	offset := (page - 1) * pageSize
	err := s.db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error

	return records, total, err
}

func (s *BackupService) DeleteBackup(id uint) error {
	var record model.BackupRecord
	if err := s.db.First(&record, id).Error; err != nil {
		return err
	}
	if record.Path != "" {
		_ = os.Remove(record.Path)
	}
	return s.db.Delete(&record).Error
}

func (s *BackupService) RestoreBackup(id uint) error {
	var record model.BackupRecord
	if err := s.db.First(&record, id).Error; err != nil {
		return err
	}
	if record.Status != 1 {
		return errors.New("backup not successful")
	}

	switch normalizeBackupType(record.Type) {
	case "database":
		return s.restoreDatabase(record.Path)
	case "files":
		return s.restoreArchive(record.Path, false)
	case "full":
		return s.restoreArchive(record.Path, true)
	default:
		return errors.New("unsupported backup type")
	}
}

func (s *BackupService) restoreDatabase(backupPath string) error {
	dbPath, err := s.resolveSQLiteDatabasePath()
	if err != nil {
		return err
	}

	if err := database.Close(); err != nil {
		return err
	}
	return copyFileContents(backupPath, dbPath)
}

func (s *BackupService) restoreArchive(backupPath string, includesDatabase bool) error {
	appRoot, err := s.resolveAppRoot()
	if err != nil {
		return err
	}

	var dbPath string
	if includesDatabase {
		dbPath, err = s.resolveSQLiteDatabasePath()
		if err != nil {
			return err
		}
		if err := database.Close(); err != nil {
			return err
		}
	}

	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		name := strings.TrimSpace(file.Name)
		if name == "" || strings.HasPrefix(name, "meta/") {
			continue
		}

		if file.FileInfo().IsDir() {
			if strings.HasPrefix(name, "database/") {
				continue
			}
			targetDir, err := resolveArchiveTargetPath(appRoot, name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return err
			}
			continue
		}

		targetPath := dbPath
		if !strings.HasPrefix(name, "database/") {
			targetPath, err = resolveArchiveTargetPath(appRoot, name)
			if err != nil {
				return err
			}
		} else if !includesDatabase {
			continue
		}

		if err := extractArchiveFile(file, targetPath); err != nil {
			return err
		}
	}

	return nil
}

func (s *BackupService) CleanupOldBackups() error {
	cfg, _ := s.GetConfig()
	if cfg == nil || cfg.RetentionDays <= 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionDays)
	var records []model.BackupRecord
	s.db.Where("created_at < ? AND status = 1", cutoff).Find(&records)

	for _, record := range records {
		if record.Path != "" {
			_ = os.Remove(record.Path)
		}
		s.db.Delete(&record)
	}
	return nil
}

func (s *BackupService) GetBackupStats() (map[string]interface{}, error) {
	var totalBackups int64
	var totalSize int64
	var lastBackup time.Time

	s.db.Model(&model.BackupRecord{}).Where("status = 1").Count(&totalBackups)
	s.db.Model(&model.BackupRecord{}).Where("status = 1").
		Select("COALESCE(SUM(size), 0)").Scan(&totalSize)

	var record model.BackupRecord
	if err := s.db.Where("status = 1").Order("created_at DESC").First(&record).Error; err == nil {
		lastBackup = record.CreatedAt
	}

	result := map[string]interface{}{
		"total_backups": totalBackups,
		"total_size":    totalSize,
	}
	if !lastBackup.IsZero() {
		result["last_backup"] = lastBackup
	}
	return result, nil
}

func (s *BackupService) resolveAppRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func (s *BackupService) resolveBackupDirectory(storagePath string) (string, error) {
	if strings.TrimSpace(storagePath) == "" {
		storagePath = "backups"
	}
	return s.resolvePath(storagePath)
}

func (s *BackupService) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	root, err := s.resolveAppRoot()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(root, path)), nil
}

func (s *BackupService) resolveSQLiteDatabasePath() (string, error) {
	if s.db == nil {
		return "", errors.New("database is not initialized")
	}

	driver := strings.ToLower(strings.TrimSpace(s.db.Dialector.Name()))
	if driver != "sqlite" {
		return "", fmt.Errorf("database backup is only supported for sqlite deployments; current driver is %s", driver)
	}

	var entries []sqliteDatabaseEntry
	if err := s.db.Raw("PRAGMA database_list").Scan(&entries).Error; err == nil {
		for _, entry := range entries {
			if entry.Name != "main" {
				continue
			}
			if strings.TrimSpace(entry.File) == "" {
				return "", errors.New("database backup is not available for in-memory sqlite databases")
			}
			return s.resolvePath(entry.File)
		}
	}

	if cfg := config.Get(); cfg != nil {
		dbPath := strings.TrimSpace(cfg.Database.Database)
		if dbPath == "" {
			dbPath = "config/data/v2board.db"
		}
		return s.resolvePath(dbPath)
	}

	return "", errors.New("unable to resolve sqlite database path")
}

func (s *BackupService) resolveOptionalSQLiteDatabasePath() (string, error) {
	if s.db == nil || strings.ToLower(strings.TrimSpace(s.db.Dialector.Name())) != "sqlite" {
		return "", nil
	}
	path, err := s.resolveSQLiteDatabasePath()
	if err != nil {
		if strings.Contains(err.Error(), "in-memory sqlite") || strings.Contains(err.Error(), "unable to resolve sqlite database path") {
			return "", nil
		}
		return "", err
	}
	return path, nil
}

func (s *BackupService) createSQLiteSnapshot(destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	quoted := strings.ReplaceAll(filepath.Clean(destination), "'", "''")
	if err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", quoted)).Error; err != nil {
		return fmt.Errorf("create sqlite backup snapshot: %w", err)
	}
	return nil
}

func (s *BackupService) createTemporarySQLiteSnapshot(backupDir string) (string, func(), error) {
	file, err := os.CreateTemp(backupDir, "sqlite-snapshot-*.db")
	if err != nil {
		return "", nil, err
	}
	snapshotPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(snapshotPath)
		return "", nil, err
	}
	if err := os.Remove(snapshotPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if err := s.createSQLiteSnapshot(snapshotPath); err != nil {
		return "", nil, err
	}
	return snapshotPath, func() {
		_ = os.Remove(snapshotPath)
	}, nil
}

func (s *BackupService) collectRuntimeFileSources(appRoot, backupDir, sqlitePath string) ([]backupArchiveSource, error) {
	candidates := []string{"config", "public", "data", ".env", ".env.example"}
	backupDir = filepath.Clean(backupDir)
	sqlitePath = filepath.Clean(sqlitePath)
	sqliteWalPath := sqlitePath + "-wal"
	sqliteShmPath := sqlitePath + "-shm"

	var sources []backupArchiveSource
	for _, candidate := range candidates {
		candidatePath := filepath.Join(appRoot, filepath.FromSlash(candidate))
		info, err := os.Stat(candidatePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		if !info.IsDir() {
			if s.shouldSkipBackupPath(candidatePath, backupDir, sqlitePath, sqliteWalPath, sqliteShmPath) {
				continue
			}
			rel, err := filepath.Rel(appRoot, candidatePath)
			if err != nil {
				return nil, err
			}
			sources = append(sources, backupArchiveSource{
				SourcePath:  candidatePath,
				ArchivePath: filepath.ToSlash(rel),
			})
			continue
		}

		err = filepath.Walk(candidatePath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				if isSameOrChildPath(path, backupDir) {
					return filepath.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() || s.shouldSkipBackupPath(path, backupDir, sqlitePath, sqliteWalPath, sqliteShmPath) {
				return nil
			}

			rel, err := filepath.Rel(appRoot, path)
			if err != nil {
				return err
			}
			sources = append(sources, backupArchiveSource{
				SourcePath:  path,
				ArchivePath: filepath.ToSlash(rel),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return sources, nil
}

func (s *BackupService) shouldSkipBackupPath(path, backupDir, sqlitePath, sqliteWalPath, sqliteShmPath string) bool {
	cleanPath := filepath.Clean(path)
	switch cleanPath {
	case sqlitePath, sqliteWalPath, sqliteShmPath:
		return true
	}
	return isSameOrChildPath(cleanPath, backupDir)
}

func (s *BackupService) writeBackupArchive(destination string, sources []backupArchiveSource, manifest backupArchiveManifest) (string, int64, error) {
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 0, err
	}

	file, err := os.Create(destination)
	if err != nil {
		return "", 0, err
	}

	writer := zip.NewWriter(file)
	if err := writeJSONArchiveEntry(writer, "meta/manifest.json", manifest); err != nil {
		_ = writer.Close()
		_ = file.Close()
		return "", 0, err
	}

	for _, source := range sources {
		if err := writeFileArchiveEntry(writer, source); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return "", 0, err
		}
	}

	if err := writer.Close(); err != nil {
		_ = file.Close()
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}

	info, err := os.Stat(destination)
	if err != nil {
		return "", 0, err
	}
	return destination, info.Size(), nil
}

func normalizeBackupType(backupType string) string {
	switch strings.ToLower(strings.TrimSpace(backupType)) {
	case "", "database":
		return "database"
	case "files":
		return "files"
	case "full":
		return "full"
	default:
		return strings.ToLower(strings.TrimSpace(backupType))
	}
}

func normalizeBackupStorageType(storageType string) string {
	normalized := strings.ToLower(strings.TrimSpace(storageType))
	if normalized == "" {
		return "local"
	}
	return normalized
}

func isSameOrChildPath(path, parent string) bool {
	if strings.TrimSpace(parent) == "" {
		return false
	}

	path = filepath.Clean(path)
	parent = filepath.Clean(parent)
	if path == parent {
		return true
	}

	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func resolveArchiveTargetPath(appRoot, archivePath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(archivePath))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid archive entry path: %s", archivePath)
	}
	return filepath.Join(appRoot, clean), nil
}

func extractArchiveFile(file *zip.File, destination string) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}

	target, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, reader)
	return err
}

func writeJSONArchiveEntry(writer *zip.Writer, name string, payload interface{}) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func writeFileArchiveEntry(writer *zip.Writer, source backupArchiveSource) error {
	info, err := os.Stat(source.SourcePath)
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(source.ArchivePath)
	header.Method = zip.Deflate

	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(source.SourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(entry, file)
	return err
}

func copyFileContents(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}

	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Sync()
}

type SystemConfigService struct {
	db *gorm.DB
}

func NewSystemConfigService(db *gorm.DB) *SystemConfigService {
	return &SystemConfigService{db: db}
}

func (s *SystemConfigService) Get(key string) (string, error) {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	return cfg.Value, err
}

func (s *SystemConfigService) GetEntry(key string) (*model.SystemConfig, error) {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *SystemConfigService) Set(key, value, cfgType, group, remark string) error {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error

	if err == gorm.ErrRecordNotFound {
		cfg = model.SystemConfig{
			Key:    key,
			Value:  value,
			Type:   cfgType,
			Group:  group,
			Remark: remark,
		}
		return s.db.Create(&cfg).Error
	}

	cfg.Value = value
	if cfgType != "" {
		cfg.Type = cfgType
	}
	if group != "" {
		cfg.Group = group
	}
	if remark != "" {
		cfg.Remark = remark
	}
	return s.db.Save(&cfg).Error
}

func (s *SystemConfigService) GetByGroup(group string) ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Where("\"group\" = ?", group).Find(&configs).Error
	return configs, err
}

func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Find(&configs).Error
	return configs, err
}

func (s *SystemConfigService) GetAsMap() (map[string]string, error) {
	configs, err := s.GetAll()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, cfg := range configs {
		result[cfg.Key] = cfg.Value
	}
	return result, nil
}

func (s *SystemConfigService) GetJSON(key string, v interface{}) error {
	value, err := s.Get(key)
	if err != nil {
		return err
	}
	if value == "" {
		return nil
	}
	return json.Unmarshal([]byte(value), v)
}

func (s *SystemConfigService) SetJSON(key string, v interface{}, group, remark string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Set(key, string(data), "json", group, remark)
}

func (s *SystemConfigService) Delete(key string) error {
	return s.db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}
