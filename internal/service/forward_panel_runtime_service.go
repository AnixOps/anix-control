package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anixops/v2board/internal/gost"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	forwardRuntimeConfigGroup                    = "forward"
	forwardRuntimeBackendConfigKey               = "forward.runtime_backend"
	forwardRuntimeAnsibleConfigJSONKey           = "forward.runtime.iptables_ansible.config"
	forwardRuntimeAnsibleInventoryConfigKey      = "forward.ansible.inventory"
	forwardRuntimeAnsibleApplyPlaybookConfigKey  = "forward.ansible.playbook_apply"
	forwardRuntimeAnsibleRemovePlaybookConfigKey = "forward.ansible.playbook_remove"
	forwardRuntimeAnsibleBecomeConfigKey         = "forward.ansible.become"
	forwardRuntimeAnsibleExtraVarsConfigKey      = "forward.ansible.extra_vars_json"
)

type panelForwardRuntimeResult struct {
	Backend string
	Status  int
	Message string
	Async   bool
}

type panelForwardRuntimeProvider interface {
	Apply(ctx context.Context, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardRuntimeResult, error)
}

type PanelForwardRuntimeService struct {
	db              *gorm.DB
	configService   *SystemConfigService
	gostProvider    panelForwardRuntimeProvider
	ansibleProvider panelForwardRuntimeProvider
}

func NewPanelForwardRuntimeService(db *gorm.DB) *PanelForwardRuntimeService {
	return &PanelForwardRuntimeService{
		db:              db,
		configService:   NewSystemConfigService(db),
		gostProvider:    &panelForwardGostRuntimeProvider{db: db, manager: gost.NewManager(db)},
		ansibleProvider: &panelForwardAnsibleRuntimeProvider{db: db, configService: NewSystemConfigService(db)},
	}
}

func (s *PanelForwardRuntimeService) Apply(ctx context.Context, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardRuntimeResult, error) {
	backend, err := s.resolveBackend()
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendGost,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	var provider panelForwardRuntimeProvider
	switch backend {
	case model.ForwardRuntimeBackendGost:
		provider = s.gostProvider
	case model.ForwardRuntimeBackendIptablesAnsible:
		provider = s.ansibleProvider
	default:
		return &panelForwardRuntimeResult{
			Backend: backend,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: fmt.Sprintf("unsupported forward runtime backend: %s", backend),
		}, fmt.Errorf("unsupported forward runtime backend: %s", backend)
	}

	result, applyErr := provider.Apply(ctx, action, forward, tunnel)
	if result == nil {
		result = &panelForwardRuntimeResult{
			Backend: backend,
		}
	}
	if result.Backend == "" {
		result.Backend = backend
	}
	if applyErr != nil {
		result.Status = model.ForwardRuntimeJobStatusFailed
		if strings.TrimSpace(result.Message) == "" {
			result.Message = applyErr.Error()
		}
	}

	return result, applyErr
}

func (s *PanelForwardRuntimeService) resolveBackend() (string, error) {
	value, err := s.configService.Get(forwardRuntimeBackendConfigKey)
	if err != nil {
		return "", err
	}

	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", model.ForwardRuntimeBackendGost:
		return model.ForwardRuntimeBackendGost, nil
	case model.ForwardRuntimeBackendIptablesAnsible:
		return model.ForwardRuntimeBackendIptablesAnsible, nil
	default:
		return "", fmt.Errorf("invalid %s value: %s", forwardRuntimeBackendConfigKey, value)
	}
}

type panelForwardGostRuntimeProvider struct {
	db      *gorm.DB
	manager *gost.Manager
}

