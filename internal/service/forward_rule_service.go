package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// ForwardRuleService 转发规则服务
type ForwardRuleService struct {
	db          *gorm.DB
	nodeService *ForwardNodeService
	gostManager *gost.Manager
	mu          sync.RWMutex
}

// NewForwardRuleService 创建服务
func NewForwardRuleService(db *gorm.DB, nodeService *ForwardNodeService) *ForwardRuleService {
	return &ForwardRuleService{
		db:          db,
		nodeService: nodeService,
		gostManager: gost.NewManager(db),
	}
}

// Create 创建规则
func (s *ForwardRuleService) Create(rule *model.ForwardRule) error {
	// 验证端口是否可用
	if err := s.validateRule(rule); err != nil {
		return err
	}

	// 保存到数据库
	if err := s.db.Create(rule).Error; err != nil {
		return err
	}

	// 如果规则启用，同步到 gost
	if rule.Enabled {
		ctx := context.Background()
		if err := s.gostManager.CreateForwardRule(ctx, rule); err != nil {
			// 记录错误但不回滚数据库操作
			// 可以通过后台任务重试同步
			fmt.Printf("sync rule %d to gost failed: %v\n", rule.ID, err)
		}
	}

	return nil
}

// Update 更新规则
func (s *ForwardRuleService) Update(rule *model.ForwardRule) error {
	if err := s.validateRule(rule); err != nil {
		return err
	}

	if err := s.db.Save(rule).Error; err != nil {
		return err
	}

	// 同步到 gost
	ctx := context.Background()
	if err := s.gostManager.UpdateForwardRule(ctx, rule); err != nil {
		fmt.Printf("sync rule %d to gost failed: %v\n", rule.ID, err)
	}

	return nil
}

// Delete 删除规则
func (s *ForwardRuleService) Delete(id uint) error {
	// 先获取规则信息
	rule, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// 从 gost 删除
	ctx := context.Background()
	if err := s.gostManager.DeleteForwardRule(ctx, rule); err != nil {
		fmt.Printf("delete rule %d from gost failed: %v\n", id, err)
	}

	return s.db.Delete(&model.ForwardRule{}, id).Error
}

// GetByID 根据ID获取规则
func (s *ForwardRuleService) GetByID(id uint) (*model.ForwardRule, error) {
	var rule model.ForwardRule
	err := s.db.Preload("RelayNode").Preload("ExitNode").First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// List 获取规则列表
func (s *ForwardRuleService) List(page, pageSize int, userID *uint) ([]*model.ForwardRule, int64, error) {
	var rules []*model.ForwardRule
	var total int64

	query := s.db.Model(&model.ForwardRule{}).Preload("RelayNode").Preload("ExitNode")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&rules).Error
	return rules, total, err
}

// GetEnabledRules 获取启用的规则
func (s *ForwardRuleService) GetEnabledRules() ([]*model.ForwardRule, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("enabled = ?", true).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules).Error
	return rules, err
}

// GetUserRules 获取用户的规则
func (s *ForwardRuleService) GetUserRules(userID uint) ([]*model.ForwardRule, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("user_id = ?", userID).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules).Error
	return rules, err
}

// Toggle 切换规则状态
func (s *ForwardRuleService) Toggle(id uint, enabled bool) error {
	rule, err := s.GetByID(id)
	if err != nil {
		return err
	}

	if err := s.db.Model(&model.ForwardRule{}).Where("id = ?", id).
		Update("enabled", enabled).Error; err != nil {
		return err
	}

	rule.Enabled = enabled

	// 同步到 gost
	ctx := context.Background()
	if err := s.gostManager.SyncRuleToGost(ctx, rule); err != nil {
		fmt.Printf("sync rule %d to gost failed: %v\n", id, err)
	}

	return nil
}

// validateRule 验证规则
func (s *ForwardRuleService) validateRule(rule *model.ForwardRule) error {
	// 检查中转节点是否存在
	relayNode, err := s.nodeService.GetByID(rule.RelayNodeID)
	if err != nil {
		return fmt.Errorf("relay node not found")
	}
	if relayNode.Type != model.ForwardNodeTypeRelay {
		return fmt.Errorf("node is not a relay node")
	}

	// 检查落地节点是否存在
	exitNode, err := s.nodeService.GetByID(rule.ExitNodeID)
	if err != nil {
		return fmt.Errorf("exit node not found")
	}
	if exitNode.Type != model.ForwardNodeTypeExit {
		return fmt.Errorf("node is not an exit node")
	}

	// 检查端口冲突 (同一中转节点上的监听端口不能重复)
	var count int64
	s.db.Model(&model.ForwardRule{}).
		Where("relay_node_id = ? AND listen_port = ? AND id != ?",
			rule.RelayNodeID, rule.ListenPort, rule.ID).
		Count(&count)
	if count > 0 {
		return fmt.Errorf("listen port %d is already in use on this relay node", rule.ListenPort)
	}

	return nil
}

