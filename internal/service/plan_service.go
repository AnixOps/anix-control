package service

import (
	"errors"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

var ErrPlanNotFound = errors.New("套餐不存在")

// PlanService 管理套餐
type PlanService struct {
	db *gorm.DB
}

// NewPlanService 创建 PlanService
func NewPlanService() *PlanService {
	return PlanServiceOn(database.Get())
}

// PlanServiceOn returns a PlanService on db.
func PlanServiceOn(db *gorm.DB) *PlanService {
	return &PlanService{db: db}
}

// Get 获取单个套餐
func (s *PlanService) Get(id uint) (*model.Plan, error) {
	if id == 0 {
		return nil, ErrPlanNotFound
	}

	var p model.Plan
	if err := s.db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, err
	}
	return &p, nil
}
