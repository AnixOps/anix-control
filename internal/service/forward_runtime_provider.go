package service

import (
	"context"

	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// ForwardRuntimeProvider abstracts the runtime backend used by forward rule sync.
// The default implementation remains gost-backed to preserve current behavior.
type ForwardRuntimeProvider interface {
	CreateForwardRule(ctx context.Context, rule *model.ForwardRule) error
	UpdateForwardRule(ctx context.Context, rule *model.ForwardRule) error
	DeleteForwardRule(ctx context.Context, rule *model.ForwardRule) error
	SyncForwardRule(ctx context.Context, rule *model.ForwardRule) error
}

// NewForwardRuntimeProvider returns the default runtime provider.
func NewForwardRuntimeProvider(db *gorm.DB) ForwardRuntimeProvider {
	return NewGostForwardRuntimeProvider(db)
}

// NewGostForwardRuntimeProvider creates a gost-backed runtime provider.
func NewGostForwardRuntimeProvider(db *gorm.DB) ForwardRuntimeProvider {
	return NewGostForwardRuntimeProviderWithManager(gost.NewManager(db))
}

// NewGostForwardRuntimeProviderWithManager allows future injection of a custom manager.
func NewGostForwardRuntimeProviderWithManager(manager *gost.Manager) ForwardRuntimeProvider {
	return &gostForwardRuntimeProvider{manager: manager}
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