func (p *panelForwardGostRuntimeProvider) Apply(ctx context.Context, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardRuntimeResult, error) {
	if tunnel != nil && tunnel.InNodeID == 0 && (action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause) {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendGost,
			Status:  model.ForwardRuntimeJobStatusSuccess,
			Message: "gost runtime skipped because ingress node is not configured",
		}, nil
	}

	client, err := p.getIngressClient(tunnel)
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendGost,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	services, err := buildPanelForwardGostServices(forward, tunnel)
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendGost,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	switch action {
	case model.ForwardRuntimeJobActionCreate, model.ForwardRuntimeJobActionResume, model.ForwardRuntimeJobActionSync:
		err = p.createServices(ctx, client, services)
	case model.ForwardRuntimeJobActionUpdate:
		if deleteErr := p.deleteServices(ctx, client, services); deleteErr != nil && !isGostMissingResource(deleteErr) {
			return &panelForwardRuntimeResult{
				Backend: model.ForwardRuntimeBackendGost,
				Status:  model.ForwardRuntimeJobStatusFailed,
				Message: deleteErr.Error(),
			}, deleteErr
		}
		err = p.createServices(ctx, client, services)
	case model.ForwardRuntimeJobActionPause, model.ForwardRuntimeJobActionDelete:
		err = p.deleteServices(ctx, client, services)
	default:
		err = fmt.Errorf("unsupported forward runtime action: %s", action)
	}
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendGost,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	message := "gost runtime synchronized"
	if action == model.ForwardRuntimeJobActionPause || action == model.ForwardRuntimeJobActionDelete {
		message = "gost runtime removed"
	}

	return &panelForwardRuntimeResult{
		Backend: model.ForwardRuntimeBackendGost,
		Status:  model.ForwardRuntimeJobStatusSuccess,
		Message: message,
		Async:   false,
	}, nil
}

func (p *panelForwardGostRuntimeProvider) getIngressClient(tunnel *model.ForwardTunnel) (*gost.Client, error) {
	if tunnel == nil {
		return nil, errors.New("forward tunnel is required")
	}
	if tunnel.InNodeID == 0 {
		return nil, errors.New("forward tunnel ingress node is not configured")
	}
	return p.manager.GetClient(tunnel.InNodeID)
}

func (p *panelForwardGostRuntimeProvider) createServices(ctx context.Context, client *gost.Client, services []panelForwardGostService) error {
	for _, service := range services {
		if err := client.CreateService(ctx, service.Config); err != nil {
			return fmt.Errorf("create gost service %s: %w", service.Name, err)
		}
	}
	return nil
}

