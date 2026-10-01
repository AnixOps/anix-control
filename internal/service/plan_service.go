package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
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

// PlanAssignmentRequestID names an administrator's plan assignment in the
// subscriber request ledger (v4_kernel_subscriber_request), so a retried
// request is applied once. token identifies the HTTP request: its
// Idempotency-Key, else its request id. A new request is a new grant, as in
// v2. The plan package's native route derives the same id
// (packages/plan/native.AssignRequestID), so a retry is recognized whichever
// side serves it.
func PlanAssignmentRequestID(planID, userID uint, expireAt *int64, token string) string {
	expiry := "-"
	if expireAt != nil {
		expiry = strconv.FormatInt(*expireAt, 10)
	}
	sum := sha256.Sum256([]byte(token + "\x00" + expiry))
	return fmt.Sprintf("plan.assign:%d:%d:%x", planID, userID, sum[:12])
}

// AssignToUser 将套餐分配给用户（同步修改用户记录），并写事件。requestID
// (PlanAssignmentRequestID) applies a retried assignment once; the event is
// written again.
func (s *PlanService) AssignToUser(planID, userID uint, expireAt *int64, requestID string) error {
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

		// An administrator's assignment resets traffic, keeps the
		// subscription groups and changes the expiry only when given.
		if _, err := subscriber.ApplyEntitlementTx(tx, subscriber.Entitlement{
			RequestID: requestID, UserID: userID, Plan: subscriber.PlanSnapshotFromPlan(plan, nil), ExpiresAt: expireAt,
			ResetTraffic: true, KeepSubscriptionGroups: true,
		}, time.Now()); err != nil {
			if errors.Is(err, subscriber.ErrSubscriberNotFound) {
				return ErrPlanUserNotFound
			}
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
