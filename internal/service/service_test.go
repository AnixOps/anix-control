package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ServiceTestSuite 服务测试基类
type ServiceTestSuite struct {
	suite.Suite
	cfg *config.Config
}

func (s *ServiceTestSuite) SetupSuite() {
	// 初始化缓存
	cache.InitMemory()

	// 初始化数据库
	database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})

	// 自动迁移所有模型
	database.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.Order{},
		&model.Node{},
		&model.NodeProtocol{},
		&model.AuthorizedKey{},
		&model.Event{},
		&model.UserSubscriptionGroup{},
		&model.PlanSubscriptionGroup{},
	)

	// 测试配置
	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 86400,
		},
	}
}

func (s *ServiceTestSuite) TearDownSuite() {
	database.Close()
}

func (s *ServiceTestSuite) SetupTest() {
	// 清理数据
	db := database.Get()
	db.Exec("DELETE FROM v2_user")
	db.Exec("DELETE FROM v2_plan")
	db.Exec("DELETE FROM v2_order")
	db.Exec("DELETE FROM v2_node")
	db.Exec("DELETE FROM v2_node_protocol")
	db.Exec("DELETE FROM v2_authorized_key")
}

// AuthServiceTestSuite 认证服务测试套件
type AuthServiceTestSuite struct {
	ServiceTestSuite
}

func (s *AuthServiceTestSuite) TestRegister_Success() {
	svc := NewAuthService()
	token, user, err := svc.Register("test@example.com", "password123", s.cfg)

	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), token)
	assert.NotNil(s.T(), user)
	assert.Equal(s.T(), "test@example.com", user.Email)
}

func (s *AuthServiceTestSuite) TestRegister_DuplicateEmail() {
	svc := NewAuthService()

	// 第一次注册
	_, _, err := svc.Register("dup@example.com", "password123", s.cfg)
	assert.NoError(s.T(), err)

	// 第二次注册相同邮箱
	_, _, err = svc.Register("dup@example.com", "password456", s.cfg)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "已被注册")
}

func (s *AuthServiceTestSuite) TestLogin_Success() {
	svc := NewAuthService()

	// 先注册
	_, _, err := svc.Register("login@example.com", "password123", s.cfg)
	assert.NoError(s.T(), err)

	// 登录
	token, user, err := svc.Login("login@example.com", "password123", s.cfg)

	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), token)
	assert.NotNil(s.T(), user)
}

func (s *AuthServiceTestSuite) TestLogin_WrongPassword() {
	svc := NewAuthService()

	// 先注册
	_, _, err := svc.Register("wrongpass@example.com", "password123", s.cfg)
	assert.NoError(s.T(), err)

	// 使用错误密码登录
	_, _, err = svc.Login("wrongpass@example.com", "wrongpassword", s.cfg)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "密码错误")
}

func (s *AuthServiceTestSuite) TestLogin_UserNotFound() {
	svc := NewAuthService()

	_, _, err := svc.Login("nonexistent@example.com", "password123", s.cfg)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "用户不存在")
}

func (s *AuthServiceTestSuite) TestLogin_BannedUser() {
	svc := NewAuthService()

	// 注册用户
	_, user, _ := svc.Register("banned@example.com", "password123", s.cfg)

	// 封禁用户
	database.Get().Model(user).Update("banned", 1)

	// 尝试登录
	_, _, err := svc.Login("banned@example.com", "password123", s.cfg)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "被封禁")
}

func TestAuthService(t *testing.T) {
	suite.Run(t, new(AuthServiceTestSuite))
}

// NodeServiceTestSuite 节点服务测试套件
type NodeServiceTestSuite struct {
	ServiceTestSuite
	svc *NodeService
}

func (s *NodeServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.svc = NewNodeService()
}

func (s *NodeServiceTestSuite) TestCreateNode() {
	node := &model.Node{
		Name: "Test Node",
		Host: "192.168.1.1",
		Port: 443,
		Rate: 1.0,
		Show: 1,
	}

	err := s.svc.CreateNode(node)
	assert.NoError(s.T(), err)
	assert.NotZero(s.T(), node.ID)
	assert.NotEmpty(s.T(), node.APIKey)
	assert.NotEmpty(s.T(), node.Secret)
	assert.Equal(s.T(), model.NodeStatusPending, node.Status)
}

