package native

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The kernel's subscription service errors; their messages are answered as
// they are.
var (
	errUserNotFound      = errors.New("用户不存在")
	errPlanNotFound      = errors.New("套餐不存在")
	errGroupNotFound     = errors.New(groupNotFound)
	errPlanGroupNotFound = errors.New("套餐订阅分组不存在")
	errUserGroupNotFound = errors.New("用户订阅分组不存在")
	errProtocolNotFound  = errors.New("协议不存在")
)

// bindingError is the kernel's panelSubscriptionBindingError.
func (s *Service) bindingError(fallback string, err error) (pluginhostsdk.NativeResponse, error) {
	switch {
	case errors.Is(err, errUserNotFound), errors.Is(err, errPlanNotFound), errors.Is(err, errGroupNotFound),
		errors.Is(err, errUserGroupNotFound), errors.Is(err, errPlanGroupNotFound):
		return s.panelError(err.Error())
	default:
		return s.panelError(fallback + ": " + err.Error())
	}
}

// planExists is the kernel's subscriptionPlanExists, on kapi_plan_catalog_v1.
func planExists(db *gorm.DB, planID uint) error {
	var plan PlanCatalog
	if err := db.Select("id").First(&plan, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPlanNotFound
		}
		return err
	}
	return nil
}

// userExists is the kernel's subscriptionUserExists, on
// kapi_subscriber_entitlement_v1 (a row per v2_user row).
func userExists(db *gorm.DB, userID uint) error {
	var user Entitlement
	if err := db.Select("id").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errUserNotFound
		}
		return err
	}
	return nil
}

// PlanGroups is GET /api/v2/admin/subscription/plans/:plan_id/groups.
func (s *Service) PlanGroups(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	planID, ok := pathUint(request, "plan_id")
	if !ok {
		return s.panelError("套餐 ID 无效")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("获取失败", err)
	}
	if err := planExists(db, planID); err != nil {
		return s.bindingError("获取失败", err)
	}
	var groups []*model.SubscriptionGroup
	if err := db.Joins("JOIN v2_plan_subscription_group ON v2_subscription_group.id = v2_plan_subscription_group.group_id").
		Where("v2_plan_subscription_group.plan_id = ?", planID).
		Find(&groups).Error; err != nil {
		return s.bindingError("获取失败", err)
	}
	return s.panel(groups)
}

// AssignPlanGroup is POST /api/v2/admin/subscription/plans/:plan_id/groups:
// the plan grants the group, once.
func (s *Service) AssignPlanGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	planID, ok := pathUint(request, "plan_id")
	if !ok {
		return s.panelError("套餐 ID 无效")
	}
	var body struct {
		GroupID uint `json:"group_id" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &body); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("分配失败", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := planExists(tx, planID); err != nil {
			return err
		}
		if err := groupExists(tx, body.GroupID); err != nil {
			return err
		}
		link := model.PlanSubscriptionGroup{PlanID: planID, GroupID: body.GroupID}
		return tx.Where("plan_id = ? AND group_id = ?", planID, body.GroupID).FirstOrCreate(&link).Error
	})
	if err != nil {
		return s.bindingError("分配失败", err)
	}
	return s.panel(map[string]any{"message": "分配成功"})
}

// RemovePlanGroup is DELETE
// /api/v2/admin/subscription/plans/:plan_id/groups/:group_id.
func (s *Service) RemovePlanGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	planID, ok := pathUint(request, "plan_id")
	if !ok {
		return s.panelError("套餐 ID 无效")
	}
	groupID, ok := pathUint(request, "group_id")
	if !ok {
		return s.panelError("分组 ID 无效")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("移除失败", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := planExists(tx, planID); err != nil {
			return err
		}
		if err := groupExists(tx, groupID); err != nil {
			return err
		}
		result := tx.Where("plan_id = ? AND group_id = ?", planID, groupID).Delete(&model.PlanSubscriptionGroup{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errPlanGroupNotFound
		}
		return nil
	})
	if err != nil {
		return s.bindingError("移除失败", err)
	}
	return s.panel(map[string]any{"message": "移除成功"})
}

// UserGroups is GET /api/v2/admin/subscription/users/:user_id/groups: the
// groups a user holds and that have not expired, from
// kapi_user_subscription_group_v1.
func (s *Service) UserGroups(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID, ok := pathUint(request, "user_id")
	if !ok {
		return s.panelError("用户 ID 无效")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("获取失败", err)
	}
	if err := userExists(db, userID); err != nil {
		return s.bindingError("获取失败", err)
	}
	var groups []*model.SubscriptionGroup
	members := UserGroup{}.TableName()
	if err := db.Joins("JOIN "+members+" ON v2_subscription_group.id = "+members+".group_id").
		Where(members+".user_id = ? AND ("+members+".expire_at IS NULL OR "+members+".expire_at > ?)", userID, s.now().Unix()).
		Find(&groups).Error; err != nil {
		return s.bindingError("获取失败", err)
	}
	return s.panel(groups)
}

// LinkProtocols is POST /api/v2/admin/subscription/groups/:id/protocols:
// the group's node protocols become exactly the listed ones. A protocol is
// known by kapi_node_protocol_v1; the link rows are the adopted
// v2_subscription_group_node_protocols. The kernel's association replace
// also touches the group's updated_at, and so does this.
func (s *Service) LinkProtocols(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	groupID, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	var body struct {
		ProtocolIDs []uint `json:"protocol_ids" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &body); err != nil {
		return s.panelError(err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	err = s.linkProtocols(db, groupID, body.ProtocolIDs)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return s.panelError(groupNotFound)
	case errors.Is(err, errProtocolNotFound):
		return s.panelError(errProtocolNotFound.Error())
	case err != nil:
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "更新成功"})
}

func (s *Service) linkProtocols(db *gorm.DB, groupID uint, protocolIDs []uint) error {
	var group model.SubscriptionGroup
	if err := db.First(&group, groupID).Error; err != nil {
		return err
	}
	unique := make([]uint, 0, len(protocolIDs))
	seen := make(map[uint]struct{}, len(protocolIDs))
	for _, id := range protocolIDs {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) > 0 {
		var count int64
		if err := db.Model(&NodeProtocol{}).Where("id IN ?", unique).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(unique)) {
			return errProtocolNotFound
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if len(unique) > 0 {
			links := make([]model.GroupProtocol, 0, len(unique))
			for _, id := range unique {
				links = append(links, model.GroupProtocol{SubscriptionGroupID: group.ID, NodeProtocolID: id})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.SubscriptionGroup{}).Where("id = ?", group.ID).Update("updated_at", s.now()).Error; err != nil {
			return err
		}
		stale := tx.Where("subscription_group_id = ?", group.ID)
		if len(unique) > 0 {
			stale = stale.Where("node_protocol_id NOT IN ?", unique)
		}
		return stale.Delete(&model.GroupProtocol{}).Error
	})
}