// MatchRule 匹配转发规则
func (s *ForwardRuleService) MatchRule(sourceIP string, targetHost string, targetPort int) (*model.ForwardRule, error) {
	rules, err := s.GetEnabledRules()
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		// 检查目标端口
		if rule.TargetPort != targetPort {
			continue
		}

		// 检查目标地址
		if rule.TargetHost != targetHost && rule.TargetHost != "0.0.0.0" {
			continue
		}

		// 检查用户限制
		if rule.UserID != nil {
			// 需要验证用户身份
			continue
		}

		// 检查过期时间
		if rule.ExpireTime != nil && rule.ExpireTime.Before(time.Now()) {
			continue
		}

		// 检查流量限制
		if rule.TrafficLimit != nil {
			total := rule.Upload + rule.Download
			if total >= *rule.TrafficLimit {
				continue
			}
		}

		return rule, nil
	}

	return nil, fmt.Errorf("no matching rule found")
}

// UpdateTraffic 更新流量统计
func (s *ForwardRuleService) UpdateTraffic(ruleID uint, upload, download int64) error {
	return s.db.Model(&model.ForwardRule{}).Where("id = ?", ruleID).Updates(map[string]interface{}{
		"upload":   gorm.Expr("upload + ?", upload),
		"download": gorm.Expr("download + ?", download),
	}).Error
}

// UpdateConnections 更新连接数
func (s *ForwardRuleService) UpdateConnections(ruleID uint, delta int) error {
	return s.db.Model(&model.ForwardRule{}).Where("id = ?", ruleID).Updates(map[string]interface{}{
		"connections":  gorm.Expr("connections + ?", delta),
		"total_conns": gorm.Expr("total_conns + ?", max(delta, 0)),
	}).Error
}

// GetPortMapping 获取端口映射表
func (s *ForwardRuleService) GetPortMapping(relayNodeID uint) (map[int]*model.ForwardRule, error) {
	rules, err := s.GetEnabledRules()
	if err != nil {
		return nil, err
	}

	mapping := make(map[int]*model.ForwardRule)
	for _, rule := range rules {
		if rule.RelayNodeID == relayNodeID {
			mapping[rule.ListenPort] = rule
		}
	}
	return mapping, nil
}

// GetTrafficStats 获取流量统计
func (s *ForwardRuleService) GetTrafficStats(ruleID uint, start, end time.Time) ([]*model.ForwardStats, error) {
	var stats []*model.ForwardStats
	err := s.db.Where("rule_id = ? AND date >= ? AND date <= ?", ruleID, start, end).
		Order("date ASC, hour ASC").
		Find(&stats).Error
	return stats, err
}

// GetFreePort 获取空闲端口
func (s *ForwardRuleService) GetFreePort(relayNodeID uint, startPort, endPort int) (int, error) {
	usedPorts, err := s.getUsedPorts(relayNodeID)
	if err != nil {
		return 0, err
	}

	for port := startPort; port <= endPort; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range %d-%d", startPort, endPort)
}

// getUsedPorts 获取已使用的端口
func (s *ForwardRuleService) getUsedPorts(relayNodeID uint) (map[int]bool, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("relay_node_id = ?", relayNodeID).Find(&rules).Error
	if err != nil {
		return nil, err
	}

	used := make(map[int]bool)
	for _, r := range rules {
		used[r.ListenPort] = true
	}
	return used, nil
}

// CheckPortAvailable 检查端口是否可用
func (s *ForwardRuleService) CheckPortAvailable(relayNodeID uint, port int) (bool, error) {
	var count int64
	s.db.Model(&model.ForwardRule{}).
		Where("relay_node_id = ? AND listen_port = ?", relayNodeID, port).
		Count(&count)
	return count == 0, nil
}

