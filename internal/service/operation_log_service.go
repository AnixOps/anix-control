package service

import (
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

type OperationLogService struct {
	db *gorm.DB
}

type OperationLogInput struct {
	UserID     *uint
	Username   string
	Action     string
	Module     string
	TargetType string
	TargetID   *uint
	Content    string
	IP         string
	UserAgent  string
	Status     int
}

func NewOperationLogService(db *gorm.DB) *OperationLogService {
	return &OperationLogService{db: db}
}

func (s *OperationLogService) Record(input *OperationLogInput) error {
	if s == nil || s.db == nil || input == nil {
		return nil
	}

	status := input.Status
	if status == 0 {
		status = 1
	}

	entry := &model.OperationLog{
		UserID:     input.UserID,
		Username:   strings.TrimSpace(input.Username),
		Action:     strings.TrimSpace(input.Action),
		Module:     strings.TrimSpace(input.Module),
		TargetType: strings.TrimSpace(input.TargetType),
		TargetID:   input.TargetID,
		Content:    strings.TrimSpace(input.Content),
		IP:         strings.TrimSpace(input.IP),
		UserAgent:  strings.TrimSpace(input.UserAgent),
		Status:     status,
	}

	return s.db.Create(entry).Error
}
