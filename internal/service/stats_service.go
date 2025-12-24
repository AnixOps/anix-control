package service

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// 缓存键
const (
	CacheKeyDashboardStats   = "stats:dashboard"
	CacheKeyUserStats        = "stats:users"
	CacheKeyOrderStats       = "stats:orders"
	CacheKeyNodeStats        = "stats:nodes"
	CacheKeyUserSubscription = "user:subscription:" // + userID
)

// 缓存过期时间
const (
	StatsCacheTTL        = 60 * time.Second // 统计数据缓存 60 秒
	SubscriptionCacheTTL = 30 * time.Second // 用户订阅缓存 30 秒
	NodeCacheTTL         = 10 * time.Second // 节点缓存 10 秒
)

// StatsService 统计服务 (带缓存)
type StatsService struct {
	db           *gorm.DB
	userService  *UserService
	orderService *OrderService
	mu           sync.RWMutex
}

// DashboardStats 仪表盘统计数据
type DashboardStats struct {
	// 用户统计
	TotalUsers    int64 `json:"total_users"`
	ActiveUsers   int64 `json:"active_users"`
	ExpiredUsers  int64 `json:"expired_users"`
	BannedUsers   int64 `json:"banned_users"`
	TodayNewUsers int64 `json:"today_new_users"`

	// 订单统计
	TotalOrders   int64 `json:"total_orders"`
	PendingOrders int64 `json:"pending_orders"`
	PaidOrders    int64 `json:"paid_orders"`
	TotalRevenue  int64 `json:"total_revenue"`  // 总收入 (分)
	MonthlyIncome int64 `json:"monthly_income"` // 月收入 (分)
	TodayIncome   int64 `json:"today_income"`   // 今日收入 (分)

	// 节点统计
	TotalNodes  int64 `json:"total_nodes"`
	ActiveNodes int64 `json:"active_nodes"`
	OnlineUsers int64 `json:"online_users"`

	// 流量统计
	TotalTrafficUsed int64 `json:"total_traffic_used"` // 总已用流量 (字节)
	TodayTraffic     int64 `json:"today_traffic"`      // 今日流量 (字节)

	// 缓存信息
	CachedAt time.Time `json:"cached_at"`
}

// UserSubscription 用户订阅详情
type UserSubscription struct {
	UserID          uint      `json:"user_id"`
	Email           string    `json:"email"`
	PlanID          *uint     `json:"plan_id"`
	PlanName        string    `json:"plan_name"`
	TransferEnable  int64     `json:"transfer_enable"`  // 总流量 (字节)
	UsedTraffic     int64     `json:"used_traffic"`     // 已用流量 (字节)
	UploadTraffic   int64     `json:"upload_traffic"`   // 上传流量 (字节)
	DownloadTraffic int64     `json:"download_traffic"` // 下载流量 (字节)
	ExpiredAt       int64     `json:"expired_at"`       // 到期时间
	IsExpired       bool      `json:"is_expired"`
	DaysRemaining   int       `json:"days_remaining"`
	UsagePercent    float64   `json:"usage_percent"`
	CachedAt        time.Time `json:"cached_at"`
}

var statsServiceInstance *StatsService

// NewStatsService 创建统计服务
func NewStatsService() *StatsService {
	if statsServiceInstance == nil {
		statsServiceInstance = &StatsService{
			db:           database.GetDB(),
			userService:  NewUserService(),
			orderService: NewOrderService(),
		}
	}
	return statsServiceInstance
}

// GetDashboardStats 获取仪表盘统计 (优先从缓存读取)
func (s *StatsService) GetDashboardStats(forceRefresh bool) (*DashboardStats, error) {
	// 尝试从缓存获取
	if !forceRefresh {
		if cached, err := s.getDashboardFromCache(); err == nil {
			return cached, nil
		}
	}

	// 缓存未命中，从数据库查询
	stats, err := s.fetchDashboardFromDB()
	if err != nil {
		return nil, err
	}

	// 存入缓存
	s.saveDashboardToCache(stats)

	return stats, nil
}

