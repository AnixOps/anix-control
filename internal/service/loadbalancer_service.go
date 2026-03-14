package service

import (
	"encoding/json"
	"errors"
	"math/rand"
	"sync"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// LoadBalancerService 负载均衡服务
type LoadBalancerService struct {
	db       *gorm.DB
	nodeSvc  *ForwardNodeService
	counters map[uint]int // 轮询计数器
	mu       sync.RWMutex
}

// NewLoadBalancerService 创建服务
func NewLoadBalancerService(db *gorm.DB) *LoadBalancerService {
	return &LoadBalancerService{
		db:       db,
		nodeSvc:  NewForwardNodeService(db),
		counters: make(map[uint]int),
	}
}

// Create 创建负载均衡器
func (s *LoadBalancerService) Create(lb *model.LoadBalancer) error {
	return s.db.Create(lb).Error
}

// Update 更新负载均衡器
func (s *LoadBalancerService) Update(lb *model.LoadBalancer) error {
	return s.db.Save(lb).Error
}

// Delete 删除负载均衡器
func (s *LoadBalancerService) Delete(id uint) error {
	return s.db.Delete(&model.LoadBalancer{}, id).Error
}

// GetByID 获取负载均衡器
func (s *LoadBalancerService) GetByID(id uint) (*model.LoadBalancer, error) {
	var lb model.LoadBalancer
	err := s.db.First(&lb, id).Error
	if err != nil {
		return nil, err
	}
	return &lb, nil
}

// List 获取负载均衡器列表
func (s *LoadBalancerService) List(groupID uint) ([]model.LoadBalancer, error) {
	var lbs []model.LoadBalancer
	db := s.db.Model(&model.LoadBalancer{})
	if groupID > 0 {
		db = db.Where("group_id = ?", groupID)
	}
	err := db.Order("created_at DESC").Find(&lbs).Error
	return lbs, err
}

// SelectNode 选择最佳节点
func (s *LoadBalancerService) SelectNode(lbID uint) (*model.ForwardNode, error) {
	lb, err := s.GetByID(lbID)
	if err != nil {
		return nil, err
	}

	if !lb.Enabled {
		return nil, errors.New("load balancer is disabled")
	}

	// 获取节点组内的在线节点
	nodes, err := s.getNodesByGroup(lb.GroupID, 1)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, errors.New("no available nodes")
	}

	// 解析节点权重
	weights := make(map[uint]int)
	if lb.NodeWeights != "" {
		json.Unmarshal([]byte(lb.NodeWeights), &weights)
	}

	// 根据策略选择节点
	switch lb.Strategy {
	case "round-robin":
		return s.selectRoundRobin(lbID, nodes), nil
	case "least-load":
		return s.selectLeastLoad(nodes), nil
	case "latency":
		return s.selectLatency(nodes), nil
	case "weight":
		return s.selectWeight(nodes, weights), nil
	case "random":
		return s.selectRandom(nodes), nil
	default:
		return s.selectRoundRobin(lbID, nodes), nil
	}
}

// getNodesByGroup 获取节点组内的节点
func (s *LoadBalancerService) getNodesByGroup(groupID uint, status int) ([]model.ForwardNode, error) {
	var nodes []model.ForwardNode
	db := s.db.Model(&model.ForwardNode{})
	if status > 0 {
		db = db.Where("status = ?", status)
	}
	err := db.Find(&nodes).Error
	return nodes, err
}

// selectRoundRobin 轮询选择
func (s *LoadBalancerService) selectRoundRobin(lbID uint, nodes []model.ForwardNode) *model.ForwardNode {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := s.counters[lbID]
	index := count % len(nodes)
	s.counters[lbID] = count + 1

	return &nodes[index]
}

// selectLeastLoad 最小负载选择
func (s *LoadBalancerService) selectLeastLoad(nodes []model.ForwardNode) *model.ForwardNode {
	var selected *model.ForwardNode
	minLoad := float64(2) // 最大负载是1

	for i := range nodes {
		if nodes[i].Load < minLoad {
			minLoad = nodes[i].Load
			selected = &nodes[i]
		}
	}

	if selected == nil && len(nodes) > 0 {
		return &nodes[0]
	}

	return selected
}

// selectLatency 最低延迟选择
func (s *LoadBalancerService) selectLatency(nodes []model.ForwardNode) *model.ForwardNode {
	var selected *model.ForwardNode
	minLatency := int(^uint(0) >> 1) // Max int

	for i := range nodes {
		if nodes[i].Latency > 0 && nodes[i].Latency < minLatency {
			minLatency = nodes[i].Latency
			selected = &nodes[i]
		}
	}

	if selected == nil && len(nodes) > 0 {
		return &nodes[0]
	}

	return selected
}

// selectWeight 加权选择
func (s *LoadBalancerService) selectWeight(nodes []model.ForwardNode, weights map[uint]int) *model.ForwardNode {
	// 计算总权重
	totalWeight := 0
	for _, node := range nodes {
		w := weights[node.ID]
		if w <= 0 {
			w = 1 // 默认权重为1
		}
		totalWeight += w
	}

	if totalWeight == 0 {
		return s.selectRandom(nodes)
	}

	// 随机选择
	r := rand.Intn(totalWeight)
	current := 0

	for i := range nodes {
		w := weights[nodes[i].ID]
		if w <= 0 {
			w = 1
		}
		current += w
		if r < current {
			return &nodes[i]
		}
	}

	return &nodes[0]
}

// selectRandom 随机选择
func (s *LoadBalancerService) selectRandom(nodes []model.ForwardNode) *model.ForwardNode {
	r := rand.Intn(len(nodes))
	return &nodes[r]
}

// RunHealthCheck 执行健康检查
func (s *LoadBalancerService) RunHealthCheck(lbID uint) error {
	lb, err := s.GetByID(lbID)
	if err != nil {
		return err
	}

	if !lb.HealthCheck {
		return nil
	}

	// 获取节点组内的所有节点
	nodes, err := s.getNodesByGroup(lb.GroupID, 0)
	if err != nil {
		return err
	}

	// 对每个节点执行健康检查
	for i := range nodes {
		_, err := s.nodeSvc.HealthCheck(nil, nodes[i].ID)
		if err != nil {
			// 标记节点离线
			nodes[i].Status = 0
		} else {
			// 标记节点在线
			nodes[i].Status = 1
		}
		s.nodeSvc.Update(&nodes[i])
	}

	return nil
}

// GetStats 获取负载均衡统计
func (s *LoadBalancerService) GetStats(lbID uint) (map[string]interface{}, error) {
	lb, err := s.GetByID(lbID)
	if err != nil {
		return nil, err
	}

	nodes, err := s.getNodesByGroup(lb.GroupID, 0)
	if err != nil {
		return nil, err
	}

	onlineCount := 0
	totalLoad := float64(0)
	avgLatency := 0

	for _, node := range nodes {
		if node.Status == 1 {
			onlineCount++
			avgLatency += node.Latency
			totalLoad += node.Load
		}
	}

	if onlineCount > 0 {
		avgLatency /= onlineCount
		totalLoad /= float64(onlineCount)
	}

	return map[string]interface{}{
		"total_nodes":  len(nodes),
		"online_nodes": onlineCount,
		"avg_load":     totalLoad,
		"avg_latency":  avgLatency,
		"strategy":     lb.Strategy,
	}, nil
}