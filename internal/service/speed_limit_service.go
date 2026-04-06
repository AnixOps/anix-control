package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	speedLimitStatusInactive = 0
	speedLimitStatusActive   = 1
)

type SpeedLimitInput struct {
	Name       string `json:"name"`
	Speed      int64  `json:"speed"`
	TunnelID   uint   `json:"tunnelId"`
	TunnelName string `json:"tunnelName"`
}

type SpeedLimitUpdateInput struct {
	ID uint `json:"id"`
	SpeedLimitInput
}

type SpeedLimitService struct {
	db *gorm.DB
}

func NewSpeedLimitService(db *gorm.DB) *SpeedLimitService {
	if db == nil {
		db = database.Get()
	}
	return &SpeedLimitService{db: db}
}

func (s *SpeedLimitService) Create(input SpeedLimitInput) (*model.SpeedLimit, error) {
	tunnel, err := s.validateTunnel(input.TunnelID, input.TunnelName)
	if err != nil {
		return nil, err
	}
	if err := validateSpeedLimitInput(input.Name, input.Speed); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	record := &model.SpeedLimit{
		Name:        strings.TrimSpace(input.Name),
		Speed:       input.Speed,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      speedLimitStatusActive,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (s *SpeedLimitService) List() ([]model.SpeedLimit, error) {
	var records []model.SpeedLimit
	if err := s.db.Order("id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *SpeedLimitService) Update(input SpeedLimitUpdateInput) (*model.SpeedLimit, error) {
	if input.ID == 0 {
		return nil, errors.New("id is required")
	}
	tunnel, err := s.validateTunnel(input.TunnelID, input.TunnelName)
	if err != nil {
		return nil, err
	}
	if err := validateSpeedLimitInput(input.Name, input.Speed); err != nil {
		return nil, err
	}

	var record model.SpeedLimit
	if err := s.db.First(&record, input.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("speed limit not found")
		}
		return nil, err
	}

	record.Name = strings.TrimSpace(input.Name)
	record.Speed = input.Speed
	record.TunnelID = tunnel.ID
	record.TunnelName = tunnel.Name
	record.UpdatedTime = time.Now().UnixMilli()
	if record.Status != speedLimitStatusActive && record.Status != speedLimitStatusInactive {
		record.Status = speedLimitStatusActive
	}

	if err := s.db.Save(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *SpeedLimitService) Delete(id uint) error {
	if id == 0 {
		return errors.New("id is required")
	}

	var record model.SpeedLimit
	if err := s.db.First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("speed limit not found")
		}
		return err
	}

	var assigned int64
	if err := s.db.Model(&model.ForwardUserTunnel{}).Where("speed_id = ?", id).Count(&assigned).Error; err != nil {
		return err
	}
	if assigned > 0 {
		return errors.New("speed limit is still assigned to user tunnels")
	}

	return s.db.Delete(&model.SpeedLimit{}, id).Error
}

func (s *SpeedLimitService) GetByIDs(ids []uint) (map[uint]model.SpeedLimit, error) {
	result := make(map[uint]model.SpeedLimit)
	if len(ids) == 0 {
		return result, nil
	}

	var records []model.SpeedLimit
	if err := s.db.Where("id IN ?", ids).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.ID] = record
	}
	return result, nil
}

func (s *SpeedLimitService) validateTunnel(tunnelID uint, tunnelName string) (*model.ForwardTunnel, error) {
	if tunnelID == 0 {
		return nil, errors.New("tunnelId is required")
	}
	trimmedName := strings.TrimSpace(tunnelName)
	if trimmedName == "" {
		return nil, errors.New("tunnelName is required")
	}

	var tunnel model.ForwardTunnel
	if err := s.db.First(&tunnel, tunnelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("tunnel not found")
		}
		return nil, err
	}
	if tunnel.Name != trimmedName {
		return nil, fmt.Errorf("tunnelName does not match tunnelId %d", tunnelID)
	}
	return &tunnel, nil
}

func validateSpeedLimitInput(name string, speed int64) error {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return errors.New("name is required")
	}
	if speed <= 0 {
		return errors.New("speed must be greater than 0")
	}
	return nil
}