func (s *NodeServiceTestSuite) TestGetNode() {
	// 创建节点
	node := &model.Node{
		Name: "Get Test Node",
		Host: "192.168.1.2",
		Port: 443,
		Rate: 1.0,
		Show: 1,
	}
	s.svc.CreateNode(node)

	// 获取节点
	found, err := s.svc.GetNode(node.ID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), node.Name, found.Name)
}

func (s *NodeServiceTestSuite) TestGetNode_NotFound() {
	_, err := s.svc.GetNode(99999)
	assert.Error(s.T(), err)
}

func (s *NodeServiceTestSuite) TestUpdateNode() {
	// 创建节点
	node := &model.Node{
		Name: "Update Test Node",
		Host: "192.168.1.3",
		Port: 443,
		Rate: 1.0,
		Show: 1,
	}
	s.svc.CreateNode(node)

	// 更新节点
	updates := map[string]any{
		"name": "Updated Node",
		"rate": 2.0,
	}
	err := s.svc.UpdateNode(node.ID, updates)
	assert.NoError(s.T(), err)

	// 验证更新
	found, _ := s.svc.GetNode(node.ID)
	assert.Equal(s.T(), "Updated Node", found.Name)
	assert.Equal(s.T(), 2.0, found.Rate)
}

func (s *NodeServiceTestSuite) TestDeleteNode() {
	// 创建节点
	node := &model.Node{
		Name: "Delete Test Node",
		Host: "192.168.1.4",
		Port: 443,
		Rate: 1.0,
		Show: 1,
	}
	s.svc.CreateNode(node)

	// 删除节点
	err := s.svc.DeleteNode(node.ID)
	assert.NoError(s.T(), err)

	// 验证删除
	_, err = s.svc.GetNode(node.ID)
	assert.Error(s.T(), err)
}

func (s *NodeServiceTestSuite) TestGetNodes() {
	// 创建多个节点
	for i := 1; i <= 3; i++ {
		node := &model.Node{
			Name: "List Test Node",
			Host: "192.168.1.10",
			Port: 443,
			Rate: 1.0,
			Show: 1,
		}
		s.svc.CreateNode(node)
	}

	// 获取列表
	params := NodeListParams{
		Page:     1,
		PageSize: 10,
	}
	result, err := s.svc.GetNodes(params)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), result.Total)
	assert.Len(s.T(), result.List, 3)
}

func (s *NodeServiceTestSuite) TestGenerateAuthKey() {
	key, rawKey, err := s.svc.GenerateAuthKey("Test Key", 7)

	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), key)
	assert.NotEmpty(s.T(), rawKey)
	assert.Equal(s.T(), "Test Key", key.Name)
	assert.NotNil(s.T(), key.ExpireAt)
}

func (s *NodeServiceTestSuite) TestGenerateAuthKey_NoExpiry() {
	key, rawKey, err := s.svc.GenerateAuthKey("No Expiry Key", 0)

	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), key)
	assert.NotEmpty(s.T(), rawKey)
	assert.Nil(s.T(), key.ExpireAt)
}

func (s *NodeServiceTestSuite) TestProtocolCRUD() {
	// 创建节点
	node := &model.Node{
		Name: "Protocol Test Node",
		Host: "192.168.1.5",
		Port: 443,
		Rate: 1.0,
		Show: 1,
	}
	s.svc.CreateNode(node)

	// 创建协议
	transport := "tcp"
	protocol := &model.NodeProtocol{
		NodeID:    node.ID,
		Name:      "Test VLESS",
		Type:      model.ProtocolVLESS,
		Port:      443,
		Enable:    1,
		Show:      1,
		Transport: &transport,
	}
	err := s.svc.CreateProtocol(protocol)
	assert.NoError(s.T(), err)

	// 获取协议
	protocols, err := s.svc.GetProtocols(node.ID)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), protocols, 1)

	// 更新协议
	updates := map[string]any{
		"name": "Updated VLESS",
	}
	err = s.svc.UpdateProtocol(protocol.ID, updates)
	assert.NoError(s.T(), err)

	// 删除协议
	err = s.svc.DeleteProtocol(protocol.ID)
	assert.NoError(s.T(), err)
}

