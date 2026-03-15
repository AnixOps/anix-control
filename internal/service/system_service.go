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

// BackupService 备份服务
type BackupService struct {
	db     *gorm.DB
	config *model.BackupConfig
}

// NewBackupService 创建服务
func NewBackupService(db *gorm.DB) *BackupService {
	return &BackupService{db: db}
}

// GetConfig 获取备份配置
func (s *BackupService) GetConfig() (*model.BackupConfig, error) {
	if s.config != nil {
		return s.config, nil
	}

	var cfg model.BackupConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// 创建默认配置
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

// UpdateConfig 更新备份配置
func (s *BackupService) UpdateConfig(cfg *model.BackupConfig) error {
	if err := s.db.Save(cfg).Error; err != nil {
		return err
	}
	s.config = cfg
	return nil
}

// CreateBackup 创建备份
func (s *BackupService) CreateBackup(backupType string, createdBy *uint) (*model.BackupRecord, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}

	// 创建备份记录
	record := &model.BackupRecord{
		Name:      fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405")),
		Type:      backupType,
		Status:    0, // 进行中
		Auto:      false,
		CreatedBy: createdBy,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	// 确保备份目录存在
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

	// 更新记录
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

// backupDatabase 备份数据库
func (s *BackupService) backupDatabase(backupDir, name string) (string, int64, error) {
	dbPath := "data/v2board.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", 0, errors.New("database file not found")
	}

	backupPath := filepath.Join(backupDir, name+".db")

	// 复制数据库文件
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return "", 0, err
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", 0, err
	}

	// 获取文件大小
	info, _ := os.Stat(backupPath)

	return backupPath, info.Size(), nil
}

// backupFiles 备份文件
func (s *BackupService) backupFiles(backupDir, name string) (string, int64, error) {
	// TODO: 实现文件备份逻辑
	return "", 0, nil
}

// ListBackups 获取备份列表
func (s *BackupService) ListBackups(page, pageSize int) ([]model.BackupRecord, int64, error) {
	var records []model.BackupRecord
	var total int64

	s.db.Model(&model.BackupRecord{}).Count(&total)

	offset := (page - 1) * pageSize
	err := s.db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records).Error

	return records, total, err
}

// DeleteBackup 删除备份
func (s *BackupService) DeleteBackup(id uint) error {
	var record model.BackupRecord
	if err := s.db.First(&record, id).Error; err != nil {
		return err
	}

	// 删除文件
	if record.Path != "" {
		os.Remove(record.Path)
	}

	return s.db.Delete(&record).Error
}

// RestoreBackup 恢复备份
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

// restoreDatabase 恢复数据库
func (s *BackupService) restoreDatabase(backupPath string) error {
	dbPath := "data/v2board.db"

	// 读取备份文件
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}

	// 关闭当前数据库连接
	database.Close()

	// 写入恢复的数据
	if err := os.WriteFile(dbPath, data, 0644); err != nil {
		return err
	}

	return nil
}

// CleanupOldBackups 清理旧备份
func (s *BackupService) CleanupOldBackups() error {
	cfg, _ := s.GetConfig()
	if cfg == nil || cfg.RetentionDays <= 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionDays)

	// 查找需要删除的备份
	var records []model.BackupRecord
	s.db.Where("created_at < ? AND status = 1", cutoff).Find(&records)

	for _, record := range records {
		// 删除文件
		if record.Path != "" {
			os.Remove(record.Path)
		}
		// 删除记录
		s.db.Delete(&record)
	}

	return nil
}

// GetBackupStats 获取备份统计
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

// SystemConfigService 系统配置服务
type SystemConfigService struct {
	db *gorm.DB
}

// NewSystemConfigService 创建服务
func NewSystemConfigService(db *gorm.DB) *SystemConfigService {
	return &SystemConfigService{db: db}
}

// Get 获取配置
func (s *SystemConfigService) Get(key string) (string, error) {
	var cfg model.SystemConfig
	err := s.db.Where("key = ?", key).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	return cfg.Value, err
}

// Set 设置配置
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

// GetByGroup 按组获取配置
func (s *SystemConfigService) GetByGroup(group string) ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Where("\"group\" = ?", group).Find(&configs).Error
	return configs, err
}

// GetAll 获取所有配置
func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := s.db.Find(&configs).Error
	return configs, err
}

// GetAsMap 获取配置Map
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

// GetJSON 获取JSON配置
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

// SetJSON 设置JSON配置
func (s *SystemConfigService) SetJSON(key string, v interface{}, group, remark string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Set(key, string(data), "json", group, remark)
}

// Delete 删除配置
func (s *SystemConfigService) Delete(key string) error {
	return s.db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}