func (p *panelForwardGostRuntimeProvider) deleteServices(ctx context.Context, client *gost.Client, services []panelForwardGostService) error {
	var errs []string
	for _, service := range services {
		if err := client.DeleteService(ctx, service.Name); err != nil && !isGostMissingResource(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", service.Name, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

type panelForwardGostService struct {
	Name   string
	Config *gost.ServiceConfig
}

func buildPanelForwardGostServices(forward *model.Forward, tunnel *model.ForwardTunnel) ([]panelForwardGostService, error) {
	if forward == nil {
		return nil, errors.New("forward is required")
	}
	if tunnel == nil {
		return nil, errors.New("forward tunnel is required")
	}

	targets, err := buildPanelForwardGostTargets(forward.RemoteAddr)
	if err != nil {
		return nil, err
	}

	interfaceName := strings.TrimSpace(forward.InterfaceName)
	if interfaceName == "" {
		interfaceName = strings.TrimSpace(tunnel.InterfaceName)
	}

	protocol := normalizePanelRuntimeProtocol(tunnel.Protocol)
	services := make([]panelForwardGostService, 0, 2)
	switch protocol {
	case "udp":
		services = append(services, panelForwardGostService{
			Name:   panelForwardGostServiceName(forward.ID, "udp"),
			Config: newPanelForwardGostServiceConfig(forward, tunnel, "udp", interfaceName, targets),
		})
	case "both":
		services = append(services,
			panelForwardGostService{
				Name:   panelForwardGostServiceName(forward.ID, "tcp"),
				Config: newPanelForwardGostServiceConfig(forward, tunnel, "tcp", interfaceName, targets),
			},
			panelForwardGostService{
				Name:   panelForwardGostServiceName(forward.ID, "udp"),
				Config: newPanelForwardGostServiceConfig(forward, tunnel, "udp", interfaceName, targets),
			},
		)
	default:
		services = append(services, panelForwardGostService{
			Name:   panelForwardGostServiceName(forward.ID, "tcp"),
			Config: newPanelForwardGostServiceConfig(forward, tunnel, "tcp", interfaceName, targets),
		})
	}

	return services, nil
}

func buildPanelForwardGostTargets(remoteAddr string) ([]gost.ForwarderNode, error) {
	rawTargets := strings.Split(normalizeRemoteAddr(remoteAddr), ",")
	targets := make([]gost.ForwarderNode, 0, len(rawTargets))
	for idx, raw := range rawTargets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		host, port, err := splitTarget(target)
		if err != nil {
			return nil, fmt.Errorf("invalid remote address %s: %w", target, err)
		}
		targets = append(targets, gost.ForwarderNode{
			Name: fmt.Sprintf("target-%d", idx+1),
			Addr: fmt.Sprintf("%s:%d", host, port),
		})
	}

	if len(targets) == 0 {
		return nil, errors.New("no valid remote target configured")
	}
	return targets, nil
}

func newPanelForwardGostServiceConfig(forward *model.Forward, tunnel *model.ForwardTunnel, protocol, interfaceName string, targets []gost.ForwarderNode) *gost.ServiceConfig {
	listenAddr := tunnel.TCPListenAddr
	if protocol == "udp" {
		listenAddr = tunnel.UDPListenAddr
	}
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}

	serviceName := panelForwardGostServiceName(forward.ID, protocol)
	config := &gost.ServiceConfig{
		Name:      serviceName,
		Addr:      fmt.Sprintf("%s:%d", listenAddr, forward.InPort),
		Interface: interfaceName,
		Handler: &gost.HandlerConfig{
			Type: protocol,
		},
		Listener: &gost.ListenerConfig{
			Type: protocol,
		},
		Forwarder: &gost.ForwarderConfig{
			Nodes: targets,
		},
		Metadata: map[string]string{
			"forward_id": fmt.Sprintf("%d", forward.ID),
			"tunnel_id":  fmt.Sprintf("%d", tunnel.ID),
			"strategy":   normalizeStrategy(forward.Strategy, forward.RemoteAddr),
		},
	}

	if selector := buildPanelForwardGostSelector(forward.Strategy, len(targets)); selector != nil {
		config.Forwarder.Selector = selector
	}

	return config
}

func buildPanelForwardGostSelector(strategy string, targetCount int) *gost.SelectorConfig {
	if targetCount <= 1 {
		return nil
	}

	switch normalizeStrategy(strategy, "") {
	case "round":
		return &gost.SelectorConfig{Strategy: "round", MaxFails: 3, FailTimeout: "30s"}
	case "rand":
		return &gost.SelectorConfig{Strategy: "rand", MaxFails: 3, FailTimeout: "30s"}
	case "hash":
		return &gost.SelectorConfig{Strategy: "hash", MaxFails: 3, FailTimeout: "30s"}
	default:
		return &gost.SelectorConfig{Strategy: "failover", MaxFails: 3, FailTimeout: "30s"}
	}
}

func panelForwardGostServiceName(forwardID uint, protocol string) string {
	if protocol == "udp" {
		return fmt.Sprintf("panel-forward-%d-udp", forwardID)
	}
	return fmt.Sprintf("panel-forward-%d", forwardID)
}

func normalizePanelRuntimeProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	case "both":
		return "both"
	default:
		return "tcp"
	}
}

func isGostMissingResource(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

type panelForwardAnsibleRuntimeProvider struct {
	db            *gorm.DB
	configService *SystemConfigService
}

type panelForwardAnsibleConfig struct {
	Inventory      string                 `json:"inventory"`
	ApplyPlaybook  string                 `json:"playbookApply"`
	RemovePlaybook string                 `json:"playbookRemove"`
	Become         bool                   `json:"become"`
	ExtraVars      map[string]interface{} `json:"extraVars"`
}

func (c *panelForwardAnsibleConfig) playbookForAction(action string) string {
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return strings.TrimSpace(c.RemovePlaybook)
	}
	return strings.TrimSpace(c.ApplyPlaybook)
}

func (c *panelForwardAnsibleConfig) ensureDefaults() {
	if c.ExtraVars == nil {
		c.ExtraVars = map[string]interface{}{}
	}
}

