package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// 缓存键
const (
	CacheKeyNodeList      = "nodes:list"
	CacheKeyNode          = "node:"           // + nodeID
	CacheKeyNodeProtocols = "node:protocols:" // + nodeID
)

// NodeService 节点服务
type NodeService struct {
	db *gorm.DB
}

// NewNodeService 创建节点服务
func NewNodeService() *NodeService {
	return &NodeService{
		db: database.GetDB(),
	}
}

// ========== 节点管理 ==========

// GetNodes 获取节点列表
func (s *NodeService) GetNodes(params NodeListParams) (*NodeListResult, error) {
	var nodes []model.Node
	var total int64

	query := s.db.Model(&model.Node{})

	// 筛选条件
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	if params.GroupID != nil {
		query = query.Where("group_id = ?", *params.GroupID)
	}
	if params.Search != "" {
		query = query.Where("name LIKE ? OR host LIKE ?", "%"+params.Search+"%", "%"+params.Search+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页
	offset := (params.Page - 1) * params.PageSize
	if err := query.Preload("Protocols").
		Order("sort ASC, id DESC").
		Offset(offset).
		Limit(params.PageSize).
		Find(&nodes).Error; err != nil {
		return nil, err
	}

	// 更新在线状态
	for i := range nodes {
		if nodes[i].IsOnline() {
			nodes[i].Status = model.NodeStatusOnline
		} else if nodes[i].Status == model.NodeStatusOnline {
			nodes[i].Status = model.NodeStatusOffline
		}
	}

	return &NodeListResult{
		Total: total,
		List:  nodes,
	}, nil
}

// GetNode 获取节点详情
func (s *NodeService) GetNode(id uint) (*model.Node, error) {
	var node model.Node
	if err := s.db.Preload("Protocols").First(&node, id).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// CreateNode 创建节点 (手动添加)
func (s *NodeService) CreateNode(node *model.Node) error {
	// 生成 API Key 和 Secret
	apiKey, err := generateSecureToken(32)
	if err != nil {
		return err
	}
	secret, err := generateSecureToken(32)
	if err != nil {
		return err
	}

	node.APIKey = apiKey
	node.APIKeyHash = hashString(apiKey)
	node.Secret = secret
	node.Status = model.NodeStatusPending

	// 创建节点 + 默认协议 (事务)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(node).Error; err != nil {
			return err
		}

		// 创建默认 VMess 协议
		defaultPort := node.Port
		if defaultPort == 0 {
			defaultPort = 443
		}
		defaultTransport := "tcp"
		protocol := &model.NodeProtocol{
			NodeID:    node.ID,
			Name:      "Default VMess",
			Type:      model.ProtocolVMess,
			Port:      defaultPort,
			Enable:    1,
			Sort:      0,
			TLS:       0,
			Transport: &defaultTransport,
		}
		return tx.Create(protocol).Error
	})
}