func TestNodeService(t *testing.T) {
	suite.Run(t, new(NodeServiceTestSuite))
}

// UserServiceTestSuite 用户服务测试套件
type UserServiceTestSuite struct {
	ServiceTestSuite
	svc *UserService
}

func (s *UserServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.svc = NewUserService()
}

func (s *UserServiceTestSuite) TestGetByID() {
	// 创建用户
	user := &model.User{
		Email:          "getuser@example.com",
		Password:       "hash",
		Token:          "token123",
		UUID:           "uuid123",
		TransferEnable: 10737418240,
	}
	database.Get().Create(user)

	// 获取用户
	found, err := s.svc.GetByID(user.ID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), user.Email, found.Email)
}

func (s *UserServiceTestSuite) TestGetByUUID() {
	user := &model.User{
		Email:          "uuiduser@example.com",
		Password:       "hash",
		Token:          "token456",
		UUID:           "uuid456",
		TransferEnable: 10737418240,
	}
	database.Get().Create(user)

	found, err := s.svc.GetByUUID("uuid456")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), user.Email, found.Email)
}

func (s *UserServiceTestSuite) TestGetByToken() {
	user := &model.User{
		Email:          "tokenuser@example.com",
		Password:       "hash",
		Token:          "token789",
		UUID:           "uuid789",
		TransferEnable: 10737418240,
	}
	database.Get().Create(user)

	found, err := s.svc.GetByToken("token789")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), user.Email, found.Email)
}

func (s *UserServiceTestSuite) TestUpdateTraffic() {
	user := &model.User{
		Email:          "traffic@example.com",
		Password:       "hash",
		Token:          "token-traffic",
		UUID:           "uuid-traffic",
		TransferEnable: 10737418240,
		U:              0,
		D:              0,
	}
	database.Get().Create(user)

	// 更新流量
	err := s.svc.UpdateTraffic(user.ID, 1024, 2048)
	assert.NoError(s.T(), err)

	// 验证
	found, _ := s.svc.GetByID(user.ID)
	assert.Equal(s.T(), int64(1024), found.U)
	assert.Equal(s.T(), int64(2048), found.D)
}

func (s *UserServiceTestSuite) TestBatchUpdateTraffic() {
	// 创建多个用户
	users := []*model.User{
		{Email: "batch1@example.com", Password: "hash", Token: "batch1", UUID: "batch1", TransferEnable: 10737418240},
		{Email: "batch2@example.com", Password: "hash", Token: "batch2", UUID: "batch2", TransferEnable: 10737418240},
	}
	for _, u := range users {
		database.Get().Create(u)
	}

	// 批量更新流量
	traffics := map[uint][2]int64{
		users[0].ID: {1024, 2048},
		users[1].ID: {2048, 4096},
	}
	err := s.svc.BatchUpdateTraffic(traffics)
	assert.NoError(s.T(), err)

	// 验证
	found1, _ := s.svc.GetByID(users[0].ID)
	assert.Equal(s.T(), int64(1024), found1.U)

	found2, _ := s.svc.GetByID(users[1].ID)
	assert.Equal(s.T(), int64(2048), found2.U)
}

func (s *UserServiceTestSuite) TestBanUnban() {
	user := &model.User{
		Email:          "ban@example.com",
		Password:       "hash",
		Token:          "ban-token",
		UUID:           "ban-uuid",
		TransferEnable: 10737418240,
	}
	database.Get().Create(user)

	// 封禁
	err := s.svc.Ban(user.ID)
	assert.NoError(s.T(), err)

	found, _ := s.svc.GetByID(user.ID)
	assert.Equal(s.T(), 1, found.Banned)

	// 解封
	err = s.svc.Unban(user.ID)
	assert.NoError(s.T(), err)

	found, _ = s.svc.GetByID(user.ID)
	assert.Equal(s.T(), 0, found.Banned)
}

func (s *UserServiceTestSuite) TestResetTraffic() {
	user := &model.User{
		Email:          "reset@example.com",
		Password:       "hash",
		Token:          "reset-token",
		UUID:           "reset-uuid",
		TransferEnable: 10737418240,
		U:              1000,
		D:              2000,
	}
	database.Get().Create(user)

	// 重置流量
	err := s.svc.ResetTraffic(user.ID)
	assert.NoError(s.T(), err)

	found, _ := s.svc.GetByID(user.ID)
	assert.Equal(s.T(), int64(0), found.U)
	assert.Equal(s.T(), int64(0), found.D)
}

