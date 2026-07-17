package service

import (
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	db *gorm.DB
}

// ErrUserNotFound is returned when a mutating user operation targets no row.
var ErrUserNotFound = errors.New("用户不存在")

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
	now := time.Now().Unix()
	err := s.db.Preload("Plan").
		Where("group_id = ?", groupID).
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now).
		Where("(u + d) < transfer_enable").
		Find(&users).Error
	return users, err
}

// GetActiveUsers 获取所有有效用户
func (s *UserService) GetActiveUsers() ([]model.User, error) {
	var users []model.User
	now := time.Now().Unix()
	err := s.db.Preload("Plan").
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now).
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
	if err := ValidateTrafficDelta(upload, download); err != nil {
		return err
	}
	return s.db.Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"u": gorm.Expr("u + ?", upload),
			"d": gorm.Expr("d + ?", download),
		}).Error
}

// BatchUpdateTraffic 批量更新用户流量
func (s *UserService) BatchUpdateTraffic(traffics map[uint][2]int64) error {
	for _, traffic := range traffics {
		if err := ValidateTrafficDelta(traffic[0], traffic[1]); err != nil {
			return err
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		for userID, traffic := range traffics {
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Updates(map[string]any{
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
func (s *UserService) Update(id uint, updates map[string]any) error {
	if err := s.ensureUserExists(id); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除用户
func (s *UserService) Delete(id uint) error {
	if id == 0 {
		return ErrUserNotFound
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := deleteWireGuardPeers(tx, "user_id = ?", id); err != nil {
			return err
		}
		res := tx.Delete(&model.User{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserNotFound
		}
		return nil
	})
}

// Ban 封禁用户
func (s *UserService) Ban(id uint) error {
	return s.Update(id, map[string]any{"banned": 1})
}

// Unban 解封用户
func (s *UserService) Unban(id uint) error {
	return s.Update(id, map[string]any{"banned": 0})
}

// ResetTraffic 重置用户流量
func (s *UserService) ResetTraffic(id uint) error {
	return s.Update(id, map[string]any{"u": 0, "d": 0})
}

// ResetToken 为用户重新生成订阅 token, 让旧的 /s/<token> 链接立即失效。
// 不改动 UUID, 所以节点端密码/连接不受影响, 用户只需重新导入订阅。
// 返回新生成的 token。
func (s *UserService) ResetToken(id uint) (string, error) {
	newToken := uuid.New().String()
	if err := s.Update(id, map[string]any{"token": newToken}); err != nil {
		return "", err
	}
	return newToken, nil
}

func (s *UserService) ensureUserExists(id uint) error {
	if id == 0 {
		return ErrUserNotFound
	}

	var user model.User
	if err := s.db.Select("id").First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

// GetStats 获取用户统计
func (s *UserService) GetStats() (map[string]any, error) {
	var totalUsers int64
	var activeUsers int64
	var expiredUsers int64
	var bannedUsers int64
	var todayNewUsers int64

	now := time.Now().Unix()
	todayStart := time.Now().Truncate(24 * time.Hour)

	if err := s.db.Model(&model.User{}).Count(&totalUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.User{}).
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now).
		Count(&activeUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.User{}).
		Where("expired_at IS NOT NULL AND expired_at <= ?", now).
		Count(&expiredUsers).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.User{}).Where("banned = 1").Count(&bannedUsers).Error; err != nil {
		return nil, err
	}

	// 今日新增用户
	if err := s.db.Model(&model.User{}).Where("created_at >= ?", todayStart).Count(&todayNewUsers).Error; err != nil {
		return nil, err
	}

	return map[string]any{
		"total_users":     totalUsers,
		"active_users":    activeUsers,
		"expired_users":   expiredUsers,
		"banned_users":    bannedUsers,
		"today_new_users": todayNewUsers,
	}, nil
}
