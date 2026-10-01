package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
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

// ActiveUsersForNodeQuery restricts a v2_user query to the users a node
// serves: the active subscribers (subscriber.Active) of the node's plan
// group, or every active subscriber when groupID is nil. The legacy user
// pulls (UniProxy user, v2board GetUsers) and the Agent Control user deltas
// read the same query, so a node gets the same set on either transport.
func ActiveUsersForNodeQuery(db *gorm.DB, groupID *uint, now time.Time) *gorm.DB {
	query := subscriber.Active(db, now)
	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
	}
	return query
}

// GetActiveUsersForNode 获取节点可用的有效用户
// groupID 为 nil 时返回所有有效用户，否则返回指定分组的用户
func (s *UserService) GetActiveUsersForNode(groupID *uint) ([]*model.User, error) {
	var users []*model.User
	err := ActiveUsersForNodeQuery(s.db.Preload("Plan"), groupID, time.Now()).Find(&users).Error
	return users, err
}

// UpdateTraffic 更新用户流量
func (s *UserService) UpdateTraffic(userID uint, upload, download int64) error {
	if err := ValidateTrafficDelta(upload, download); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		_, err := subscriber.RecordTrafficTx(tx, "", []subscriber.TrafficEntry{{UserID: userID, Upload: upload, Download: download}}, time.Now())
		return err
	})
}

// BatchUpdateTraffic 批量更新用户流量
func (s *UserService) BatchUpdateTraffic(traffics map[uint][2]int64) error {
	for _, traffic := range traffics {
		if err := ValidateTrafficDelta(traffic[0], traffic[1]); err != nil {
			return err
		}
	}

	entries := make([]subscriber.TrafficEntry, 0, len(traffics))
	for userID, traffic := range traffics {
		entries = append(entries, subscriber.TrafficEntry{UserID: userID, Upload: traffic[0], Download: traffic[1]})
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		_, err := subscriber.RecordTrafficTx(tx, "", entries, time.Now())
		return err
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
	Total int64          `json:"total"`
	List  []UserListItem `json:"list"`
}

// UserListItem is one user in the administrator's list: the account
// (e-mail, administrator, staff and ban flags), the subscription summary
// (plan, group, traffic, limits, reset day, expiry, balances) and the plan's
// id and name: the columns of kapi_user_directory_v1,
// kapi_subscriber_entitlement_v1 and kapi_plan_name_v1. A plan that no
// longer exists is left out. It carries no credential: the subscription
// token and proxy uuid, and the rest of the row, are read one user at a time
// from GET /api/v2/admin/users/:id. identity-platform's native list answers
// the same fields (packages/identity-platform/native/directory.go).
type UserListItem struct {
	ID                uint      `json:"id"`
	Email             string    `json:"email"`
	Balance           int64     `json:"balance"`
	CommissionBalance int64     `json:"commission_balance"`
	DeviceLimit       *int      `json:"device_limit"`
	SpeedLimit        *int64    `json:"speed_limit"`
	FlowResetTime     int64     `json:"flowResetTime"`
	TransferEnable    int64     `json:"transfer_enable"`
	U                 int64     `json:"u"`
	D                 int64     `json:"d"`
	PlanID            *uint     `json:"plan_id"`
	GroupID           *uint     `json:"group_id"`
	ExpiredAt         *int64    `json:"expired_at"`
	Banned            int       `json:"banned"`
	IsAdmin           int       `json:"is_admin"`
	IsStaff           int       `json:"is_staff"`
	CreatedAt         time.Time `json:"created_at"`
	// Plan is never loaded with the row.
	Plan *UserListPlan `gorm:"-" json:"plan,omitempty"`
}

// UserListPlan names a listed user's plan: its id and name, never the rest
// of the plan row.
type UserListPlan struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// userListColumns are the v2_user columns of a UserListItem.
const userListColumns = "id, email, balance, commission_balance, device_limit, speed_limit, flow_reset_time, transfer_enable, " +
	"u, d, plan_id, group_id, expired_at, banned, is_admin, is_staff, created_at"

// GetList 获取用户列表. Users created in the same instant keep a stable
// order (created_at DESC, id DESC), so pages neither repeat nor skip a user.
func (s *UserService) GetList(params UserListParams) (*UserListResult, error) {
	users := []UserListItem{}
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
	orderBy := "created_at DESC, id DESC"
	if params.OrderBy != "" {
		orderBy = params.OrderBy
	}

	// 分页
	offset := (params.Page - 1) * params.PageSize
	if err := query.Select(userListColumns).
		Order(orderBy).
		Offset(offset).
		Limit(params.PageSize).
		Find(&users).Error; err != nil {
		return nil, err
	}
	if err := s.nameListedPlans(users); err != nil {
		return nil, err
	}

	return &UserListResult{
		Total: total,
		List:  users,
	}, nil
}

// nameListedPlans gives each listed user on a plan that still exists the
// plan's id and name.
func (s *UserService) nameListedPlans(users []UserListItem) error {
	planIDs := make([]uint, 0, len(users))
	for _, user := range users {
		if user.PlanID != nil {
			planIDs = append(planIDs, *user.PlanID)
		}
	}
	if len(planIDs) == 0 {
		return nil
	}
	var plans []UserListPlan
	if err := s.db.Model(&model.Plan{}).Select("id", "name").Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
		return err
	}
	byID := make(map[uint]*UserListPlan, len(plans))
	for i := range plans {
		byID[plans[i].ID] = &plans[i]
	}
	for i := range users {
		if users[i].PlanID != nil {
			users[i].Plan = byID[*users[i].PlanID]
		}
	}
	return nil
}