func TestUserService(t *testing.T) {
	suite.Run(t, new(UserServiceTestSuite))
}

// PlanServiceTestSuite 套餐服务测试套件
type PlanServiceTestSuite struct {
	ServiceTestSuite
	svc *PlanService
}

func (s *PlanServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.svc = NewPlanService()
}

func (s *PlanServiceTestSuite) TestCreatePlan() {
	groupID := uint(1)
	speedLimit := int64(100000000)
	deviceLimit := 5
	monthPrice := int64(1000)

	plan := &model.Plan{
		Name:             "Test Plan",
		GroupID:          groupID,
		TransferEnable:   100, // 100GB
		SpeedLimit:       &speedLimit,
		DeviceLimit:      &deviceLimit,
		MonthPrice:       &monthPrice,
		Show:             1,
	}

	err := s.svc.Create(plan)
	assert.NoError(s.T(), err)
	assert.NotZero(s.T(), plan.ID)
}

func (s *PlanServiceTestSuite) TestGetPlan() {
	monthPrice := int64(2000)
	plan := &model.Plan{
		Name:           "Get Test Plan",
		GroupID:        1,
		TransferEnable: 50,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.svc.Create(plan)

	found, err := s.svc.Get(plan.ID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "Get Test Plan", found.Name)
	assert.Equal(s.T(), int64(2000), *found.MonthPrice)
}

func (s *PlanServiceTestSuite) TestGetPlan_NotFound() {
	_, err := s.svc.Get(99999)
	assert.Error(s.T(), err)
}

func (s *PlanServiceTestSuite) TestUpdatePlan() {
	monthPrice := int64(3000)
	plan := &model.Plan{
		Name:           "Update Test Plan",
		GroupID:        1,
		TransferEnable: 50,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.svc.Create(plan)

	plan.Name = "Updated Plan"
	plan.TransferEnable = 100
	err := s.svc.Update(plan)
	assert.NoError(s.T(), err)

	found, _ := s.svc.Get(plan.ID)
	assert.Equal(s.T(), "Updated Plan", found.Name)
	assert.Equal(s.T(), int64(100), found.TransferEnable)
}

func (s *PlanServiceTestSuite) TestDeletePlan() {
	monthPrice := int64(4000)
	plan := &model.Plan{
		Name:           "Delete Test Plan",
		GroupID:        1,
		TransferEnable: 50,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.svc.Create(plan)

	err := s.svc.Delete(plan.ID)
	assert.NoError(s.T(), err)

	_, err = s.svc.Get(plan.ID)
	assert.Error(s.T(), err)
}

func (s *PlanServiceTestSuite) TestListPlans() {
	monthPrice := int64(5000)
	for i := 1; i <= 3; i++ {
		plan := &model.Plan{
			Name:           "List Test Plan",
			GroupID:        1,
			TransferEnable: int64(i * 50),
			MonthPrice:     &monthPrice,
			Show:           1,
		}
		s.svc.Create(plan)
	}

	list, err := s.svc.List()
	assert.NoError(s.T(), err)
	assert.GreaterOrEqual(s.T(), len(list), 3)
}

func (s *PlanServiceTestSuite) TestAssignToUser() {
	// 创建套餐
	speedLimit := int64(100000000)
	deviceLimit := 5
	monthPrice := int64(1000)
	groupID := uint(1)
	plan := &model.Plan{
		Name:           "Assign Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		SpeedLimit:     &speedLimit,
		DeviceLimit:    &deviceLimit,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.svc.Create(plan)

	// 创建用户
	user := &model.User{
		Email:          "planuser@example.com",
		Password:       "hash",
		Token:          "plan-token",
		UUID:           "plan-uuid",
		TransferEnable: 10737418240,
	}
	database.Get().Create(user)

	// 分配套餐
	expireAt := time.Now().Add(30 * 24 * time.Hour).Unix()
	err := s.svc.AssignToUser(plan.ID, user.ID, &expireAt)
	assert.NoError(s.T(), err)

	// 验证用户更新
	var updatedUser model.User
	database.Get().First(&updatedUser, user.ID)
	assert.Equal(s.T(), plan.ID, *updatedUser.PlanID)
	assert.Equal(s.T(), int64(100*1073741824), updatedUser.TransferEnable)
}

func TestPlanService(t *testing.T) {
	suite.Run(t, new(PlanServiceTestSuite))
}

// OrderServiceTestSuite 订单服务测试套件
type OrderServiceTestSuite struct {
	ServiceTestSuite
	svc       *OrderService
	planSvc   *PlanService
	authSvc   *AuthService
	testUser  *model.User
	testPlan  *model.Plan
}

func (s *OrderServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.svc = NewOrderService()
	s.planSvc = NewPlanService()
	s.authSvc = NewAuthService()

	// 创建测试用户
	_, user, _ := s.authSvc.Register("order@example.com", "password123", s.cfg)
	s.testUser = user

	// 创建测试套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	quarterPrice := int64(2700)
	yearPrice := int64(10000)
	onetimePrice := int64(5000)
	plan := &model.Plan{
		Name:           "Order Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		QuarterPrice:   &quarterPrice,
		YearPrice:      &yearPrice,
		OnetimePrice:   &onetimePrice,
		Show:           1,
	}
	s.planSvc.Create(plan)
	s.testPlan = plan
}

func (s *OrderServiceTestSuite) TestCreateOrder_Monthly() {
	order, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), order)
	assert.NotEmpty(s.T(), order.TradeNo)
	assert.Equal(s.T(), int64(1000), order.TotalAmount)
	assert.Equal(s.T(), 1, order.Type) // 新购
	assert.Equal(s.T(), 0, order.Status)
}

func (s *OrderServiceTestSuite) TestCreateOrder_Quarterly() {
	order, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "quarter",
	})

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2700), order.TotalAmount)
}

