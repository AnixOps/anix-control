package service

import (
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	db *gorm.DB
}

// NewUserService 创建用户服务
func NewUserService() *UserService {
	return &UserService{
		db: database.Get(),
	}
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(id uint) (*model.User, error) {
	var user model.User
	err := s.db.Preload("Plan").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUUID 根据UUID获取用户
func (s *UserService) GetByUUID(uuid string) (*model.User, error) {
	var user model.User
	err := s.db.Preload("Plan").Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByToken 根据Token获取用户
func (s *UserService) GetByToken(token string) (*model.User, error) {
	var user model.User
	err := s.db.Preload("Plan").Where("token = ?", token).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail 根据Email获取用户
func (s *UserService) GetByEmail(email string) (*model.User, error) {
	var user model.User
	err := s.db.Preload("Plan").Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetActiveUsersByGroupID 获取指定分组的有效用户
func (s *UserService) GetActiveUsersByGroupID(groupID uint) ([]model.User, error) {
	var users []model.User
	err := s.db.Preload("Plan").
		Where("group_id = ?", groupID).
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > UNIX_TIMESTAMP())").
		Where("(u + d) < transfer_enable").
		Find(&users).Error
	return users, err
}

// GetActiveUsers 获取所有有效用户
func (s *UserService) GetActiveUsers() ([]model.User, error) {
	var users []model.User
	err := s.db.Preload("Plan").
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > UNIX_TIMESTAMP())").
		Where("(u + d) < transfer_enable").
		Find(&users).Error
	return users, err
}

// GetActiveUsersForNode 获取节点可用的有效用户
// groupID 为 nil 时返回所有有效用户，否则返回指定分组的用户
func (s *UserService) GetActiveUsersForNode(groupID *uint) ([]*model.User, error) {
	var users []*model.User
	query := s.db.Preload("Plan").
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", time.Now().Unix()).
		Where("(u + d) < transfer_enable")

	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
	}

	err := query.Find(&users).Error
	return users, err
}

// UpdateTraffic 更新用户流量
func (s *UserService) UpdateTraffic(userID uint, upload, download int64) error {
	return s.db.Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"u": gorm.Expr("u + ?", upload),
			"d": gorm.Expr("d + ?", download),
		}).Error
}

// BatchUpdateTraffic 批量更新用户流量
func (s *UserService) BatchUpdateTraffic(traffics map[uint][2]int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for userID, traffic := range traffics {
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Updates(map[string]interface{}{
					"u": gorm.Expr("u + ?", traffic[0]),
					"d": gorm.Expr("d + ?", traffic[1]),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UserListParams 用户列表查询参数
type UserListParams struct {
	Page     int
	PageSize int
	Email    string
	PlanID   *uint
	Status   string // all, active, expired, banned
	OrderBy  string
}

// UserListResult 用户列表结果
type UserListResult struct {
	Total int64        `json:"total"`
	List  []model.User `json:"list"`
}

// GetList 获取用户列表
func (s *UserService) GetList(params UserListParams) (*UserListResult, error) {
	var users []model.User
	var total int64

	query := s.db.Model(&model.User{})

	// 筛选条件
	if params.Email != "" {
		query = query.Where("email LIKE ?", "%"+params.Email+"%")
	}
	if params.PlanID != nil {
		query = query.Where("plan_id = ?", *params.PlanID)
	}
	switch params.Status {
	case "active":
		now := time.Now().Unix()
		query = query.Where("banned = 0").
			Where("(expired_at IS NULL OR expired_at > ?)", now)
	case "expired":
		now := time.Now().Unix()
		query = query.Where("expired_at IS NOT NULL AND expired_at <= ?", now)
	case "banned":
		query = query.Where("banned = 1")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序
	orderBy := "created_at DESC"
	if params.OrderBy != "" {
		orderBy = params.OrderBy
	}

	// 分页
	offset := (params.Page - 1) * params.PageSize
	if err := query.Preload("Plan").
		Order(orderBy).
		Offset(offset).
		Limit(params.PageSize).
		Find(&users).Error; err != nil {
		return nil, err
	}

	return &UserListResult{
		Total: total,
		List:  users,
	}, nil
}

// Create 创建用户
func (s *UserService) Create(user *model.User) error {
	return s.db.Create(user).Error
}

// Update 更新用户
func (s *UserService) Update(id uint, updates map[string]interface{}) error {
	return s.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除用户
func (s *UserService) Delete(id uint) error {
	return s.db.Delete(&model.User{}, id).Error
}

// Ban 封禁用户
func (s *UserService) Ban(id uint) error {
	return s.Update(id, map[string]interface{}{"banned": 1})
}

// Unban 解封用户
func (s *UserService) Unban(id uint) error {
	return s.Update(id, map[string]interface{}{"banned": 0})
}

// ResetTraffic 重置用户流量
func (s *UserService) ResetTraffic(id uint) error {
	return s.Update(id, map[string]interface{}{"u": 0, "d": 0})
}

// GetStats 获取用户统计
func (s *UserService) GetStats() (map[string]interface{}, error) {
	var totalUsers int64
	var activeUsers int64
	var expiredUsers int64
	var bannedUsers int64
	var todayNewUsers int64

	now := time.Now().Unix()
	todayStart := time.Now().Truncate(24 * time.Hour)

	s.db.Model(&model.User{}).Count(&totalUsers)
	s.db.Model(&model.User{}).
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now).
		Count(&activeUsers)
	s.db.Model(&model.User{}).
		Where("expired_at IS NOT NULL AND expired_at <= ?", now).
		Count(&expiredUsers)
	s.db.Model(&model.User{}).Where("banned = 1").Count(&bannedUsers)

	// 今日新增用户
	s.db.Model(&model.User{}).Where("created_at >= ?", todayStart).Count(&todayNewUsers)

	return map[string]interface{}{
		"total_users":     totalUsers,
		"active_users":    activeUsers,
		"expired_users":   expiredUsers,
		"banned_users":    bannedUsers,
		"today_new_users": todayNewUsers,
	}, nil
}
