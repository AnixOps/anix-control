package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// ForwardRuleService handles forward rule persistence and runtime sync.
type ForwardRuleService struct {
	db              *gorm.DB
	nodeService     *ForwardNodeService
	runtimeProvider ForwardRuntimeProvider
}

// NewForwardRuleService preserves the existing constructor shape and uses the default runtime provider.
func NewForwardRuleService(db *gorm.DB, nodeService *ForwardNodeService) *ForwardRuleService {
	return NewForwardRuleServiceWithProvider(db, nodeService, NewForwardRuntimeProvider(db))
}

// NewForwardRuleServiceWithProvider allows future runtime backends to be injected explicitly.
func NewForwardRuleServiceWithProvider(db *gorm.DB, nodeService *ForwardNodeService, provider ForwardRuntimeProvider) *ForwardRuleService {
	return &ForwardRuleService{
		db:              db,
		nodeService:     nodeService,
		runtimeProvider: provider,
	}
}

// ApplyRuntime pushes a rule to its relay and exit nodes through the
// runtime provider (NodeX): create, update or sync apply the row as it is,
// delete removes it. The legacy routes and the KernelNodeOps
// forward.legacy_rule executor run it alike; the row is the caller's.
func (s *ForwardRuleService) ApplyRuntime(ctx context.Context, rule *model.ForwardRule, action string) error {
	if rule == nil {
		return errors.New("forward rule is required")
	}
	switch action {
	case model.ForwardRuntimeJobActionCreate:
		return s.runtimeProvider.CreateForwardRule(ctx, rule)
	case model.ForwardRuntimeJobActionUpdate:
		return s.runtimeProvider.UpdateForwardRule(ctx, rule)
	case model.ForwardRuntimeJobActionDelete:
		return s.runtimeProvider.DeleteForwardRule(ctx, rule)
	case model.ForwardRuntimeJobActionSync, model.ForwardRuntimeJobActionPause, model.ForwardRuntimeJobActionResume:
		return s.runtimeProvider.SyncForwardRule(ctx, rule)
	}
	return fmt.Errorf("unsupported forward rule action: %s", action)
}

// GetByID fetches a rule by ID.
func (s *ForwardRuleService) GetByID(id uint) (*model.ForwardRule, error) {
	var rule model.ForwardRule
	err := s.db.Preload("RelayNode").Preload("ExitNode").First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetUserRules fetches rules owned by a user, for the user. Their relay and
// exit nodes are shown without the nodes' API tokens: a node's token
// authenticates its agent (verifyForwardNodeToken), so a user who saw it
// could act as the node.
func (s *ForwardRuleService) GetUserRules(userID uint) ([]*model.ForwardRule, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("user_id = ?", userID).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules).Error
	for _, rule := range rules {
		for _, node := range []*model.ForwardNode{rule.RelayNode, rule.ExitNode} {
			if node != nil {
				node.APIToken = ""
			}
		}
	}
	return rules, err
}

// ErrForwardRuleAdminOnly refuses a user's write of a legacy forward rule.
var ErrForwardRuleAdminOnly = errors.New("only administrators can create or change legacy forward rules; forward through your tunnels instead")

// CreateRuleRequest is the request payload for user-owned rule creation.
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