func (s *OrderServiceTestSuite) TestCreateOrder_Yearly() {
	order, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "year",
	})

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(10000), order.TotalAmount)
}

func (s *OrderServiceTestSuite) TestCreateOrder_Onetime() {
	order, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "onetime",
	})

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(5000), order.TotalAmount)
}

func (s *OrderServiceTestSuite) TestCreateOrder_InvalidPeriod() {
	_, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "invalid_period",
	})

	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "无效的付费周期")
}

func (s *OrderServiceTestSuite) TestCreateOrder_PlanNotFound() {
	_, err := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: 99999,
		Period: "month",
	})

	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "套餐不存在")
}

func (s *OrderServiceTestSuite) TestGetOrderByID() {
	order, _ := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	found, err := s.svc.GetByID(order.ID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), order.TradeNo, found.TradeNo)
}

func (s *OrderServiceTestSuite) TestGetOrderByTradeNo() {
	order, _ := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	found, err := s.svc.GetByTradeNo(order.TradeNo)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), order.ID, found.ID)
}

func (s *OrderServiceTestSuite) TestUpdateStatus() {
	order, _ := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	err := s.svc.UpdateStatus(order.ID, 1)
	assert.NoError(s.T(), err)

	found, _ := s.svc.GetByID(order.ID)
	assert.Equal(s.T(), 1, found.Status)
	assert.NotZero(s.T(), found.PaidAt)
}

func (s *OrderServiceTestSuite) TestCancelOrder() {
	order, _ := s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	err := s.svc.Cancel(order.ID)
	assert.NoError(s.T(), err)

	found, _ := s.svc.GetByID(order.ID)
	assert.Equal(s.T(), 2, found.Status)
}

func (s *OrderServiceTestSuite) TestGetUserOrders() {
	// 创建多个订单
	for i := 0; i < 3; i++ {
		s.svc.Create(CreateOrderParams{
			UserID: s.testUser.ID,
			PlanID: s.testPlan.ID,
			Period: "month",
		})
	}

	result, err := s.svc.GetUserOrders(s.testUser.ID, 1, 10)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(3), result.Total)
	assert.Len(s.T(), result.List, 3)
}

func (s *OrderServiceTestSuite) TestGetStats() {
	// 创建几个订单
	s.svc.Create(CreateOrderParams{
		UserID: s.testUser.ID,
		PlanID: s.testPlan.ID,
		Period: "month",
	})

	stats, err := s.svc.GetStats()
	assert.NoError(s.T(), err)
	assert.GreaterOrEqual(s.T(), stats["total_orders"].(int64), int64(1))
}

