package gost

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// Manager gost 转发管理器
type Manager struct {
	db      *gorm.DB
	clients sync.Map // nodeID -> *Client
	mu      sync.RWMutex
}

// NewManager 创建管理器
func NewManager(db *gorm.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// GetClient 获取节点的 gost 客户端
func (m *Manager) GetClient(nodeID uint) (*Client, error) {
	if client, ok := m.clients.Load(nodeID); ok {
		return client.(*Client), nil
	}

	// 从数据库加载节点信息
	var node model.ForwardNode
	if err := m.db.First(&node, nodeID).Error; err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	return m.createClient(&node)
}

// createClient 创建客户端并缓存
func (m *Manager) createClient(node *model.ForwardNode) (*Client, error) {
	if node.APIPort == 0 {
		return nil, fmt.Errorf("node API port not configured")
	}

	client := NewClient(&Config{
		Host:     fmt.Sprintf("http://%s:%d", node.Host, node.APIPort),
		APIToken: node.APIToken,
	})

	m.clients.Store(node.ID, client)
	return client, nil
}

// RegisterNode 注册节点（创建客户端）
func (m *Manager) RegisterNode(node *model.ForwardNode) error {
	_, err := m.createClient(node)
	return err
}

// UnregisterNode 注销节点（移除客户端）
func (m *Manager) UnregisterNode(nodeID uint) {
	m.clients.Delete(nodeID)
}

// CreateForwardRule 创建转发规则
// 对于中转节点(relay): 创建监听服务
// 对于落地节点(exit): 需要在中转节点上配置转发链
func (m *Manager) CreateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	// 获取中转节点客户端
	relayClient, err := m.GetClient(rule.RelayNodeID)
	if err != nil {
		return fmt.Errorf("get relay node client: %w", err)
	}

	// 获取落地节点信息
	var exitNode model.ForwardNode
	if err := m.db.First(&exitNode, rule.ExitNodeID).Error; err != nil {
		return fmt.Errorf("exit node not found: %w", err)
	}

	// 构建服务名称（使用规则 ID 作为唯一标识）
	serviceName := m.getServiceName(rule)

	// 构建服务配置
	svc := &ServiceConfig{
		Name: serviceName,
		Addr: ":" + strconv.Itoa(rule.ListenPort),
		Handler: &HandlerConfig{
			Type: "tcp", // 根据 protocol 选择 tcp/udp
		},
		Listener: &ListenerConfig{
			Type: "tcp",
		},
		Forwarder: &ForwarderConfig{
			Nodes: []ForwarderNode{
				{
					Name: fmt.Sprintf("target-%d", rule.ID),
					Addr: fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort),
				},
			},
			Selector: &SelectorConfig{
				Strategy:    "round",
				MaxFails:    3,
				FailTimeout: "30s",
			},
		},
	}

	// 根据 protocol 设置类型
	switch rule.Protocol {
	case "udp":
		svc.Handler.Type = "udp"
		svc.Listener.Type = "udp"
	case "both":
		// 需要创建两个服务 (TCP + UDP)
		if err := relayClient.CreateService(ctx, svc); err != nil {
			return fmt.Errorf("create tcp service: %w", err)
		}
		// 创建 UDP 服务
		udpSvc := *svc
		udpSvc.Name = serviceName + "-udp"
		udpSvc.Handler.Type = "udp"
		udpSvc.Listener.Type = "udp"
		if err := relayClient.CreateService(ctx, &udpSvc); err != nil {
			return fmt.Errorf("create udp service: %w", err)
		}
		return nil
	}

	return relayClient.CreateService(ctx, svc)
}

// UpdateForwardRule 更新转发规则
func (m *Manager) UpdateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	// 先删除旧规则
	if err := m.DeleteForwardRule(ctx, rule); err != nil {
		// 忽略不存在的错误
	}

	// 创建新规则
	return m.CreateForwardRule(ctx, rule)
}