// UpdateNode 更新节点
func (s *NodeService) UpdateNode(id uint, updates map[string]any) error {
	// 清除缓存
	cache.Delete(CacheKeyNode + string(rune(id)))
	cache.Delete(CacheKeyNodeList)

	return s.db.Model(&model.Node{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteNode 删除节点
func (s *NodeService) DeleteNode(id uint) error {
	// 先删除关联的协议
	if err := s.db.Where("node_id = ?", id).Delete(&model.NodeProtocol{}).Error; err != nil {
		return err
	}
	return s.db.Delete(&model.Node{}, id).Error
}

// ========== 节点自动注册 ==========

// RegisterNode 节点自动注册
func (s *NodeService) RegisterNode(req *model.NodeRegisterRequest, clientIP string) (*model.NodeRegisterResponse, error) {
	// 1. 计算密钥哈希
	keyHash := hashString(req.AuthKey)

	// 2. 生成节点凭证
	apiKey, err := generateSecureToken(32)
	if err != nil {
		return nil, errors.New("生成API密钥失败")
	}
	secret, err := generateSecureToken(32)
	if err != nil {
		return nil, errors.New("生成共享密钥失败")
	}

	// 3. 创建节点 + 默认协议 + 标记授权密钥 (事务 + 悲观锁防并发)
	nodeName := req.Name
	if nodeName == "" {
		nodeName = "Node-" + clientIP
	}

	// Fall back to the client IP when the node did not advertise a host. This
	// matters for gRPC registration (the HTTP handler also fills this in, but
	// the gRPC path goes straight to the service), and keeps the node probeable.
	// clientIP may be "ip:port" (gRPC peer addr) — strip the port to a bare host.
	nodeHost := req.Host
	if nodeHost == "" && clientIP != "" && clientIP != "unknown" {
		if host, _, splitErr := net.SplitHostPort(clientIP); splitErr == nil && host != "" {
			nodeHost = host
		} else {
			nodeHost = clientIP
		}
	}

	node := &model.Node{
		Name:          nodeName,
		Host:          nodeHost,
		Port:          req.Port,
		APIKey:        apiKey,
		APIKeyHash:    hashString(apiKey),
		Secret:        secret,
		Status:        model.NodeStatusOnline,
		ServerIP:      &clientIP,
		ServerVersion: &req.ServerVersion,
		ServerOS:      &req.ServerOS,
		AutoRegister:  1,
	}

	now := time.Now().Unix()
	node.LastCheckAt = &now

	defaultPort := req.Port
	if defaultPort == 0 {
		defaultPort = 443
	}
	defaultTransport := "tcp"
	protocol := &model.NodeProtocol{
		Name:      "Default VMess",
		Type:      model.ProtocolVMess,
		Port:      defaultPort,
		Enable:    1,
		Sort:      0,
		TLS:       0,
		Transport: &defaultTransport,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// 在事务内用 SELECT FOR UPDATE 锁定授权密钥，防止并发注册
		var authKey model.AuthorizedKey
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("key_hash = ?", keyHash).First(&authKey).Error; err != nil {
			return errors.New("授权密钥无效")
		}

		// 检查是否过期
		if authKey.ExpireAt != nil && *authKey.ExpireAt < time.Now().Unix() {
			return errors.New("授权密钥已过期")
		}

		// 创建节点
		if err := tx.Create(node).Error; err != nil {
			return errors.New("创建节点失败")
		}

		// 创建默认协议
		protocol.NodeID = node.ID
		if err := tx.Create(protocol).Error; err != nil {
			return errors.New("创建默认协议失败")
		}

		// 计数器 +1，记录有多少节点用此密钥注册
		if err := tx.Model(&authKey).Update("used", authKey.Used+1).Error; err != nil {
			return errors.New("更新密钥使用计数失败")
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &model.NodeRegisterResponse{
		NodeID:  node.ID,
		APIKey:  apiKey,
		Secret:  secret,
		Message: "节点注册成功",
	}, nil
}

// Heartbeat 节点心跳
func (s *NodeService) Heartbeat(nodeID uint, req *model.NodeHeartbeatRequest) error {
	now := time.Now().Unix()

	updates := map[string]any{
		"cpu_usage":     req.CPUUsage,
		"memory_usage":  req.MemoryUsage,
		"disk_usage":    req.DiskUsage,
		"uptime":        req.Uptime,
		"online_users":  req.OnlineUsers,
		"last_check_at": now,
		"status":        model.NodeStatusOnline,
	}

	// 累加流量
	if req.Upload > 0 || req.Download > 0 {
		s.db.Model(&model.Node{}).Where("id = ?", nodeID).
			UpdateColumn("total_upload", gorm.Expr("total_upload + ?", req.Upload)).
			UpdateColumn("total_download", gorm.Expr("total_download + ?", req.Download))
	}

	result := s.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateLastCheckAt 更新节点最后检查时间 (用于 UniProxy 接口)
func (s *NodeService) UpdateLastCheckAt(nodeID uint) error {
	now := time.Now().Unix()
	return s.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{
		"last_check_at": now,
		"status":        model.NodeStatusOnline,
	}).Error
}

// GetNodeByAPIKey 通过API Key获取节点
func (s *NodeService) GetNodeByAPIKey(apiKey string) (*model.Node, error) {
	keyHash := hashString(apiKey)
	var node model.Node
	if err := s.db.Where("api_key_hash = ?", keyHash).First(&node).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// ========== 协议管理 ==========

// GetProtocols 获取节点的协议列表
func (s *NodeService) GetProtocols(nodeID uint) ([]model.NodeProtocol, error) {
	var protocols []model.NodeProtocol
	if err := s.db.Where("node_id = ?", nodeID).
		Order("sort ASC, id ASC").
		Find(&protocols).Error; err != nil {
		return nil, err
	}
	return protocols, nil
}

// GetProtocolsByGroup 获取关联到特定分组的协议列表
func (s *NodeService) GetProtocolsByGroup(groupID uint) ([]model.NodeProtocol, error) {
	var protocols []model.NodeProtocol
	if err := s.db.Preload("Node").
		Joins("JOIN v2_subscription_group_node_protocols ON v2_subscription_group_node_protocols.node_protocol_id = v2_node_protocol.id").
		Where("v2_subscription_group_node_protocols.subscription_group_id = ?", groupID).
		Find(&protocols).Error; err != nil {
		return nil, err
	}
	return protocols, nil
}

// AssignProtocolsToGroup 为分组分配节点协议
func (s *NodeService) AssignProtocolsToGroup(groupID uint, protocolIDs []uint) error {
	var group model.SubscriptionGroup
	if err := s.db.First(&group, groupID).Error; err != nil {
		return err
	}

	var protocols []model.NodeProtocol
	if len(protocolIDs) > 0 {
		if err := s.db.Where("id IN ?", protocolIDs).Find(&protocols).Error; err != nil {
			return err
		}
	}

	// 替换关联（清空旧的并添加新的）
	return s.db.Model(&group).Association("Protocols").Replace(protocols)
}

// GetAllAvailableProtocols 获取所有可显示在订阅中的协议
func (s *NodeService) GetAllAvailableProtocols() ([]model.NodeProtocol, error) {
	var protocols []model.NodeProtocol
	if err := s.db.Preload("Node").Preload("SubscriptionGroups").
		Where("show = 1").
		Order("node_id ASC, sort ASC").
		Find(&protocols).Error; err != nil {
		return nil, err
	}
	return protocols, nil
}

// GetProtocol 获取协议详情
func (s *NodeService) GetProtocol(id uint) (*model.NodeProtocol, error) {
	var protocol model.NodeProtocol
	if err := s.db.Preload("Node").First(&protocol, id).Error; err != nil {
		return nil, err
	}
	return &protocol, nil
}

// CreateProtocol 创建协议
func (s *NodeService) CreateProtocol(protocol *model.NodeProtocol) error {
	// 验证节点存在
	var node model.Node
	if err := s.db.First(&node, protocol.NodeID).Error; err != nil {
		return errors.New("节点不存在")
	}

	return s.db.Create(protocol).Error
}

// UpdateProtocol 更新协议
func (s *NodeService) UpdateProtocol(id uint, updates map[string]any) error {
	return s.db.Model(&model.NodeProtocol{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteProtocol 删除协议
func (s *NodeService) DeleteProtocol(id uint) error {
	return s.db.Delete(&model.NodeProtocol{}, id).Error
}

// SyncProtocolToNode 同步协议配置到节点
func (s *NodeService) SyncProtocolToNode(nodeID uint) error {
	// Unimplemented: push protocol configuration to node via API.
	// When implemented, this should fetch the node's protocol configs
	// (NodeProtocol records) and push them to the node through either:
	//   - gRPC ConfigSync service (preferred for connected nodes)
	//   - REST API call to the node's management endpoint
	// This requires the node to implement a config-receive endpoint.
	log.Printf("[node] SyncProtocolToNode: config push to node %d not yet implemented", nodeID)
	return nil
}

// ========== 授权密钥管理 ==========

// GenerateAuthKey 生成授权密钥
func (s *NodeService) GenerateAuthKey(name string, expireDays int) (*model.AuthorizedKey, string, error) {
	key, err := generateSecureToken(32)
	if err != nil {
		return nil, "", err
	}

	authKey := &model.AuthorizedKey{
		Name:    name,
		Key:     key,
		KeyHash: hashString(key),
		Used:    0,
	}

	if expireDays > 0 {
		expireAt := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour).Unix()
		authKey.ExpireAt = &expireAt
	}

	if err := s.db.Create(authKey).Error; err != nil {
		return nil, "", err
	}

	return authKey, key, nil
}

// GetAuthKeys 获取授权密钥列表
func (s *NodeService) GetAuthKeys() ([]model.AuthorizedKey, error) {
	var keys []model.AuthorizedKey
	if err := s.db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteAuthKey 删除授权密钥
func (s *NodeService) DeleteAuthKey(id uint) error {
	return s.db.Delete(&model.AuthorizedKey{}, id).Error
}

// ========== 统计 ==========

// GetNodeStats 获取节点统计
func (s *NodeService) GetNodeStats() (map[string]any, error) {
	var totalNodes, onlineNodes, pendingNodes int64
	var totalUpload, totalDownload int64

	s.db.Model(&model.Node{}).Count(&totalNodes)

	fiveMinAgo := time.Now().Unix() - 300
	s.db.Model(&model.Node{}).Where("last_check_at > ?", fiveMinAgo).Count(&onlineNodes)
	s.db.Model(&model.Node{}).Where("status = ?", model.NodeStatusPending).Count(&pendingNodes)

	s.db.Model(&model.Node{}).
		Select("COALESCE(SUM(total_upload), 0)").
		Scan(&totalUpload)
	s.db.Model(&model.Node{}).
		Select("COALESCE(SUM(total_download), 0)").
		Scan(&totalDownload)

	return map[string]any{
		"total":   totalNodes,
		"online":  onlineNodes,
		"pending": pendingNodes,
		"offline": totalNodes - onlineNodes - pendingNodes,
		"up":      totalUpload,
		"down":    totalDownload,
	}, nil
}

// ========== 初始化 ==========

// InitDefaultAuthKeyFromEnv 从环境变量初始化默认授权密钥
// 环境变量: NODE_DEFAULT_AUTH_KEY
// 如果设置了该环境变量：
//   - 如果密钥内容已存在 → 跳过
//   - 如果存在同名默认密钥但内容不同 → 删除旧的，创建新的
//   - 如果不存在 → 创建
//
// Note: 改变 AuthKey 只会影响新节点注册，已注册节点使用 APIKey 通信，不受影响
func InitDefaultAuthKeyFromEnv() {
	defaultKey := os.Getenv("NODE_DEFAULT_AUTH_KEY")
	if defaultKey == "" {
		return // 环境变量未设置，跳过
	}

	db := database.Get()
	keyHash := hashString(defaultKey)

	// 1. 检查密钥内容是否已经存在（相同哈希就是相同密钥）
	var count int64
	db.Model(&model.AuthorizedKey{}).Where("key_hash = ?", keyHash).Count(&count)
	if count > 0 {
		log.Printf("Default auth key from environment already exists (hash: %s), skipping", keyHash[:12])
		return
	}

	// 2. 如果存在同名"Default (from env)"，删除旧的（环境变量已改变，需要更新）
	var existingIDs []uint
	db.Model(&model.AuthorizedKey{}).
		Where("name = ?", "Default (from env)").
		Pluck("id", &existingIDs)
	if len(existingIDs) > 0 {
		log.Printf("Removing old default auth key (name matches, content changed)...")
		for _, id := range existingIDs {
			db.Delete(&model.AuthorizedKey{}, id)
		}
	}

	// 3. 创建新的授权密钥
	log.Println("Creating default auth key from environment...")
	authKey := &model.AuthorizedKey{
		Name:    "Default (from env)",
		Key:     defaultKey,
		KeyHash: keyHash,
		Used:    0,
	}

	if err := db.Create(authKey).Error; err != nil {
		log.Printf("Failed to create default auth key: %v", err)
		return
	}

	log.Printf("Default auth key created/updated successfully (name: %s, hash: %s)", authKey.Name, keyHash[:12])
}

// ========== 辅助函数 ==========

// generateSecureToken 生成安全随机 token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashString 对字符串进行 SHA256 哈希
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// ========== 参数结构 ==========

type NodeListParams struct {
	Page     int
	PageSize int
	Status   *model.NodeStatus
	GroupID  *uint
	Search   string
}

type NodeListResult struct {
	Total int64        `json:"total"`
	List  []model.Node `json:"list"`
}
