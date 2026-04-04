package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// BackupService 澶囦唤鏈嶅姟
type BackupService struct {
	db     *gorm.DB
	config *model.BackupConfig
}

// NewBackupService 鍒涘缓鏈嶅姟
func NewBackupService(db *gorm.DB) *BackupService {
	return &BackupService{db: db}
}

// GetConfig 鑾峰彇澶囦唤閰嶇疆
func (s *BackupService) GetConfig() (*model.BackupConfig, error) {
	if s.config != nil {
		return s.config, nil
	}

	var cfg model.BackupConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// 鍒涘缓榛樿閰嶇疆
		cfg = model.BackupConfig{
			Enabled:        false,
			AutoBackup:     false,
			RetentionDays:  7,
			BackupDatabase: true,
			StorageType:    "local",
			StoragePath:    "backups",
		}
		s.db.Create(&cfg)
	}
	s.config = &cfg
	return &cfg, err
}

// UpdateConfig 鏇存柊澶囦唤閰嶇疆
func (s *BackupService) UpdateConfig(cfg *model.BackupConfig) error {
	if err := s.db.Save(cfg).Error; err != nil {
		return err
	}
	s.config = cfg
	return nil
}

// CreateBackup 鍒涘缓澶囦唤
func (s *BackupService) CreateBackup(backupType string, createdBy *uint) (*model.BackupRecord, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}

	// 鍒涘缓澶囦唤璁板綍
	record := &model.BackupRecord{
		Name:      fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405")),
		Type:      backupType,
		Status:    0, // 杩涜涓?
		Auto:      false,
		CreatedBy: createdBy,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	// 纭繚澶囦唤鐩綍瀛樺湪
	backupDir := cfg.StoragePath
	if backupDir == "" {
		backupDir = "backups"
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		record.Status = 2
		record.Error = err.Error()
		s.db.Save(record)
		return nil, err
	}

	var backupErr error
	var backupPath string
	var backupSize int64

	switch backupType {
	case "full", "database":
		backupPath, backupSize, backupErr = s.backupDatabase(backupDir, record.Name)
	case "files":
		backupPath, backupSize, backupErr = s.backupFiles(backupDir, record.Name)
	default:
		backupErr = errors.New("unknown backup type")
	}

	// 鏇存柊璁板綍
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
	s.db.Save(record)

	return record, backupErr
}

// backupDatabase 澶囦唤鏁版嵁搴?
func (s *BackupService) backupDatabase(backupDir, name string) (string, int64, error) {
	dbPath := "config/data/v2board.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", 0, errors.New("database file not found")
	}

	backupPath := filepath.Join(backupDir, name+".db")

	// 澶嶅埗鏁版嵁搴撴枃浠?
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return "", 0, err
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", 0, err
	}

	// 鑾峰彇鏂囦欢澶у皬
	info, _ := os.Stat(backupPath)

	return backupPath, info.Size(), nil
}

// backupFiles 澶囦唤鏂囦欢
func (s *BackupService) backupFiles(backupDir, name string) (string, int64, error) {
	// TODO: 瀹炵幇鏂囦欢澶囦唤閫昏緫
	return "", 0, nil
}

// ListBackups 鑾峰彇澶囦唤鍒楄〃
func (s *BackupService) ListBackups(page, pageSize int) ([]model.BackupRecord, int64, error) {
	var records []model.BackupRecord
	var total int64

	s.db.Model(&model.BackupRecord{}).Count(&total)

	offset := (page - 1) * pageSize
	err := s.db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error

	return records, total, err
}

// DeleteBackup 鍒犻櫎澶囦唤
func (s *BackupService) DeleteBackup(id uint) error {
	var record model.BackupRecord
	if err := s.db.First(&record, id).Error; err != nil {
		return err
	}

	// 鍒犻櫎鏂囦欢
	if record.Path != "" {
		os.Remove(record.Path)
	}

	return s.db.Delete(&record).Error
}

// RestoreBackup 鎭㈠澶囦唤
func (s *BackupService) RestoreBackup(id uint) error {
	var record model.BackupRecord
	if err := s.db.First(&record, id).Error; err != nil {
		return err
	}

	if record.Status != 1 {
		return errors.New("backup not successful")
	}

	if record.Type == "database" || record.Type == "full" {
		return s.restoreDatabase(record.Path)
	}

	return errors.New("unsupported backup type")
}

// restoreDatabase 鎭㈠鏁版嵁搴?
func (s *BackupService) restoreDatabase(backupPath string) error {
	dbPath := "config/data/v2board.db"

	// 璇诲彇澶囦唤鏂囦欢
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}

	// 鍏抽棴褰撳墠鏁版嵁搴撹繛鎺?
	database.Close()

	// 鍐欏叆鎭㈠鐨勬暟鎹?
	if err := os.WriteFile(dbPath, data, 0644); err != nil {
		return err
	}

	return nil
}

// CleanupOldBackups 娓呯悊鏃у浠?
func (s *BackupService) CleanupOldBackups() error {
	cfg, _ := s.GetConfig()
	if cfg == nil || cfg.RetentionDays <= 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionDays)

	// 鏌ユ壘闇€瑕佸垹闄ょ殑澶囦唤
	var records []model.BackupRecord
	s.db.Where("created_at < ? AND status = 1", cutoff).Find(&records)

	for _, record := range records {
		// 鍒犻櫎鏂囦欢
		if record.Path != "" {
			os.Remove(record.Path)
		}
		// 鍒犻櫎璁板綍
		s.db.Delete(&record)
	}

	return nil
}

// GetBackupStats 鑾峰彇澶囦唤缁熻
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

// SystemConfigService 绯荤粺閰嶇疆鏈嶅姟
type SystemConfigService struct {
	db *gorm.DB
}

// NewSystemConfigService 鍒涘缓鏈嶅姟
func NewSystemConfigService(db *gorm.DB) *SystemConfigService {
	return &SystemConfigService{db: db}
}

// Get 鑾峰彇閰嶇疆
func (s *SystemConfigService) Get(key string) (string, error) {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	return cfg.Value, err
}

// Set 璁剧疆閰嶇疆
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

// GetByGroup 鎸夌粍鑾峰彇閰嶇疆
func (s *SystemConfigService) GetByGroup(group string) ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Where("\"group\" = ?", group).Find(&configs).Error
	return configs, err
}

// GetAll 鑾峰彇鎵€鏈夐厤缃?
func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Find(&configs).Error
	return configs, err
}

// GetAsMap 鑾峰彇閰嶇疆Map
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

// GetJSON 鑾峰彇JSON閰嶇疆
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

// SetJSON 璁剧疆JSON閰嶇疆
func (s *SystemConfigService) SetJSON(key string, v interface{}, group, remark string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Set(key, string(data), "json", group, remark)
}

// Delete 鍒犻櫎閰嶇疆
func (s *SystemConfigService) Delete(key string) error {
	return s.db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}
