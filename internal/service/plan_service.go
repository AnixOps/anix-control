package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

var (
	ErrPlanNotFound     = errors.New("套餐不存在")
	ErrPlanUserNotFound = errors.New("用户不存在")
)

// PlanService 管理套餐
type PlanService struct {
	db *gorm.DB
}

// NewPlanService 创建 PlanService
func NewPlanService() *PlanService {
	return &PlanService{db: database.Get()}
}

// Create 创建套餐并写入事件
func (s *PlanService) Create(p *model.Plan) error {
	if err := s.db.Create(p).Error; err != nil {
		return err
	}
	s.emitEvent("plan.created", p)
	return nil
}

// Update 更新套餐并写事件
func (s *PlanService) Update(p *model.Plan) error {
	if p.ID == 0 {
		return ErrPlanNotFound
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Plan
		if err := tx.First(&existing, p.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlanNotFound
			}
			return err
		}

		p.CreatedAt = existing.CreatedAt
		return tx.Save(p).Error
	}); err != nil {
		return err
	}

	s.emitEvent("plan.updated", p)
	return nil
}

// Delete 删除套餐并写事件
func (s *PlanService) Delete(id uint) error {
	if id == 0 {
		return ErrPlanNotFound
	}

	var p model.Plan
	if err := s.db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlanNotFound
		}
		return err
	}
	if err := s.db.Delete(&model.Plan{}, id).Error; err != nil {
		return err
	}
	s.emitEvent("plan.deleted", &p)
	return nil
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

// List 获取所有套餐
func (s *PlanService) List() ([]*model.Plan, error) {
	var list []*model.Plan
	if err := s.db.Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// AssignToUser 将套餐分配给用户（同步修改用户记录），并写事件
func (s *PlanService) AssignToUser(planID, userID uint, expireAt *int64) error {
	if planID == 0 {
		return ErrPlanNotFound
	}
	if userID == 0 {
		return ErrPlanUserNotFound
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// 获取套餐详情
		var plan model.Plan
		if err := tx.First(&plan, planID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlanNotFound
			}
			return err
		}

		var user model.User
		if err := tx.Select("id").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlanUserNotFound
			}
			return err
		}

		// 更新用户
		updates := map[string]any{
			"plan_id":         plan.ID,
			"group_id":        plan.GroupID,
			"transfer_enable": plan.TransferEnable * 1073741824,
			"u":               0,
			"d":               0,
		}
		if expireAt != nil {
			updates["expired_at"] = *expireAt
		}
		if plan.SpeedLimit != nil {
			updates["speed_limit"] = *plan.SpeedLimit
		}
		if plan.DeviceLimit != nil {
			updates["device_limit"] = *plan.DeviceLimit
		}

		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return err
		}

		// 记录事件
		payload := map[string]any{"plan_id": planID, "user_id": userID}
		if expireAt != nil {
			payload["expired_at"] = *expireAt
		}
		b, _ := json.Marshal(payload)
		pt := string(b)
		ev := model.Event{Type: "plan.assigned", Payload: &pt, Status: "pending", CreatedAt: time.Now()}
		return tx.Create(&ev).Error
	})
}

// emitEvent 助手：把对象序列化写入事件表（异步处理系统可以消费）
func (s *PlanService) emitEvent(eventType string, obj any) {
	b, _ := json.Marshal(obj)
	pt := string(b)
	ev := model.Event{Type: eventType, Payload: &pt, Status: "pending", CreatedAt: time.Now()}
	_ = s.db.Create(&ev).Error
}
