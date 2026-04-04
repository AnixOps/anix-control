package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// ForwardNodeService 转发节点服务
type ForwardNodeService struct {
	db *gorm.DB
	mu sync.RWMutex
}

// NewForwardNodeService 创建服务
func NewForwardNodeService(db *gorm.DB) *ForwardNodeService {
	return &ForwardNodeService{db: db}
}

// Create 创建节点
func (s *ForwardNodeService) Create(node *model.ForwardNode) error {
	return s.db.Create(node).Error
}

// Update 更新节点
func (s *ForwardNodeService) Update(node *model.ForwardNode) error {
	return s.db.Save(node).Error
}

// Delete 删除节点
func (s *ForwardNodeService) Delete(id uint) error {
	return s.db.Delete(&model.ForwardNode{}, id).Error
}

// GetByID 根据ID获取节点
func (s *ForwardNodeService) GetByID(id uint) (*model.ForwardNode, error) {
	var node model.ForwardNode
	err := s.db.First(&node, id).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// GetByType 根据类型获取节点列表
func (s *ForwardNodeService) GetByType(nodeType string) ([]*model.ForwardNode, error) {
	var nodes []*model.ForwardNode
	err := s.db.Where("type = ? AND enabled = ?", nodeType, true).Find(&nodes).Error
	return nodes, err
}

// List 获取节点列表
func (s *ForwardNodeService) List(nodeType string, status *int, page, pageSize int) ([]*model.ForwardNode, int64, error) {
	var nodes []*model.ForwardNode
	var total int64

	query := s.db.Model(&model.ForwardNode{})
	if nodeType != "" {
		query = query.Where("type = ?", nodeType)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&nodes).Error
	return nodes, total, err
}

// GetOnlineNodes 获取在线节点
func (s *ForwardNodeService) GetOnlineNodes(nodeType string) ([]*model.ForwardNode, error) {
	var nodes []*model.ForwardNode
	query := s.db.Where("status = ? AND enabled = ?", model.ForwardNodeStatusOnline, true)
	if nodeType != "" {
		query = query.Where("type = ?", nodeType)
	}
	err := query.Order("latency ASC").Find(&nodes).Error
	return nodes, err
}

// HealthCheck 健康检查
func (s *ForwardNodeService) HealthCheck(ctx context.Context, nodeID uint) (*HealthCheckResult, error) {
	node, err := s.GetByID(nodeID)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result := &HealthCheckResult{
		NodeID:    node.ID,
		CheckTime: start,
	}

	// TCP连接测试
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", node.Host, node.Port), 5*time.Second)
	if err != nil {
		result.Status = model.ForwardNodeStatusOffline
		result.Error = err.Error()
	} else {
		conn.Close()
		result.Status = model.ForwardNodeStatusOnline
		result.Latency = time.Since(start).Milliseconds()
	}

	// 更新节点状态
	now := time.Now()
	updates := map[string]interface{}{
		"status":     result.Status,
		"last_check": now,
		"latency":    result.Latency,
	}

	if result.Status == model.ForwardNodeStatusOnline {
		// 计算在线率
		var stats struct {
			Total  int64
			Online int64
		}
		s.db.Model(&model.ForwardNode{}).
			Where("id = ?", node.ID).
			Select("COUNT(*) as total, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as online", model.ForwardNodeStatusOnline).
			Scan(&stats)
		if stats.Total > 0 {
			updates["uptime"] = float64(stats.Online) / float64(stats.Total) * 100
		}
	}

	s.db.Model(&node).Updates(updates)

	return result, nil
}

// HealthCheckAll 检查所有节点
func (s *ForwardNodeService) HealthCheckAll(ctx context.Context) ([]*HealthCheckResult, error) {
	var nodes []*model.ForwardNode
	err := s.db.Where("enabled = ?", true).Find(&nodes).Error
	if err != nil {
		return nil, err
	}

	results := make([]*HealthCheckResult, len(nodes))
	var wg sync.WaitGroup

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, nodeID uint) {
			defer wg.Done()
			result, _ := s.HealthCheck(ctx, nodeID)
			results[idx] = result
		}(i, node.ID)
	}

	wg.Wait()
	return results, nil
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	NodeID    uint      `json:"node_id"`
	Status    int       `json:"status"`
	Latency   int64     `json:"latency"`
	CheckTime time.Time `json:"check_time"`
	Error     string    `json:"error,omitempty"`
}

// SelectBestNode 选择最佳节点 (负载均衡)
func (s *ForwardNodeService) SelectBestNode(nodeType string, mode string) (*model.ForwardNode, error) {
	nodes, err := s.GetOnlineNodes(nodeType)
	if err != nil || len(nodes) == 0 {
		return nil, fmt.Errorf("no available nodes")
	}

	switch mode {
	case "latency":
		// 最低延迟
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Latency < nodes[j].Latency
		})
		return nodes[0], nil

	case "least-conn":
		// 最少连接
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].CurrentConn < nodes[j].CurrentConn
		})
		return nodes[0], nil

	case "weight":
		// 加权随机
		totalWeight := 0
		for _, n := range nodes {
			totalWeight += n.Weight
		}
		if totalWeight == 0 {
			return nodes[0], nil
		}
		// 简单实现: 按权重比例选择
		weights := make([]int, len(nodes))
		weights[0] = nodes[0].Weight
		for i := 1; i < len(nodes); i++ {
			weights[i] = weights[i-1] + nodes[i].Weight
		}
		// 随机选择
		r := int(randBytes(1)[0]) % totalWeight
		for i, w := range weights {
			if r < w {
				return nodes[i], nil
			}
		}
		return nodes[0], nil

	case "random":
		// 随机选择
		idx := int(randBytes(1)[0]) % len(nodes)
		return nodes[idx], nil

	default: // round-robin
		// 轮询 (使用缓存计数)
		key := fmt.Sprintf("forward_lb_%s", nodeType)
		idx := int(cache.Incr(key)) % len(nodes)
		return nodes[idx], nil
	}
}

// randBytes 生成随机字节
func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// GenerateAPIToken 生成API Token
func (s *ForwardNodeService) GenerateAPIToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// UpdateStats 更新节点统计
func (s *ForwardNodeService) UpdateStats(nodeID uint, upload, download int64, connDelta int) error {
	return s.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
		"total_upload":   gorm.Expr("total_upload + ?", upload),
		"total_download": gorm.Expr("total_download + ?", download),
		"current_conn":   gorm.Expr("current_conn + ?", connDelta),
	}).Error
}

// GetNodesByGroup 根据标签获取节点组
func (s *ForwardNodeService) GetNodesByGroup(nodeType, group string) ([]*model.ForwardNode, error) {
	var nodes []*model.ForwardNode

	// 查找包含指定标签的节点
	err := s.db.Where("type = ? AND enabled = ? AND status = ?",
		nodeType, true, model.ForwardNodeStatusOnline).
		Where("tags LIKE ?", fmt.Sprintf("%%\"%s\"%%", group)).
		Find(&nodes).Error
	return nodes, err
}

// ParseTags 解析标签
func (s *ForwardNodeService) ParseTags(tags string) []string {
	if tags == "" {
		return []string{}
	}
	var result []string
	json.Unmarshal([]byte(tags), &result)
	return result
}

// SetTags 设置标签
func (s *ForwardNodeService) SetTags(nodeID uint, tags []string) error {
	tagsJSON, _ := json.Marshal(tags)
	return s.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).
		Update("tags", string(tagsJSON)).Error
}
