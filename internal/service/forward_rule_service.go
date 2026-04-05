package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ForwardRuleService handles forward rule persistence and runtime sync.
type ForwardRuleService struct {
	db              *gorm.DB
	nodeService     *ForwardNodeService
	runtimeProvider ForwardRuntimeProvider
	mu              sync.RWMutex
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

// GetEnabledRules fetches enabled rules.
func (s *ForwardRuleService) GetEnabledRules() ([]*model.ForwardRule, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("enabled = ?", true).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules).Error
	return rules, err
}

// GetUserRules fetches rules owned by a user.
func (s *ForwardRuleService) GetUserRules(userID uint) ([]*model.ForwardRule, error) {
	var rules []*model.ForwardRule
	err := s.db.Where("user_id = ?", userID).
		Preload("RelayNode").
		Preload("ExitNode").
		Find(&rules).Error
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

// MatchRule matches an incoming request against enabled rules.
func (s *ForwardRuleService) MatchRule(sourceIP string, targetHost string, targetPort int) (*model.ForwardRule, error) {
	rules, err := s.GetEnabledRules()
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if rule.TargetPort != targetPort {
			continue
		}

		if rule.TargetHost != targetHost && rule.TargetHost != "0.0.0.0" {
			continue
		}

		if rule.UserID != nil {
			// TODO: implement user validation for user-bound rules.
			continue
		}

		if rule.ExpireTime != nil && rule.ExpireTime.Before(time.Now()) {
			continue
		}

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

// UpdateTraffic updates traffic counters.
func (s *ForwardRuleService) UpdateTraffic(ruleID uint, upload, download int64) error {
	return s.db.Model(&model.ForwardRule{}).Where("id = ?", ruleID).Updates(map[string]interface{}{
		"upload":   gorm.Expr("upload + ?", upload),
		"download": gorm.Expr("download + ?", download),
	}).Error
}

// UpdateConnections updates connection counters.
func (s *ForwardRuleService) UpdateConnections(ruleID uint, delta int) error {
	return s.db.Model(&model.ForwardRule{}).Where("id = ?", ruleID).Updates(map[string]interface{}{
		"connections": gorm.Expr("connections + ?", delta),
		"total_conns": gorm.Expr("total_conns + ?", max(delta, 0)),
	}).Error
}

// GetPortMapping returns the port-to-rule mapping for a relay node.
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

// GetTrafficStats returns traffic stats for a rule over a time range.
func (s *ForwardRuleService) GetTrafficStats(ruleID uint, start, end time.Time) ([]*model.ForwardStats, error) {
	var stats []*model.ForwardStats
	err := s.db.Where("rule_id = ? AND date >= ? AND date <= ?", ruleID, start, end).
		Order("date ASC, hour ASC").
		Find(&stats).Error
	return stats, err
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

// CheckPortAvailable checks whether a relay port is free.
func (s *ForwardRuleService) CheckPortAvailable(relayNodeID uint, port int) (bool, error) {
	var count int64
	s.db.Model(&model.ForwardRule{}).
		Where("relay_node_id = ? AND listen_port = ?", relayNodeID, port).
		Count(&count)
	return count == 0, nil
}

// ValidateUserRule validates whether a user can access a rule.
func (s *ForwardRuleService) ValidateUserRule(userID, ruleID uint) (bool, error) {
	rule, err := s.GetByID(ruleID)
	if err != nil {
		return false, err
	}

	if rule.UserID == nil {
		return true, nil
	}

	return *rule.UserID == userID, nil
}

// CreateRuleForUser creates a user-owned rule.
func (s *ForwardRuleService) CreateRuleForUser(userID uint, req *CreateRuleRequest) (*model.ForwardRule, error) {
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

// GetConfigForNode returns runtime config that should be pushed to a node.
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

// NodeForwardConfig is the rule bundle for a node.
type NodeForwardConfig struct {
	NodeID uint                `json:"node_id"`
	Rules  []ForwardRuleConfig `json:"rules"`
}

// ForwardRuleConfig is the node-side rule payload.
type ForwardRuleConfig struct {
	RuleID     uint   `json:"rule_id"`
	ListenPort int    `json:"listen_port"`
	Protocol   string `json:"protocol"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	SpeedLimit int64  `json:"speed_limit,omitempty"`
}

// CheckIPAllowed checks whether an IP is allowed to access the rule.
func (s *ForwardRuleService) CheckIPAllowed(rule *model.ForwardRule, ip string) bool {
	if rule.UserID != nil {
		// TODO: implement IP allowlist checks.
		return true
	}
	return true
}

// ParseIPRange parses CIDR or single-IP input.
func ParseIPRange(r string) ([]net.IP, error) {
	_, ipnet, err := net.ParseCIDR(r)
	if err == nil {
		var ips []net.IP
		for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
			ips = append(ips, net.IP(make([]byte, len(ip))))
			copy(ips[len(ips)-1], ip)
		}
		return ips, nil
	}

	ip := net.ParseIP(r)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	return nil, fmt.Errorf("invalid IP range: %s", r)
}

// inc increments an IP address in place.
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
