package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// Create creates a rule and syncs it to the runtime when enabled.
func (s *ForwardRuleService) Create(rule *model.ForwardRule) error {
	if err := s.validateRule(rule); err != nil {
		return err
	}

	if err := s.db.Create(rule).Error; err != nil {
		return err
	}

	if rule.Enabled {
		ctx := context.Background()
		if err := s.runtimeProvider.CreateForwardRule(ctx, rule); err != nil {
			// Keep DB persistence compatible with current behavior even if runtime sync fails.
			fmt.Printf("sync rule %d failed: %v\n", rule.ID, err)
		}
	}

	return nil
}

// Update updates a rule and syncs it to the runtime backend.
func (s *ForwardRuleService) Update(rule *model.ForwardRule) error {
	if err := s.validateRule(rule); err != nil {
		return err
	}

	// The rule may carry preloaded associations (RelayNode/ExitNode/User).
	// Persist only rule fields, otherwise GORM may upsert stale associations
	// and overwrite updated foreign keys with old relation IDs.
	if err := s.db.Omit(clause.Associations).Save(rule).Error; err != nil {
		return err
	}

	ctx := context.Background()
	if err := s.runtimeProvider.UpdateForwardRule(ctx, rule); err != nil {
		fmt.Printf("sync rule %d failed: %v\n", rule.ID, err)
	}

	return nil
}

// Delete deletes a rule from the runtime backend and then removes it from the database.
func (s *ForwardRuleService) Delete(id uint) error {
	rule, err := s.GetByID(id)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := s.runtimeProvider.DeleteForwardRule(ctx, rule); err != nil {
		fmt.Printf("delete rule %d failed: %v\n", id, err)
	}

	return s.db.Delete(&model.ForwardRule{}, id).Error
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

// List fetches a paginated rule list.
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

// Toggle updates the enabled state and syncs it to the runtime backend.
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

	ctx := context.Background()
	if err := s.runtimeProvider.SyncForwardRule(ctx, rule); err != nil {
		fmt.Printf("sync rule %d failed: %v\n", id, err)
	}

	return nil
}

// validateRule validates rule topology and port uniqueness.
func (s *ForwardRuleService) validateRule(rule *model.ForwardRule) error {
	relayNode, err := s.nodeService.GetByID(rule.RelayNodeID)
	if err != nil {
		return fmt.Errorf("relay node not found")
	}
	if relayNode.Type != model.ForwardNodeTypeRelay {
		return fmt.Errorf("node is not a relay node")
	}

	exitNode, err := s.nodeService.GetByID(rule.ExitNodeID)
	if err != nil {
		return fmt.Errorf("exit node not found")
	}
	if exitNode.Type != model.ForwardNodeTypeExit {
		return fmt.Errorf("node is not an exit node")
	}

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

// GetFreePort finds a free relay port in the given range.
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

// getUsedPorts returns the used ports for a relay node.
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

// ErrForwardRuleAdminOnly refuses a user's write of a legacy forward rule.
var ErrForwardRuleAdminOnly = errors.New("only administrators can create or change legacy forward rules; forward through your tunnels instead")

// CreateRuleForUser creates a rule owned by the caller, who must be an
// administrator (ErrForwardRuleAdminOnly).
//
// Legacy rules are administrator-only. A rule runs on the relay and exit
// nodes it names, and no user entitlement covers them: users are entitled
// to tunnels (v2_forward_user_tunnel), and a rule is neither counted in a
// tunnel permission's forward or traffic quota nor paused when the
// permission is disabled, expires or is removed. Users forward through
// their tunnels (PanelForwardService.CreateForward).
func (s *ForwardRuleService) CreateRuleForUser(userID uint, isAdmin bool, req *CreateRuleRequest) (*model.ForwardRule, error) {
	if !isAdmin {
		return nil, ErrForwardRuleAdminOnly
	}
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
