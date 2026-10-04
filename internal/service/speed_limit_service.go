package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
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
	db             *gorm.DB
	forwardService *PanelForwardService
}

func NewSpeedLimitService(db *gorm.DB) *SpeedLimitService {
	if db == nil {
		db = database.Get()
	}
	return &SpeedLimitService{
		db:             db,
		forwardService: NewPanelForwardService(db),
	}
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
