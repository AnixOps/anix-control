package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/cache"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

// ForwardNodeService 转发节点服务
type ForwardNodeService struct {
	db *gorm.DB
}

var forwardNodeRandomInt = cryptoRandomInt

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
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", node.Host, node.Port))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		result.Status = model.ForwardNodeStatusOffline
		result.Error = err.Error()
	} else {
		if err := conn.Close(); err != nil {
			return nil, fmt.Errorf("close health check connection: %w", err)
		}
		result.Status = model.ForwardNodeStatusOnline
		result.Latency = time.Since(start).Milliseconds()
	}

	// 更新节点状态
	now := time.Now()
	updates := map[string]any{
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
		if err := s.db.Model(&model.ForwardNode{}).
			Where("id = ?", node.ID).
			Select("COUNT(*) as total, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as online", model.ForwardNodeStatusOnline).
			Scan(&stats).Error; err != nil {
			return nil, fmt.Errorf("calculate forward node uptime: %w", err)
		}
		if stats.Total > 0 {
			updates["uptime"] = float64(stats.Online) / float64(stats.Total) * 100
		}
	}

	if err := s.db.Model(&model.ForwardNode{}).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update forward node health check: %w", err)
	}

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
	errs := make([]error, len(nodes))
	var wg sync.WaitGroup

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, nodeID uint) {
			defer wg.Done()
			result, err := s.HealthCheck(ctx, nodeID)
			results[idx] = result
			errs[idx] = err
		}(i, node.ID)
	}

	wg.Wait()
	return results, errors.Join(errs...)
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
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
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
			if n.Weight > 0 {
				totalWeight += n.Weight
			}
		}
		if totalWeight == 0 {
			return nodes[0], nil
		}
		// 简单实现: 按权重比例选择
		weights := make([]int, len(nodes))
		current := 0
		for i := range nodes {
			if nodes[i].Weight > 0 {
				current += nodes[i].Weight
			}
			weights[i] = current
		}
		// 随机选择
		r, err := forwardNodeRandomInt(totalWeight)
		if err != nil {
			return nil, fmt.Errorf("select weighted forward node: %w", err)
		}
		for i, w := range weights {
			if r < w {
				return nodes[i], nil
			}
		}
		return nodes[0], nil

	case "random":
		// 随机选择
		idx, err := forwardNodeRandomInt(len(nodes))
		if err != nil {
			return nil, fmt.Errorf("select random forward node: %w", err)
		}
		return nodes[idx], nil

	default: // round-robin
		// 轮询 (使用缓存计数)
		key := fmt.Sprintf("forward_lb_%s", nodeType)
		idx := int(cache.Incr(key)) % len(nodes)
		return nodes[idx], nil
	}
}

// randBytes 生成随机字节
func randBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, errors.New("random byte length must be positive")
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateAPIToken 生成API Token
func (s *ForwardNodeService) GenerateAPIToken() (string, error) {
	b, err := randBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// UpdateStats 更新节点统计
func (s *ForwardNodeService) UpdateStats(nodeID uint, upload, download int64, connDelta int) error {
	return s.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).Updates(map[string]any{
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
	result, err := s.ParseTagsWithError(tags)
	if err != nil {
		return []string{}
	}
	return result
}

// ParseTagsWithError 解析标签并返回无效 JSON 错误
func (s *ForwardNodeService) ParseTagsWithError(tags string) ([]string, error) {
	if tags == "" {
		return []string{}, nil
	}
	var result []string
	if err := json.Unmarshal([]byte(tags), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SetTags 设置标签
func (s *ForwardNodeService) SetTags(nodeID uint, tags []string) error {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	return s.db.Model(&model.ForwardNode{}).Where("id = ?", nodeID).
		Update("tags", string(tagsJSON)).Error
}
