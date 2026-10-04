package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// ForwardNodeService 转发节点服务
type ForwardNodeService struct {
	db *gorm.DB
}

// NewForwardNodeService 创建服务
func NewForwardNodeService(db *gorm.DB) *ForwardNodeService {
	return &ForwardNodeService{db: db}
}

// Create 创建节点. The node's token is written to the split tables in the
// same transaction (nodesecrets.Sync), as by Update and Delete.
func (s *ForwardNodeService) Create(node *model.ForwardNode) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(node).Error; err != nil {
			return err
		}
		return nodesecrets.Sync(tx, nodesecrets.TableForwardNode, node.ID)
	})
}

// Update 更新节点. A changed token or a disabled node revokes the node's
// agent certificates.
func (s *ForwardNodeService) Update(node *model.ForwardNode) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		reason, err := forwardNodeAgentRevocation(tx, node)
		if err != nil {
			return err
		}
		if err := tx.Save(node).Error; err != nil {
			return err
		}
		if err := nodesecrets.Sync(tx, nodesecrets.TableForwardNode, node.ID); err != nil {
			return err
		}
		if reason == "" {
			return nil
		}
		return revokeNodeAgents(tx, agentcontrol.NodeKindForward, node.ID, reason)
	})
}

// Delete 删除节点 and revoke its agent certificates. The kernel's part, the
// token's split row and the certificates, is RetireForwardNodeTx, which
// RetireNode runs too; the node row goes with it in the same transaction.
func (s *ForwardNodeService) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if _, err := RetireForwardNodeTx(tx, id); err != nil {
			return err
		}
		if err := tx.Delete(&model.ForwardNode{}, id).Error; err != nil {
			return err
		}
		return nodesecrets.Sync(tx, nodesecrets.TableForwardNode, id)
	})
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

// HealthCheck 健康检查: the forward node check, from Control, recording what
// it found (CheckEndpoint).
func (s *ForwardNodeService) HealthCheck(ctx context.Context, nodeID uint) (*HealthCheckResult, error) {
	return s.CheckEndpoint(ctx, DiagnosisProbes{}, nodeID, true)
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	NodeID    uint      `json:"node_id"`
	Status    int       `json:"status"`
	Latency   int64     `json:"latency"`
	CheckTime time.Time `json:"check_time"`
	Error     string    `json:"error,omitempty"`
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
