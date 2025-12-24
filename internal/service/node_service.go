package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	return s.db.Create(node).Error
}

// UpdateNode 更新节点
func (s *NodeService) UpdateNode(id uint, updates map[string]interface{}) error {
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
	// 1. 验证授权密钥
	keyHash := hashString(req.AuthKey)
	var authKey model.AuthorizedKey
	if err := s.db.Where("key_hash = ?", keyHash).First(&authKey).Error; err != nil {
		return nil, errors.New("授权密钥无效")
	}

	// 检查是否已使用
	if authKey.Used == 1 {
		return nil, errors.New("授权密钥已被使用")
	}

	// 检查是否过期
	if authKey.ExpireAt != nil && *authKey.ExpireAt < time.Now().Unix() {
		return nil, errors.New("授权密钥已过期")
	}

	// 2. 生成节点凭证
	apiKey, err := generateSecureToken(32)
	if err != nil {
		return nil, errors.New("生成API密钥失败")
	}
	secret, err := generateSecureToken(32)
	if err != nil {
		return nil, errors.New("生成共享密钥失败")
	}

	// 3. 创建节点
	nodeName := req.Name
	if nodeName == "" {
		nodeName = "Node-" + clientIP
	}

	node := &model.Node{
		Name:          nodeName,
		Host:          req.Host,
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

	if err := s.db.Create(node).Error; err != nil {
		return nil, errors.New("创建节点失败")
	}

	// 4. 标记授权密钥已使用
	s.db.Model(&authKey).Updates(map[string]interface{}{
		"used":            1,
		"used_by_node_id": node.ID,
	})

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

	updates := map[string]interface{}{
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

	return s.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates).Error
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
func (s *NodeService) UpdateProtocol(id uint, updates map[string]interface{}) error {
	return s.db.Model(&model.NodeProtocol{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteProtocol 删除协议
func (s *NodeService) DeleteProtocol(id uint) error {
	return s.db.Delete(&model.NodeProtocol{}, id).Error
}

// SyncProtocolToNode 同步协议配置到节点
func (s *NodeService) SyncProtocolToNode(nodeID uint) error {
	// TODO: 通过 API 将配置推送到节点
	// 这需要节点端实现相应的接收接口
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
func (s *NodeService) GetNodeStats() (map[string]interface{}, error) {
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

	return map[string]interface{}{
		"total_nodes":    totalNodes,
		"online_nodes":   onlineNodes,
		"pending_nodes":  pendingNodes,
		"total_upload":   totalUpload,
		"total_download": totalDownload,
	}, nil
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