func (c *panelForwardAnsibleConfig) applyLegacyValues(inventory, applyPlaybook, removePlaybook string, become bool, extraVars map[string]interface{}) {
	if strings.TrimSpace(c.Inventory) == "" {
		c.Inventory = strings.TrimSpace(inventory)
	}
	if strings.TrimSpace(c.ApplyPlaybook) == "" {
		c.ApplyPlaybook = strings.TrimSpace(applyPlaybook)
	}
	if strings.TrimSpace(c.RemovePlaybook) == "" {
		c.RemovePlaybook = strings.TrimSpace(removePlaybook)
	}
	if !c.Become {
		c.Become = become
	}
	if len(c.ExtraVars) == 0 && len(extraVars) > 0 {
		c.ExtraVars = extraVars
	}
}

func (c *panelForwardAnsibleConfig) validate(action string) error {
	if strings.TrimSpace(c.Inventory) == "" {
		return fmt.Errorf("%s is required for iptables_ansible runtime", forwardRuntimeAnsibleInventoryConfigKey)
	}
	if strings.TrimSpace(c.playbookForAction(action)) == "" {
		if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
			return fmt.Errorf("%s is required for iptables_ansible runtime", forwardRuntimeAnsibleRemovePlaybookConfigKey)
		}
		return fmt.Errorf("%s is required for iptables_ansible runtime", forwardRuntimeAnsibleApplyPlaybookConfigKey)
	}
	return nil
}

func (p *panelForwardAnsibleRuntimeProvider) Apply(ctx context.Context, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardRuntimeResult, error) {
	_ = ctx

	cfg, err := p.loadConfig(action)
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendIptablesAnsible,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	node, err := p.loadIngressNode(tunnel)
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendIptablesAnsible,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	payload, err := p.buildPayload(action, forward, tunnel, node, cfg)
	if err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendIptablesAnsible,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendIptablesAnsible,
		Action:       action,
		ResourceType: "forward",
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       uintPtr(node.ID),
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      payload,
	}
	if err := p.db.Create(job).Error; err != nil {
		return &panelForwardRuntimeResult{
			Backend: model.ForwardRuntimeBackendIptablesAnsible,
			Status:  model.ForwardRuntimeJobStatusFailed,
			Message: err.Error(),
		}, err
	}

	return &panelForwardRuntimeResult{
		Backend: model.ForwardRuntimeBackendIptablesAnsible,
		Status:  model.ForwardRuntimeJobStatusPending,
		Message: fmt.Sprintf("queued ansible runtime job #%d", job.ID),
		Async:   true,
	}, nil
}

func (p *panelForwardAnsibleRuntimeProvider) loadConfig(action string) (*panelForwardAnsibleConfig, error) {
	cfg := &panelForwardAnsibleConfig{}
	if err := p.configService.GetJSON(forwardRuntimeAnsibleConfigJSONKey, cfg); err != nil {
		return nil, err
	}

	inventory, err := p.configService.Get(forwardRuntimeAnsibleInventoryConfigKey)
	if err != nil {
		return nil, err
	}
	playbookKey := forwardRuntimeAnsibleApplyPlaybookConfigKey
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		playbookKey = forwardRuntimeAnsibleRemovePlaybookConfigKey
	}
	playbook, err := p.configService.Get(playbookKey)
	if err != nil {
		return nil, err
	}
	becomeValue, err := p.configService.Get(forwardRuntimeAnsibleBecomeConfigKey)
	if err != nil {
		return nil, err
	}
	extraVarsValue, err := p.configService.Get(forwardRuntimeAnsibleExtraVarsConfigKey)
	if err != nil {
		return nil, err
	}

	extraVars := map[string]interface{}{}
	if strings.TrimSpace(extraVarsValue) != "" {
		if err := json.Unmarshal([]byte(extraVarsValue), &extraVars); err != nil {
			return nil, fmt.Errorf("%s is invalid JSON: %w", forwardRuntimeAnsibleExtraVarsConfigKey, err)
		}
	}

	cfg.ensureDefaults()
	legacyApplyPlaybook := ""
	legacyRemovePlaybook := ""
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		legacyRemovePlaybook = playbook
	} else {
		legacyApplyPlaybook = playbook
	}
	cfg.applyLegacyValues(
		inventory,
		legacyApplyPlaybook,
		legacyRemovePlaybook,
		strings.EqualFold(strings.TrimSpace(becomeValue), "true") || strings.TrimSpace(becomeValue) == "1",
		extraVars,
	)
	if err := cfg.validate(action); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (p *panelForwardAnsibleRuntimeProvider) loadIngressNode(tunnel *model.ForwardTunnel) (*model.ForwardNode, error) {
	if tunnel == nil {
		return nil, errors.New("forward tunnel is required")
	}
	if tunnel.InNodeID == 0 {
		return nil, errors.New("forward tunnel ingress node is not configured")
	}

	var node model.ForwardNode
	if err := p.db.First(&node, tunnel.InNodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("forward ingress node not found")
		}
		return nil, err
	}
	return &node, nil
}

