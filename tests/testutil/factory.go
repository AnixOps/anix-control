package testutil

import (
	"time"

	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// MockFactory 模拟数据工厂
type MockFactory struct{}

var Factory = &MockFactory{}

// CreateUser 创建测试用户
func (f *MockFactory) CreateUser(overrides ...func(*model.User)) *model.User {
	user := &model.User{
		Email:          "test@example.com",
		Password:       f.HashPassword("password123"),
		Token:          uuid.New().String(),
		UUID:           uuid.New().String(),
		Balance:        0,
		TransferEnable: 10737418240, // 10GB
		U:              0,
		D:              0,
		Banned:         0,
		IsAdmin:        0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	for _, override := range overrides {
		override(user)
	}

	return user
}

// CreateAdminUser 创建管理员用户
func (f *MockFactory) CreateAdminUser(overrides ...func(*model.User)) *model.User {
	return f.CreateUser(func(u *model.User) {
		u.IsAdmin = 1
		u.Email = "admin@example.com"
		for _, override := range overrides {
			override(u)
		}
	})
}

// CreatePlan 创建测试套餐
func (f *MockFactory) CreatePlan(overrides ...func(*model.Plan)) *model.Plan {
	content := "Test plan description"
	speedLimit := int64(104857600) // 100MB/s
	deviceLimit := 5
	sort := 0

	plan := &model.Plan{
		Name:           "Test Plan",
		Content:        &content,
		GroupID:        0,
		SpeedLimit:     &speedLimit,
		DeviceLimit:    &deviceLimit,
		TransferEnable: 107374182400, // 100GB
		Sort:           &sort,
		Show:           1,
		Renew:          1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	for _, override := range overrides {
		override(plan)
	}

	return plan
}

// CreateNode 创建测试节点
func (f *MockFactory) CreateNode(overrides ...func(*model.Node)) *model.Node {
	node := &model.Node{
		Name:         "Test Node",
		Host:         "127.0.0.1",
		Port:         443,
		Status:       model.NodeStatusOnline,
		Rate:         1.0,
		TrafficRate:  1.0,
		Show:         1,
		Sort:         0,
		AutoRegister: 0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	for _, override := range overrides {
		override(node)
	}

	return node
}

// CreateNodeProtocol 创建节点协议配置
func (f *MockFactory) CreateNodeProtocol(nodeID uint, overrides ...func(*model.NodeProtocol)) *model.NodeProtocol {
	protocol := &model.NodeProtocol{
		NodeID:    nodeID,
		Name:      "VLESS + Reality",
		Type:      model.ProtocolVLESS,
		Port:      443,
		Enable:    1,
		Show:      1,
		Sort:      0,
		TLS:       2, // Reality
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for _, override := range overrides {
		override(protocol)
	}

	return protocol
}

// CreateOrder 创建测试订单
func (f *MockFactory) CreateOrder(userID, planID uint, overrides ...func(*model.Order)) *model.Order {
	order := &model.Order{
		TradeNo:     uuid.New().String()[:16],
		UserID:      userID,
		PlanID:      planID,
		Type:        1, // 新购
		Period:      "month",
		TotalAmount: 1000, // 10.00 元 (单位分)
		Status:      0,    // pending
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	for _, override := range overrides {
		override(order)
	}

	return order
}

// CreateSubscriptionGroup 创建订阅分组
func (f *MockFactory) CreateSubscriptionGroup(overrides ...func(*model.SubscriptionGroup)) *model.SubscriptionGroup {
	description := "Test subscription group"
	group := &model.SubscriptionGroup{
		Name:        "Test Group",
		Description: &description,
		Priority:    0,
		Enable:      1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	for _, override := range overrides {
		override(group)
	}

	return group
}

// CreateCoupon 创建优惠券
func (f *MockFactory) CreateCoupon(overrides ...func(*model.Coupon)) *model.Coupon {
	limitUse := 100
	coupon := &model.Coupon{
		Code:      "TEST1234",
		Name:      "Test Coupon",
		Type:      1, // 金额
		Value:     1000, // 10.00 元 (单位分)
		LimitUse:  &limitUse,
		UseCount:  0,
		StartedAt: time.Now().Unix(),
		EndedAt:   time.Now().AddDate(0, 1, 0).Unix(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for _, override := range overrides {
		override(coupon)
	}

	return coupon
}

// CreateAuthorizedKey 创建授权密钥
func (f *MockFactory) CreateAuthorizedKey(overrides ...func(*model.AuthorizedKey)) *model.AuthorizedKey {
	key := &model.AuthorizedKey{
		Name:      "Test Key",
		Key:       uuid.New().String()[:32],
		KeyHash:   uuid.New().String()[:32],
		Used:      0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for _, override := range overrides {
		override(key)
	}

	return key
}

// HashPassword 哈希密码
func (f *MockFactory) HashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed)
}

// GenerateJWTToken 生成测试 JWT Token
func (f *MockFactory) GenerateJWTToken(userID uint, isAdmin bool) string {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"is_admin": isAdmin,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-jwt-secret-key-for-testing"))
	return tokenString
}

// SetupTestRouter 创建测试路由
func SetupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}