// Create 创建用户
func (s *UserService) Create(user *model.User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return subscriber.RecordChangesTx(tx, []uint{user.ID}, false, time.Now())
	})
}

// revokingUserFields are the user columns whose change ends the user's
// sessions: tokens issued before it stop working.
var revokingUserFields = []string{"password", "email", "is_admin", "banned"}

// Update 更新用户
func (s *UserService) Update(id uint, updates map[string]any) error {
	if err := s.ensureUserExists(id); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	var revocation *authn.Revocation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		revocation, err = UpdateUserTx(tx, id, updates)
		return err
	})
	if err != nil {
		return err
	}
	if revocation != nil {
		authn.Remember(*revocation)
	}
	return nil
}

// UpdateUserTx updates a user in tx. When the change ends the user's
// sessions it also records the revocation and returns it; apply it with
// authn.Remember after the commit.
func UpdateUserTx(tx *gorm.DB, id uint, updates map[string]any) (*authn.Revocation, error) {
	return UpdateUserTxWithTokenVersion(tx, id, updates, 0)
}

// UpdateUserTxWithTokenVersion is UpdateUserTx for a change identity made:
// tokenVersion, when positive, is the account's new token version, so the
// revocation ends identity tokens by version rather than by time.
func UpdateUserTxWithTokenVersion(tx *gorm.DB, id uint, updates map[string]any, tokenVersion uint64) (*authn.Revocation, error) {
	reason, err := sessionEndingChange(tx, id, updates)
	if err != nil {
		return nil, err
	}
	if err := tx.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	if subscriber.TouchesNodeFields(updates) {
		if err := subscriber.RecordChangesTx(tx, []uint{id}, false, time.Now()); err != nil {
			return nil, err
		}
	}
	if reason == "" {
		return nil, nil
	}
	revocation := authn.UserRevocation(id, reason)
	revocation.TokenVersion = tokenVersion
	if err := authn.Write(tx, revocation); err != nil {
		return nil, err
	}
	return &revocation, nil
}

// sessionEndingChange names the first revoking field that updates actually
// changes; forms that resend unchanged values keep the user's sessions.
func sessionEndingChange(tx *gorm.DB, id uint, updates map[string]any) (string, error) {
	present := false
	for _, field := range revokingUserFields {
		if _, ok := updates[field]; ok {
			present = true
			break
		}
	}
	if !present {
		return "", nil
	}
	var current model.User
	if err := tx.Select("id", "password", "email", "is_admin", "banned").Where("id = ?", id).Take(&current).Error; err != nil {
		return "", err
	}
	was := map[string]string{
		"password": current.Password,
		"email":    current.Email,
		"is_admin": fmt.Sprint(current.IsAdmin),
		"banned":   fmt.Sprint(current.Banned),
	}
	for _, field := range revokingUserFields {
		if value, ok := updates[field]; ok && fmt.Sprint(value) != was[field] {
			return "user " + field + " changed", nil
		}
	}
	return "", nil
}

// Delete 删除用户
func (s *UserService) Delete(id uint) error {
	if id == 0 {
		return ErrUserNotFound
	}
	var revocation authn.Revocation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		revocation, err = DeleteUserTx(tx, id)
		return err
	})
	if err != nil {
		return err
	}
	authn.Remember(revocation)
	return nil
}

// DeleteUserTx deletes a user and its WireGuard peers in tx and records the
// revocation of its tokens; apply it with authn.Remember after the commit.
func DeleteUserTx(tx *gorm.DB, id uint) (authn.Revocation, error) {
	revocation := authn.UserRevocation(id, "user deleted")
	if err := deleteWireGuardPeers(tx, "user_id = ?", id); err != nil {
		return revocation, err
	}
	res := tx.Delete(&model.User{}, id)
	if res.Error != nil {
		return revocation, res.Error
	}
	if res.RowsAffected == 0 {
		return revocation, ErrUserNotFound
	}
	if err := subscriber.RecordChangesTx(tx, []uint{id}, true, time.Now()); err != nil {
		return revocation, err
	}
	return revocation, authn.Write(tx, revocation)
}