func (p *panelForwardAnsibleRuntimeProvider) buildPayload(action string, forward *model.Forward, tunnel *model.ForwardTunnel, node *model.ForwardNode, cfg *panelForwardAnsibleConfig) (string, error) {
	targets, err := buildPanelForwardGostTargets(forward.RemoteAddr)
	if err != nil {
		return "", err
	}

	type targetPayload struct {
		Name string `json:"name"`
		Addr string `json:"addr"`
	}
	type forwardPayload struct {
		ID            uint   `json:"id"`
		UserID        uint   `json:"userId"`
		Name          string `json:"name"`
		InPort        int    `json:"inPort"`
		RemoteAddr    string `json:"remoteAddr"`
		InterfaceName string `json:"interfaceName"`
		Strategy      string `json:"strategy"`
		Status        int    `json:"status"`
	}
	type tunnelPayload struct {
		ID            uint   `json:"id"`
		Name          string `json:"name"`
		InNodeID      uint   `json:"inNodeId"`
		Protocol      string `json:"protocol"`
		TCPListenAddr string `json:"tcpListenAddr"`
		UDPListenAddr string `json:"udpListenAddr"`
		InterfaceName string `json:"interfaceName"`
	}
	type nodePayload struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		Host    string `json:"host"`
		Port    int    `json:"port"`
		APIPort int    `json:"apiPort"`
	}
	type ansiblePayload struct {
		Action    string                 `json:"action"`
		Inventory string                 `json:"inventory"`
		Playbook  string                 `json:"playbook"`
		Become    bool                   `json:"become"`
		ExtraVars map[string]interface{} `json:"extraVars,omitempty"`
		Forward   forwardPayload         `json:"forward"`
		Tunnel    tunnelPayload          `json:"tunnel"`
		Node      nodePayload            `json:"node"`
		Targets   []targetPayload        `json:"targets"`
	}

	items := make([]targetPayload, 0, len(targets))
	for _, target := range targets {
		items = append(items, targetPayload{Name: target.Name, Addr: target.Addr})
	}

	payload := ansiblePayload{
		Action:    action,
		Inventory: cfg.Inventory,
		Playbook:  cfg.playbookForAction(action),
		Become:    cfg.Become,
		ExtraVars: cfg.ExtraVars,
		Forward: forwardPayload{
			ID:            forward.ID,
			UserID:        forward.UserID,
			Name:          forward.Name,
			InPort:        forward.InPort,
			RemoteAddr:    forward.RemoteAddr,
			InterfaceName: forward.InterfaceName,
			Strategy:      normalizeStrategy(forward.Strategy, forward.RemoteAddr),
			Status:        forward.Status,
		},
		Tunnel: tunnelPayload{
			ID:            tunnel.ID,
			Name:          tunnel.Name,
			InNodeID:      tunnel.InNodeID,
			Protocol:      normalizePanelRuntimeProtocol(tunnel.Protocol),
			TCPListenAddr: tunnel.TCPListenAddr,
			UDPListenAddr: tunnel.UDPListenAddr,
			InterfaceName: tunnel.InterfaceName,
		},
		Node: nodePayload{
			ID:      node.ID,
			Name:    node.Name,
			Host:    node.Host,
			Port:    node.Port,
			APIPort: node.APIPort,
		},
		Targets: items,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func uintPtr(v uint) *uint {
	return &v
}
