package service

import (
	"encoding/json"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The backups moved out of the service package: Control's bundles are
// internal/backup (docs/reference/backups.md). The former backup service,
// its settings (schedule, retention, S3, which nothing executed) and the
// online restore are gone; v2_backup_record and v2_backup_config stay in the
// schema, unused, until a cleanup migration drops them.

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

func (s *SystemConfigService) GetJSON(key string, v any) error {
	value, err := s.Get(key)
	if err != nil {
		return err
	}
	if value == "" {
		return nil
	}
	return json.Unmarshal([]byte(value), v)
}

func (s *SystemConfigService) SetJSON(key string, v any, group, remark string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Set(key, string(data), "json", group, remark)
}

func (s *SystemConfigService) Delete(key string) error {
	return s.db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}