// ValidateUserRule 验证用户规则权限
func (s *ForwardRuleService) ValidateUserRule(userID, ruleID uint) (bool, error) {
	rule, err := s.GetByID(ruleID)
	if err != nil {
		return false, err
	}

	// 公共规则
	if rule.UserID == nil {
		return true, nil
	}

	// 用户自己的规则
	return *rule.UserID == userID, nil
}

// CreateRuleForUser 为用户创建规则
func (s *ForwardRuleService) CreateRuleForUser(userID uint, req *CreateRuleRequest) (*model.ForwardRule, error) {
	// 获取空闲端口
	port, err := s.GetFreePort(req.RelayNodeID, 10000, 65535)
	if err != nil {
		return nil, fmt.Errorf("no available port: %w", err)
	}

	rule := &model.ForwardRule{
		Name:         req.Name,
		Enabled:      true,
		RelayNodeID:  req.RelayNodeID,
		ListenPort:   port,
		Protocol:     req.Protocol,
		ExitNodeID:   req.ExitNodeID,
		TargetHost:   req.TargetHost,
		TargetPort:   req.TargetPort,
		UserID:       &userID,
		SpeedLimit:   req.SpeedLimit,
		TrafficLimit: req.TrafficLimit,
		ExpireTime:   req.ExpireTime,
	}

	if err := s.Create(rule); err != nil {
		return nil, err
	}

	return rule, nil
}

// CreateRuleRequest 创建规则请求
type CreateRuleRequest struct {
	Name         string     `json:"name"`
	RelayNodeID  uint       `json:"relay_node_id"`
	ExitNodeID   uint       `json:"exit_node_id"`
	Protocol     string     `json:"protocol"`
	TargetHost   string     `json:"target_host"`
	TargetPort   int        `json:"target_port"`
	SpeedLimit   *int64     `json:"speed_limit"`
	TrafficLimit *int64     `json:"traffic_limit"`
	ExpireTime   *time.Time `json:"expire_time"`
}

// GetConfigForNode 获取节点配置 (用于下发到节点)
func (s *ForwardRuleService) GetConfigForNode(nodeID uint) (*NodeForwardConfig, error) {
	rules, err := s.GetEnabledRules()
	if err != nil {
		return nil, err
	}

	config := &NodeForwardConfig{
		NodeID: nodeID,
		Rules:  make([]ForwardRuleConfig, 0),
	}

	for _, rule := range rules {
		// 只返回该节点相关的规则
		if rule.RelayNodeID != nodeID && rule.ExitNodeID != nodeID {
			continue
		}

		rc := ForwardRuleConfig{
			RuleID:     rule.ID,
			ListenPort: rule.ListenPort,
			Protocol:   rule.Protocol,
			TargetHost: rule.TargetHost,
			TargetPort: rule.TargetPort,
		}

		if rule.SpeedLimit != nil {
			rc.SpeedLimit = *rule.SpeedLimit
		}

		config.Rules = append(config.Rules, rc)
	}

	return config, nil
}

// NodeForwardConfig 节点转发配置
type NodeForwardConfig struct {
	NodeID uint                 `json:"node_id"`
	Rules  []ForwardRuleConfig  `json:"rules"`
}

// ForwardRuleConfig 转发规则配置
type ForwardRuleConfig struct {
	RuleID     uint   `json:"rule_id"`
	ListenPort int    `json:"listen_port"`
	Protocol   string `json:"protocol"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	SpeedLimit int64  `json:"speed_limit,omitempty"`
}

// CheckIPAllowed 检查IP是否允许访问
func (s *ForwardRuleService) CheckIPAllowed(rule *model.ForwardRule, ip string) bool {
	// 检查是否需要验证用户
	if rule.UserID != nil {
		// TODO: 实现IP白名单检查
		return true
	}
	return true
}

// ParseIPRange 解析IP范围
func ParseIPRange(r string) ([]net.IP, error) {
	// 支持 CIDR 格式: 192.168.1.0/24
	// 支持范围格式: 192.168.1.1-192.168.1.100
	// 支持单个IP: 192.168.1.1

	_, ipnet, err := net.ParseCIDR(r)
	if err == nil {
		// CIDR 格式
		var ips []net.IP
		for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			ips = append(ips, net.IP(make([]byte, len(ip))))
			copy(ips[len(ips)-1], ip)
		}
		return ips, nil
	}

	// 单个IP
	ip := net.ParseIP(r)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	return nil, fmt.Errorf("invalid IP range: %s", r)
}

// inc IP自增
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}