func TestOrderService(t *testing.T) {
	suite.Run(t, new(OrderServiceTestSuite))
}

// StatsServiceTestSuite 统计服务测试套件
type StatsServiceTestSuite struct {
	ServiceTestSuite
	svc      *StatsService
	authSvc  *AuthService
	planSvc  *PlanService
}

func (s *StatsServiceTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	// 重置 statsServiceInstance 以便创建新实例
	statsServiceInstance = nil
	s.svc = NewStatsService()
	s.authSvc = NewAuthService()
	s.planSvc = NewPlanService()
}

func (s *StatsServiceTestSuite) TestGetDashboardStats() {
	// 创建一些用户
	for i := 0; i < 3; i++ {
		s.authSvc.Register(fmt.Sprintf("stats%d@example.com", i), "password123", s.cfg)
	}

	stats, err := s.svc.GetDashboardStats(false)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), stats)
	assert.GreaterOrEqual(s.T(), stats.TotalUsers, int64(3))
}

func (s *StatsServiceTestSuite) TestGetDashboardStats_ForceRefresh() {
	stats, err := s.svc.GetDashboardStats(true)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), stats)
	assert.False(s.T(), stats.CachedAt.IsZero())
}

func (s *StatsServiceTestSuite) TestGetUserSubscription() {
	// 创建用户
	_, user, _ := s.authSvc.Register("subuser@example.com", "password123", s.cfg)

	// 创建套餐
	groupID := uint(1)
	monthPrice := int64(1000)
	plan := &model.Plan{
		Name:           "Sub Test Plan",
		GroupID:        groupID,
		TransferEnable: 100,
		MonthPrice:     &monthPrice,
		Show:           1,
	}
	s.planSvc.Create(plan)

	// 分配套餐给用户
	expireAt := time.Now().Add(30 * 24 * time.Hour).Unix()
	s.planSvc.AssignToUser(plan.ID, user.ID, &expireAt)

	sub, err := s.svc.GetUserSubscription(user.ID, false)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), sub)
	assert.Equal(s.T(), user.Email, sub.Email)
	assert.Equal(s.T(), "Sub Test Plan", sub.PlanName)
	assert.False(s.T(), sub.IsExpired)
	assert.Greater(s.T(), sub.DaysRemaining, 0)
}

func (s *StatsServiceTestSuite) TestGetUserSubscription_Expired() {
	// 创建用户
	_, user, _ := s.authSvc.Register("expireduser@example.com", "password123", s.cfg)

	// 设置过期时间
	expiredAt := time.Now().Add(-24 * time.Hour).Unix() // 昨天过期
	database.Get().Model(user).Update("expired_at", expiredAt)

	sub, err := s.svc.GetUserSubscription(user.ID, true)
	assert.NoError(s.T(), err)
	assert.True(s.T(), sub.IsExpired)
	assert.LessOrEqual(s.T(), sub.DaysRemaining, 0)
}

func (s *StatsServiceTestSuite) TestGetUserSubscription_NoPlan() {
	// 创建无套餐用户
	_, user, _ := s.authSvc.Register("noplanuser@example.com", "password123", s.cfg)

	sub, err := s.svc.GetUserSubscription(user.ID, true)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "无套餐", sub.PlanName)
}

func (s *StatsServiceTestSuite) TestInvalidateUserCache() {
	_, user, _ := s.authSvc.Register("cacheuser@example.com", "password123", s.cfg)

	// 获取订阅（会缓存）
	s.svc.GetUserSubscription(user.ID, false)

	// 使缓存失效
	s.svc.InvalidateUserCache(user.ID)

	// 验证缓存已删除
	exists := cache.Exists(fmt.Sprintf("%s%d", CacheKeyUserSubscription, user.ID))
	assert.False(s.T(), exists)
}

func (s *StatsServiceTestSuite) TestRefreshDashboardCache() {
	err := s.svc.RefreshDashboardCache()
	assert.NoError(s.T(), err)

	// 验证缓存存在
	exists := cache.Exists(CacheKeyDashboardStats)
	assert.True(s.T(), exists)
}

func TestStatsService(t *testing.T) {
	suite.Run(t, new(StatsServiceTestSuite))
}