// DeleteForwardRule 删除转发规则
func (m *Manager) DeleteForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	relayClient, err := m.GetClient(rule.RelayNodeID)
	if err != nil {
		return fmt.Errorf("get relay node client: %w", err)
	}

	serviceName := m.getServiceName(rule)

	// 删除 TCP 服务
	if err := relayClient.DeleteService(ctx, serviceName); err != nil {
		// 忽略不存在的错误
	}

	// 删除 UDP 服务（如果存在）
	if rule.Protocol == "both" || rule.Protocol == "udp" {
		relayClient.DeleteService(ctx, serviceName+"-udp")
	}

	return nil
}

// getServiceName 生成服务名称
func (m *Manager) getServiceName(rule *model.ForwardRule) string {
	return fmt.Sprintf("forward-rule-%d", rule.ID)
}

// SyncRuleToGost 同步规则到 gost（用于规则状态变更）
func (m *Manager) SyncRuleToGost(ctx context.Context, rule *model.ForwardRule) error {
	if rule.Enabled {
		return m.CreateForwardRule(ctx, rule)
	}
	return m.DeleteForwardRule(ctx, rule)
}

// GetNodeStats 获取节点统计信息
func (m *Manager) GetNodeStats(ctx context.Context, nodeID uint) (*StatsResponse, error) {
	client, err := m.GetClient(nodeID)
	if err != nil {
		return nil, err
	}

	return client.GetStats(ctx)
}

// GetRuleStats 获取规则统计信息
func (m *Manager) GetRuleStats(ctx context.Context, rule *model.ForwardRule) (*ServiceStats, error) {
	client, err := m.GetClient(rule.RelayNodeID)
	if err != nil {
		return nil, err
	}

	serviceName := m.getServiceName(rule)
	return client.GetServiceStats(ctx, serviceName)
}

// CheckNodeHealth 检查节点健康状态
func (m *Manager) CheckNodeHealth(ctx context.Context, nodeID uint) error {
	client, err := m.GetClient(nodeID)
	if err != nil {
		return err
	}

	return client.HealthCheck(ctx)
}

// CreateChain 创建转发链（中转 -> 落地）
func (m *Manager) CreateChain(ctx context.Context, relayNodeID, exitNodeID uint, chainName string) error {
	// 获取节点信息
	var relayNode, exitNode model.ForwardNode
	if err := m.db.First(&relayNode, relayNodeID).Error; err != nil {
		return fmt.Errorf("relay node not found: %w", err)
	}
	if err := m.db.First(&exitNode, exitNodeID).Error; err != nil {
		return fmt.Errorf("exit node not found: %w", err)
	}

	// 在中转节点上创建转发链
	relayClient, err := m.GetClient(relayNodeID)
	if err != nil {
		return err
	}

	// 创建跳点（指向落地节点）
	hopName := fmt.Sprintf("hop-%s", chainName)
	hop := &HopConfig{
		Name: hopName,
		Nodes: []HopNode{
			{
				Name: fmt.Sprintf("node-%s", chainName),
				Addr: fmt.Sprintf("%s:%d", exitNode.Host, exitNode.Port),
				Connector: &ConnectorConfig{
					Type: "socks5", // 或其他协议
				},
				Dialer: &DialerConfig{
					Type: "tcp",
				},
			},
		},
	}

	if err := relayClient.CreateHop(ctx, hop); err != nil {
		return fmt.Errorf("create hop: %w", err)
	}

	// 创建转发链
	chain := &ChainConfig{
		Name: chainName,
		Hops: []string{hopName},
	}

	return relayClient.CreateChain(ctx, chain)
}

// SyncAllRules 同步所有启用的规则到 gost
func (m *Manager) SyncAllRules(ctx context.Context) error {
	var rules []*model.ForwardRule
	if err := m.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return err
	}

	for _, rule := range rules {
		if err := m.CreateForwardRule(ctx, rule); err != nil {
			// 记录错误但继续同步其他规则
			fmt.Printf("sync rule %d failed: %v\n", rule.ID, err)
		}
	}

	return nil
}