// Ban 封禁用户
func (s *UserService) Ban(id uint) error {
	return s.Update(id, map[string]any{"banned": 1})
}

// Unban 解封用户
func (s *UserService) Unban(id uint) error {
	return s.Update(id, map[string]any{"banned": 0})
}

// ResetTraffic 重置用户流量. A non-empty requestID (AdminUserResetRequestID)
// applies a retried reset once, through the subscriber request ledger.
func (s *UserService) ResetTraffic(id uint, requestID string) error {
	if err := s.ensureUserExists(id); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		_, err := subscriber.ResetTrafficTx(tx, requestID, []uint{id}, time.Now())
		return err
	})
}

// ResetToken 为用户重新生成订阅 token, 让旧的 /s/<token> 链接立即失效。
// 不改动 UUID, 所以节点端密码/连接不受影响, 用户只需重新导入订阅。
// 返回当前 token. requestID (AdminUserResetRequestID) applies a retried
// reset once, as KernelSubscriber.ResetCredentials does: a repeat changes
// nothing and returns the token the first one issued, if it is still the
// current one.
func (s *UserService) ResetToken(id uint, requestID string) (string, error) {
	if err := s.ensureUserExists(id); err != nil {
		return "", err
	}
	var tokens []string
	var revocation *authn.Revocation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		if _, revocation, err = UpdateUserOnceTx(tx, requestID, SubscriberReissueMethod, id, map[string]any{"token": uuid.New().String()}, time.Now()); err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", id).Limit(1).Pluck("token", &tokens).Error
	})
	if err != nil {
		return "", err
	}
	if revocation != nil {
		authn.Remember(*revocation)
	}
	if len(tokens) == 0 {
		return "", ErrUserNotFound
	}
	return tokens[0], nil
}

// SubscriberReissueMethod is the subscriber request ledger's method for a
// new subscription token or proxy uuid (KernelSubscriber.ResetCredentials
// and the administrator's subscription reset).
const SubscriberReissueMethod = "reset_credentials"

// Administrator resets of one user, for AdminUserResetRequestID.
const (
	AdminUserResetTraffic   = "reset_traffic"
	AdminUserResetSubscribe = "reset_subscribe"
)

// AdminUserResetRequestID names an administrator's traffic or subscription
// reset of a user in the subscriber request ledger, so a retried request is
// applied once. token identifies the HTTP request: its Idempotency-Key, else
// its request id. The identity package's native handlers derive the same id
// (packages/identity-platform/native), so a retry is recognized whichever
// side serves it; a new request is a new reset, as in v2.
func AdminUserResetRequestID(kind string, userID uint, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("identity.%s:%d:%x", kind, userID, sum[:12])
}

// ForwardUserResetRequestID names an administrator's subscriber traffic
// reset through the Flux-style POST /api/v2/user/reset (type 1) in the
// subscriber request ledger, so a retried request is applied once. token
// identifies the HTTP request, as for AdminUserResetRequestID. The forward
// package's native handler derives the same id (packages/forward/native),
// so a retry is recognized whichever side serves it.
func ForwardUserResetRequestID(userID uint, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("forward.reset_traffic:%d:%x", userID, sum[:12])
}

// UpdateUserOnceTx applies updates to an existing user through UpdateUserTx
// once per request id, recording it in the subscriber request ledger: a
// repeated id changes nothing and reports applied false. A user that does
// not exist is ErrUserNotFound, and nothing is recorded. The revocation, if
// any, is for authn.Remember after the commit. An empty request id applies
// every time.
func UpdateUserOnceTx(tx *gorm.DB, requestID, method string, userID uint, updates map[string]any, now time.Time) (bool, *authn.Revocation, error) {
	var previous struct{}
	seen, err := subscriber.Replay(tx, requestID, &previous)
	if err != nil || seen {
		return false, nil, err
	}
	var found []uint
	if err := tx.Model(&model.User{}).Where("id = ?", userID).Limit(1).Pluck("id", &found).Error; err != nil {
		return false, nil, err
	}
	if len(found) == 0 {
		return false, nil, ErrUserNotFound
	}
	revocation, err := UpdateUserTx(tx, userID, updates)
	if err != nil {
		return false, nil, err
	}
	return true, revocation, subscriber.Record(tx, requestID, method, userID, struct{}{}, now)
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