// GetUserSubscription 获取用户订阅详情 (优先从缓存读取)
func (s *StatsService) GetUserSubscription(userID uint, forceRefresh bool) (*UserSubscription, error) {
	cacheKey := CacheKeyUserSubscription + string(rune(userID))

	// 尝试从缓存获取
	if !forceRefresh {
		if cached, err := s.getSubscriptionFromCache(cacheKey); err == nil {
			return cached, nil
		}
	}

	// 缓存未命中，从数据库查询
	sub, err := s.fetchSubscriptionFromDB(userID)
	if err != nil {
		return nil, err
	}

	// 存入缓存
	s.saveSubscriptionToCache(cacheKey, sub)

	return sub, nil
}

// RefreshDashboardCache 强制刷新仪表盘缓存 (后台任务用)
func (s *StatsService) RefreshDashboardCache() error {
	stats, err := s.fetchDashboardFromDB()
	if err != nil {
		return err
	}
	s.saveDashboardToCache(stats)
	return nil
}

// InvalidateUserCache 使用户缓存失效 (用户数据变更时调用)
func (s *StatsService) InvalidateUserCache(userID uint) {
	cacheKey := CacheKeyUserSubscription + string(rune(userID))
	cache.Delete(cacheKey)
}

// ========== 私有方法 ==========

// 从缓存获取仪表盘数据
func (s *StatsService) getDashboardFromCache() (*DashboardStats, error) {
	data, err := cache.Get(CacheKeyDashboardStats)
	if err != nil {
		return nil, err
	}

	if stats, ok := data.(*DashboardStats); ok {
		return stats, nil
	}

	// 如果是 JSON 字符串 (Redis 场景)
	if str, ok := data.(string); ok {
		var stats DashboardStats
		if err := json.Unmarshal([]byte(str), &stats); err != nil {
			return nil, err
		}
		return &stats, nil
	}

	return nil, cache.ErrKeyNotFound
}

// 保存仪表盘数据到缓存
func (s *StatsService) saveDashboardToCache(stats *DashboardStats) {
	cache.Set(CacheKeyDashboardStats, stats, StatsCacheTTL)
}

// 从缓存获取订阅数据
func (s *StatsService) getSubscriptionFromCache(key string) (*UserSubscription, error) {
	data, err := cache.Get(key)
	if err != nil {
		return nil, err
	}

	if sub, ok := data.(*UserSubscription); ok {
		return sub, nil
	}

	if str, ok := data.(string); ok {
		var sub UserSubscription
		if err := json.Unmarshal([]byte(str), &sub); err != nil {
			return nil, err
		}
		return &sub, nil
	}

	return nil, cache.ErrKeyNotFound
}

// 保存订阅数据到缓存
func (s *StatsService) saveSubscriptionToCache(key string, sub *UserSubscription) {
	cache.Set(key, sub, SubscriptionCacheTTL)
}

