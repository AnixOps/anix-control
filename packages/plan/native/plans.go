package native

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Messages of the kernel's service.ErrPlanNotFound and ErrPlanUserNotFound.
const (
	planNotFound     = "套餐不存在"
	planUserNotFound = "用户不存在"
)

var errPlanNotFound = errors.New(planNotFound)

// AdminList is GET /api/v2/admin/plans.
func (s *Service) AdminList(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取失败: " + err.Error())
	}
	var list []*Plan
	if err := db.Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		return s.panelError("获取失败: " + err.Error())
	}
	return s.panel(list)
}

// AdminCreate is POST /api/v2/admin/plans.
func (s *Service) AdminCreate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var p Plan
	if err := binding.JSON.BindBody(request.Body, &p); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("创建失败: " + err.Error())
	}
	if err := db.Create(&p).Error; err != nil {
		return s.panelError("创建失败: " + err.Error())
	}
	s.emit(db, "plan.created", &p)
	return s.panel(p)
}

// AdminGet is GET /api/v2/admin/plans/:id.
func (s *Service) AdminGet(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("ID 无效")
	}
	if id == 0 {
		return s.panelError(planNotFound)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取失败: " + err.Error())
	}
	var p Plan
	if err := db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(planNotFound)
		}
		return s.panelError("获取失败: " + err.Error())
	}
	return s.panel(&p)
}

// AdminUpdate is PUT /api/v2/admin/plans/:id. The body replaces every
// column but the creation time.
func (s *Service) AdminUpdate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("ID 无效")
	}
	var p Plan
	if err := binding.JSON.BindBody(request.Body, &p); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	p.ID = uint(id)
	if p.ID == 0 {
		return s.panelError(planNotFound)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("更新失败: " + err.Error())
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var existing Plan
		if err := tx.First(&existing, p.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errPlanNotFound
			}
			return err
		}
		p.CreatedAt = existing.CreatedAt
		return tx.Save(&p).Error
	})
	if err != nil {
		if errors.Is(err, errPlanNotFound) {
			return s.panelError(planNotFound)
		}
		return s.panelError("更新失败: " + err.Error())
	}
	s.emit(db, "plan.updated", &p)
	return s.panel(p)
}

// AdminDelete is DELETE /api/v2/admin/plans/:id.
func (s *Service) AdminDelete(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return s.panelError("ID 无效")
	}
	if id == 0 {
		return s.panelError(planNotFound)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("删除失败: " + err.Error())
	}
	var p Plan
	if err := db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(planNotFound)
		}
		return s.panelError("删除失败: " + err.Error())
	}
	if err := db.Delete(&Plan{}, id).Error; err != nil {
		return s.panelError("删除失败: " + err.Error())
	}
	s.emit(db, "plan.deleted", &p)
	return s.panel(map[string]any{"message": "删除成功"})
}

// UserPlans is GET /api/v2/user/plan: the plans on sale.
func (s *Service) UserPlans(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取套餐列表失败")
	}
	var plans []Plan
	if err := db.Where("show = ?", 1).Order("sort ASC").Find(&plans).Error; err != nil {
		return s.panelError("获取套餐列表失败")
	}
	return s.panel(plans)
}
