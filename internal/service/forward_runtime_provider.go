package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// ForwardRuntimeProvider abstracts the runtime backend used by forward rule sync.
type ForwardRuntimeProvider interface {
	CreateForwardRule(ctx context.Context, rule *model.ForwardRule) error
	UpdateForwardRule(ctx context.Context, rule *model.ForwardRule) error
	DeleteForwardRule(ctx context.Context, rule *model.ForwardRule) error
	SyncForwardRule(ctx context.Context, rule *model.ForwardRule) error
}

// NewForwardRuntimeProvider returns the default runtime provider.
func NewForwardRuntimeProvider(db *gorm.DB) ForwardRuntimeProvider {
	configService := NewSystemConfigService(db)
	return &nodeXForwardRuntimeProvider{
		db:     db,
		client: newNodeXForwardRuntimeClient(configService),
	}
}

// NewGostForwardRuntimeProvider creates a gost-backed runtime provider.
func NewGostForwardRuntimeProvider(db *gorm.DB) ForwardRuntimeProvider {
	return NewGostForwardRuntimeProviderWithManager(gost.NewManager(db))
}

// NewGostForwardRuntimeProviderWithManager allows future injection of a custom manager.
func NewGostForwardRuntimeProviderWithManager(manager *gost.Manager) ForwardRuntimeProvider {
	return &gostForwardRuntimeProvider{manager: manager}
}

type nodeXForwardRuntimeProvider struct {
	db     *gorm.DB
	client forwardRuntimeNodeXExecutor
}

func (p *nodeXForwardRuntimeProvider) CreateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.execute(ctx, model.ForwardRuntimeJobActionCreate, rule)
}

func (p *nodeXForwardRuntimeProvider) UpdateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.execute(ctx, model.ForwardRuntimeJobActionUpdate, rule)
}

func (p *nodeXForwardRuntimeProvider) DeleteForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.execute(ctx, model.ForwardRuntimeJobActionDelete, rule)
}

func (p *nodeXForwardRuntimeProvider) SyncForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.execute(ctx, model.ForwardRuntimeJobActionSync, rule)
}

func (p *nodeXForwardRuntimeProvider) execute(ctx context.Context, action string, rule *model.ForwardRule) error {
	if rule == nil {
		return errors.New("forward rule is required")
	}
	if p.client == nil {
		return errors.New("forward runtime executor is required")
	}
	req, err := p.buildRequest(action, rule)
	if err != nil {
		return err
	}
	_, err = p.client.Execute(ctx, req)
	return err
}

func (p *nodeXForwardRuntimeProvider) buildRequest(action string, rule *model.ForwardRule) (nodeXForwardExecuteRequest, error) {
	relayNode, err := p.loadNode(rule.RelayNode, rule.RelayNodeID)
	if err != nil {
		return nodeXForwardExecuteRequest{}, fmt.Errorf("load relay node: %w", err)
	}
	exitNode, err := p.loadNode(rule.ExitNode, rule.ExitNodeID)
	if err != nil {
		return nodeXForwardExecuteRequest{}, fmt.Errorf("load exit node: %w", err)
	}

	return nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypeLegacyRule,
		Backend:      model.ForwardRuntimeBackendGost,
		Action:       action,
		LegacyRule: &nodeXLegacyForwardRuleInput{
			Rule: nodeXLegacyForwardRulePayload{
				ID:         rule.ID,
				Name:       rule.Name,
				ListenPort: rule.ListenPort,
				Protocol:   normalizePanelRuntimeProtocol(rule.Protocol),
				TargetHost: rule.TargetHost,
				TargetPort: rule.TargetPort,
				Enabled:    rule.Enabled,
			},
			RelayNode: mapForwardNodeToNodeXPayload(relayNode),
			ExitNode:  mapForwardNodeToNodeXPayload(exitNode),
		},
	}, nil
}

func (p *nodeXForwardRuntimeProvider) loadNode(preloaded *model.ForwardNode, id uint) (*model.ForwardNode, error) {
	if preloaded != nil && preloaded.ID != 0 {
		return preloaded, nil
	}

	var node model.ForwardNode
	if err := p.db.First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("node %d not found", id)
		}
		return nil, err
	}
	return &node, nil
}

func mapForwardNodeToNodeXPayload(node *model.ForwardNode) nodeXForwardNodePayload {
	if node == nil {
		return nodeXForwardNodePayload{}
	}
	return nodeXForwardNodePayload{
		ID:       node.ID,
		Name:     node.Name,
		Host:     node.Host,
		Port:     node.Port,
		APIPort:  node.APIPort,
		APIToken: node.APIToken,
	}
}

type gostForwardRuntimeProvider struct {
	manager *gost.Manager
}

func (p *gostForwardRuntimeProvider) CreateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.manager.CreateForwardRule(ctx, rule)
}

func (p *gostForwardRuntimeProvider) UpdateForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.manager.UpdateForwardRule(ctx, rule)
}

func (p *gostForwardRuntimeProvider) DeleteForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.manager.DeleteForwardRule(ctx, rule)
}

func (p *gostForwardRuntimeProvider) SyncForwardRule(ctx context.Context, rule *model.ForwardRule) error {
	return p.manager.SyncRuleToGost(ctx, rule)
}
