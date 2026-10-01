package gost

import (
	"context"
	"fmt"
	"sync"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// Manager gost 转发管理器
type Manager struct {
	db      *gorm.DB
	clients sync.Map // nodeID -> *Client
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

	var metricsHost string
	if node.MetricsPort > 0 {
		metricsHost = fmt.Sprintf("http://%s:%d", node.Host, node.MetricsPort)
	}

	client := NewClient(&Config{
		Host:        fmt.Sprintf("http://%s:%d", node.Host, node.APIPort),
		MetricsHost: metricsHost,
		APIToken:    nodesecrets.ForwardNodeToken(m.db, node),
	})

	m.clients.Store(node.ID, client)
	return client, nil
}

// GetNodeTrafficTotals 通过节点的 Prometheus /metrics 端点汇总该节点上所有 service 的流量，
// 用于节点级"同步统计"，替代已失效的 GetNodeStats(REST /api/stats)。
func (m *Manager) GetNodeTrafficTotals(ctx context.Context, nodeID uint) (map[string]*ServiceTrafficTotals, error) {
	client, err := m.GetClient(nodeID)
	if err != nil {
		return nil, err
	}
	if !client.HasMetricsEndpoint() {
		return nil, fmt.Errorf("metrics endpoint not configured for node %d", nodeID)
	}

	return client.GetAllServiceTrafficTotals(ctx)
}