// 从数据库获取仪表盘统计
func (s *StatsService) fetchDashboardFromDB() (*DashboardStats, error) {
	stats := &DashboardStats{
		CachedAt: time.Now(),
	}

	now := time.Now().Unix()
	todayStart := time.Now().Truncate(24 * time.Hour)
	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)

	// 用户统计 (批量查询优化)
	s.db.Model(&model.User{}).Count(&stats.TotalUsers)
	s.db.Model(&model.User{}).
		Where("banned = 0 AND (expired_at IS NULL OR expired_at > ?)", now).
		Count(&stats.ActiveUsers)
	s.db.Model(&model.User{}).
		Where("expired_at IS NOT NULL AND expired_at <= ?", now).
		Count(&stats.ExpiredUsers)
	s.db.Model(&model.User{}).Where("banned = 1").Count(&stats.BannedUsers)
	s.db.Model(&model.User{}).Where("created_at >= ?", todayStart).Count(&stats.TodayNewUsers)

	// 订单统计
	s.db.Model(&model.Order{}).Count(&stats.TotalOrders)
	s.db.Model(&model.Order{}).Where("status = 0").Count(&stats.PendingOrders)
	s.db.Model(&model.Order{}).Where("status IN (1, 3)").Count(&stats.PaidOrders)
	s.db.Model(&model.Order{}).
		Where("status IN (1, 3)").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.TotalRevenue)
	s.db.Model(&model.Order{}).
		Where("status IN (1, 3) AND paid_at >= ?", monthStart.Unix()).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.MonthlyIncome)
	s.db.Model(&model.Order{}).
		Where("status IN (1, 3) AND paid_at >= ?", todayStart.Unix()).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&stats.TodayIncome)

	// 节点统计
	var vmessCount, vlessCount, trojanCount, ssCount int64
	s.db.Model(&model.ServerVMess{}).Where("show = 1").Count(&vmessCount)
	s.db.Model(&model.ServerVLESS{}).Where("show = 1").Count(&vlessCount)
	s.db.Model(&model.ServerTrojan{}).Where("show = 1").Count(&trojanCount)
	s.db.Model(&model.ServerShadowsocks{}).Where("show = 1").Count(&ssCount)
	stats.TotalNodes = vmessCount + vlessCount + trojanCount + ssCount

	// 活跃节点 (最近5分钟有心跳)
	fiveMinAgo := now - 300
	var activeVmess, activeVless, activeTrojan, activeSS int64
	s.db.Model(&model.ServerVMess{}).Where("show = 1 AND last_check_at > ?", fiveMinAgo).Count(&activeVmess)
	s.db.Model(&model.ServerVLESS{}).Where("show = 1 AND last_check_at > ?", fiveMinAgo).Count(&activeVless)
	s.db.Model(&model.ServerTrojan{}).Where("show = 1 AND last_check_at > ?", fiveMinAgo).Count(&activeTrojan)
	s.db.Model(&model.ServerShadowsocks{}).Where("show = 1 AND last_check_at > ?", fiveMinAgo).Count(&activeSS)
	stats.ActiveNodes = activeVmess + activeVless + activeTrojan + activeSS

	// 在线用户数 (从缓存的 alive list 获取)
	stats.OnlineUsers = s.getOnlineUsersCount()

	// 流量统计
	s.db.Model(&model.User{}).
		Select("COALESCE(SUM(u + d), 0)").
		Scan(&stats.TotalTrafficUsed)

	return stats, nil
}

// 从数据库获取用户订阅
func (s *StatsService) fetchSubscriptionFromDB(userID uint) (*UserSubscription, error) {
	var user model.User
	if err := s.db.Preload("Plan").First(&user, userID).Error; err != nil {
		return nil, err
	}

	now := time.Now().Unix()

	// 处理 ExpiredAt 指针
	var expiredAt int64 = 0
	if user.ExpiredAt != nil {
		expiredAt = *user.ExpiredAt
	}

	sub := &UserSubscription{
		UserID:          user.ID,
		Email:           user.Email,
		PlanID:          user.PlanID,
		TransferEnable:  user.TransferEnable,
		UsedTraffic:     user.U + user.D,
		UploadTraffic:   user.U,
		DownloadTraffic: user.D,
		ExpiredAt:       expiredAt,
		CachedAt:        time.Now(),
	}

	// 套餐名称
	if user.Plan != nil {
		sub.PlanName = user.Plan.Name
	} else {
		sub.PlanName = "无套餐"
	}

	// 是否过期
	if expiredAt > 0 {
		sub.IsExpired = expiredAt <= now
		if !sub.IsExpired {
			sub.DaysRemaining = int((expiredAt - now) / 86400)
		}
	} else {
		sub.IsExpired = false
		sub.DaysRemaining = -1 // 永久
	}

	// 使用百分比
	if user.TransferEnable > 0 {
		sub.UsagePercent = float64(sub.UsedTraffic) / float64(user.TransferEnable) * 100
	}

	return sub, nil
}

// 获取在线用户数
func (s *StatsService) getOnlineUsersCount() int64 {
	// 尝试从缓存的 alive list 获取
	count, err := cache.SCard("alive:users")
	if err != nil {
		return 0
	}
	return count
}
