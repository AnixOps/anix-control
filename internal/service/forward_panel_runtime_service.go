package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	forwardRuntimeConfigGroup                    = "forward"
	forwardRuntimeNodeXModeConfigKey             = "forward.runtime.nodex_mode"
	forwardRuntimeBackendConfigKey               = "forward.runtime_backend"
	forwardRuntimeLocalBackendConfigKey          = "forward.runtime.ansible.backend"
	forwardRuntimeAnsibleConfigJSONKey           = "forward.runtime.ansible.config"
	forwardRuntimeAnsibleInventoryConfigKey      = "forward.runtime.ansible.inventory"
	forwardRuntimeAnsibleApplyPlaybookConfigKey  = "forward.runtime.ansible.apply_playbook"
	forwardRuntimeAnsibleRemovePlaybookConfigKey = "forward.runtime.ansible.remove_playbook"
	forwardRuntimeAnsibleBecomeConfigKey         = "forward.runtime.ansible.become"
	forwardRuntimeAnsibleExtraVarsConfigKey      = "forward.runtime.ansible.extra_vars_json"

	legacyForwardRuntimeAnsibleConfigJSONKey           = "forward.runtime.iptables_ansible.config"
	legacyForwardRuntimeAnsibleInventoryConfigKey      = "forward.ansible.inventory"
	legacyForwardRuntimeAnsibleApplyPlaybookConfigKey  = "forward.ansible.playbook_apply"
	legacyForwardRuntimeAnsibleRemovePlaybookConfigKey = "forward.ansible.playbook_remove"
	legacyForwardRuntimeAnsibleBecomeConfigKey         = "forward.ansible.become"
	legacyForwardRuntimeAnsibleExtraVarsConfigKey      = "forward.ansible.extra_vars_json"

	defaultForwardAnsibleInventoryPath          = "config/deploy/ansible/inventory.ini"
	defaultForwardNftablesApplyPlaybookPath     = "config/deploy/ansible/playbooks/forward_apply_nftables.yml"
	defaultForwardNftablesRemovePlaybookPath    = "config/deploy/ansible/playbooks/forward_remove_nftables.yml"
	defaultForwardIptablesApplyPlaybookPath     = "config/deploy/ansible/playbooks/forward_apply.yml"
	defaultForwardIptablesRemovePlaybookPath    = "config/deploy/ansible/playbooks/forward_remove.yml"
	defaultForwardAnsibleWorkingDir             = "config/deploy/ansible"
	defaultForwardAnsibleConfigPath             = "config/deploy/ansible/ansible.cfg"
	defaultForwardAnsibleTargetPattern          = "{{node.host}}"
	defaultForwardLocalAnsibleBackend           = model.ForwardRuntimeBackendNftablesAnsible
	defaultForwardLocalAnsibleFirewallDriverNft = "nftables"
	defaultForwardLocalAnsibleFirewallDriverIpt = "iptables"
)

type panelForwardRuntimeResult struct {
	Backend string
	Status  int
	Message string
	Async   bool
}

type PanelForwardRuntimeService struct {
	db            *gorm.DB
	configService *SystemConfigService
	client        forwardRuntimeNodeXExecutor
}

type panelForwardAnsibleConfig struct {
	Inventory      string            `json:"inventory"`
	ApplyPlaybook  string            `json:"playbookApply"`
	RemovePlaybook string            `json:"playbookRemove"`
	Become         bool              `json:"become"`
	ExtraVars      map[string]any    `json:"extraVars"`
	Command        string            `json:"command"`
	WorkingDir     string            `json:"workingDir"`
	TargetPattern  string            `json:"targetPattern"`
	Environment    map[string]string `json:"environment"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

func NewPanelForwardRuntimeService(db *gorm.DB) *PanelForwardRuntimeService {
	configService := NewSystemConfigService(db)
	return &PanelForwardRuntimeService{
		db:            db,
		configService: configService,
		client:        newNodeXForwardRuntimeClient(configService),
	}
}

func (s *PanelForwardRuntimeService) Apply(ctx context.Context, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardRuntimeResult, error) {
	backend, err := s.resolveBackendForForward(forward)
	if err != nil {
		return failedPanelForwardRuntimeResult(model.ForwardRuntimeBackendGost, err), err
	}
	if err := s.validateBackendConfig(backend); err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}
	if isForwardRuntimeLocalAnsibleBackend(backend) {
		if err := s.validateAnsiblePaths(backend, action); err != nil {
			return failedPanelForwardRuntimeResult(backend, err), err
		}
	}

	req, nodeID, err := s.buildExecuteRequest(backend, action, forward, tunnel)
	if err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	if isForwardRuntimeLocalAnsibleBackend(backend) {
		return s.enqueueLocalAnsibleJob(action, forward, tunnel, req, nodeID)
	}
	if backend == model.ForwardRuntimeBackendCleanAgent {
		return s.enqueueCleanAgentJob(action, forward, tunnel, req, nodeID)
	}

	payloadJSON, err := json.Marshal(req)
	if err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	job := &model.ForwardRuntimeJob{
		Backend:      backend,
		Action:       action,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       nodeID,
		Payload:      string(payloadJSON),
	}

	startedAt := time.Now()
	job.Status = model.ForwardRuntimeJobStatusRunning
	job.StartedAt = &startedAt
	if err := s.db.Create(job).Error; err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	if nodeID == nil && backend == model.ForwardRuntimeBackendGost &&
		(action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause) {
		result := &panelForwardRuntimeResult{
			Backend: backend,
			Status:  model.ForwardRuntimeJobStatusSuccess,
			Message: "gost runtime skipped because ingress node is not configured",
		}
		completedAt := time.Now()
		if err := s.db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":       result.Status,
			"result":       result.Message,
			"error":        "",
			"completed_at": &completedAt,
		}).Error; err != nil {
			result.Message = strings.TrimSpace(result.Message + "; local audit update failed: " + err.Error())
		}
		return result, nil
	}

	execResult, execErr := s.client.Execute(ctx, req)
	result := buildPanelForwardRuntimeResult(backend, action, execResult, execErr)
	if execErr == nil && result.Status == model.ForwardRuntimeJobStatusFailed {
		execErr = errors.New(result.Message)
	}

	updateValues := map[string]any{
		"status": result.Status,
		"result": "",
		"error":  "",
	}
	if execResult != nil {
		updateValues["result"] = strings.TrimSpace(execResult.Result)
	}
	if execErr != nil {
		updateValues["error"] = execErr.Error()
	} else if result.Status == model.ForwardRuntimeJobStatusFailed {
		updateValues["error"] = strings.TrimSpace(result.Message)
	}
	if isForwardRuntimeTerminalStatus(result.Status) {
		completedAt := time.Now()
		updateValues["completed_at"] = &completedAt
	} else {
		updateValues["completed_at"] = nil
	}

	if err := s.db.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).Updates(updateValues).Error; err != nil {
		if execErr != nil {
			return result, execErr
		}
		result.Message = strings.TrimSpace(result.Message + "; local audit update failed: " + err.Error())
	}

	return result, execErr
}

func (s *PanelForwardRuntimeService) resolveBackendForForward(forward *model.Forward) (string, error) {
	if forward != nil {
		if backend, ok := normalizeForwardRuntimeBackend(forward.RuntimeBackend); ok {
			return backend, nil
		}
	}
	return s.resolveBackend()
}

func (s *PanelForwardRuntimeService) enqueueLocalAnsibleJob(action string, forward *model.Forward, tunnel *model.ForwardTunnel, req nodeXForwardExecuteRequest, nodeID *uint) (*panelForwardRuntimeResult, error) {
	backend, ok := normalizeForwardRuntimeLocalAnsibleBackend(req.Backend)
	if !ok {
		backend = defaultForwardLocalAnsibleBackend
	}
	if req.AnsibleRuntime == nil {
		err := errors.New("ansible runtime payload is required for local execution")
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	payloadJSON, err := json.Marshal(req.AnsibleRuntime)
	if err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	job := &model.ForwardRuntimeJob{
		Backend:      backend,
		Action:       action,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       nodeID,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      string(payloadJSON),
	}
	if err := s.db.Create(job).Error; err != nil {
		return failedPanelForwardRuntimeResult(backend, err), err
	}

	return &panelForwardRuntimeResult{
		Backend: backend,
		Status:  model.ForwardRuntimeJobStatusPending,
		Message: queuedPanelForwardRuntimeMessage(action),
		Async:   true,
	}, nil
}

func (s *PanelForwardRuntimeService) enqueueCleanAgentJob(action string, forward *model.Forward, tunnel *model.ForwardTunnel, req nodeXForwardExecuteRequest, nodeID *uint) (*panelForwardRuntimeResult, error) {
	payloadJSON, err := json.Marshal(req)
	if err != nil {
		return failedPanelForwardRuntimeResult(model.ForwardRuntimeBackendCleanAgent, err), err
	}

	job := &model.ForwardRuntimeJob{
		Backend:      model.ForwardRuntimeBackendCleanAgent,
		Action:       action,
		ResourceType: nodeXForwardResourceTypePanelForward,
		ResourceID:   uintPtr(forward.ID),
		ForwardID:    uintPtr(forward.ID),
		TunnelID:     uintPtr(tunnel.ID),
		NodeID:       nodeID,
		Status:       model.ForwardRuntimeJobStatusPending,
		Payload:      string(payloadJSON),
	}
	if err := s.db.Create(job).Error; err != nil {
		return failedPanelForwardRuntimeResult(model.ForwardRuntimeBackendCleanAgent, err), err
	}

	return &panelForwardRuntimeResult{
		Backend: model.ForwardRuntimeBackendCleanAgent,
		Status:  model.ForwardRuntimeJobStatusPending,
		Message: queuedCleanAgentRuntimeMessage(action),
		Async:   true,
	}, nil
}

func (s *PanelForwardRuntimeService) resolveBackend() (string, error) {
	if s.configService == nil {
		return model.ForwardRuntimeBackendGost, nil
	}

	nodeXMode, err := s.resolveNodeXMode()
	if err != nil {
		return "", err
	}
	if nodeXMode != nil {
		if *nodeXMode {
			return model.ForwardRuntimeBackendGost, nil
		}
		return s.resolveLocalAnsibleBackend()
	}

	value, err := s.configService.Get(forwardRuntimeBackendConfigKey)
	if err != nil {
		return "", err
	}

	switch backend, ok := normalizeForwardRuntimeBackend(value); {
	case strings.TrimSpace(value) == "":
		return model.ForwardRuntimeBackendGost, nil
	case ok:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid %s value: %s", forwardRuntimeBackendConfigKey, value)
	}
}

func (s *PanelForwardRuntimeService) resolveLocalAnsibleBackend() (string, error) {
	if s.configService == nil {
		return defaultForwardLocalAnsibleBackend, nil
	}

	value, err := s.configService.Get(forwardRuntimeLocalBackendConfigKey)
	if err != nil {
		return "", err
	}
	if backend, ok := normalizeForwardRuntimeLocalAnsibleBackend(value); ok {
		return backend, nil
	}

	value, err = s.configService.Get(forwardRuntimeBackendConfigKey)
	if err != nil {
		return "", err
	}
	if backend, ok := normalizeForwardRuntimeLocalAnsibleBackend(value); ok {
		return backend, nil
	}

	return defaultForwardLocalAnsibleBackend, nil
}

func (s *PanelForwardRuntimeService) resolveNodeXMode() (*bool, error) {
	if s.configService == nil {
		return nil, nil
	}

	value, err := s.configService.Get(forwardRuntimeNodeXModeConfigKey)
	if err != nil {
		return nil, err
	}
	return parseForwardRuntimeBoolValue(value, forwardRuntimeNodeXModeConfigKey)
}

func normalizeForwardRuntimeBackend(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case model.ForwardRuntimeBackendGost:
		return model.ForwardRuntimeBackendGost, true
	case model.ForwardRuntimeBackendNftablesAnsible:
		return model.ForwardRuntimeBackendNftablesAnsible, true
	case model.ForwardRuntimeBackendIptablesAnsible:
		return model.ForwardRuntimeBackendIptablesAnsible, true
	case model.ForwardRuntimeBackendCleanAgent:
		return model.ForwardRuntimeBackendCleanAgent, true
	default:
		return "", false
	}
}

func normalizeForwardRuntimeLocalAnsibleBackend(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case model.ForwardRuntimeBackendNftablesAnsible:
		return model.ForwardRuntimeBackendNftablesAnsible, true
	case model.ForwardRuntimeBackendIptablesAnsible:
		return model.ForwardRuntimeBackendIptablesAnsible, true
	default:
		return "", false
	}
}

func isForwardRuntimeLocalAnsibleBackend(backend string) bool {
	_, ok := normalizeForwardRuntimeLocalAnsibleBackend(backend)
	return ok
}

func isForwardRuntimeExecutionNodeBackend(backend string) bool {
	return isForwardRuntimeLocalAnsibleBackend(backend) || backend == model.ForwardRuntimeBackendCleanAgent
}

func forwardRuntimeLocalFirewallDriver(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return defaultForwardLocalAnsibleFirewallDriverIpt
	}
	return defaultForwardLocalAnsibleFirewallDriverNft
}

func forwardRuntimeLocalBackendLabel(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return "iptables / Ansible"
	}
	return "nftables / Ansible"
}

func (s *PanelForwardRuntimeService) validateBackendConfig(backend string) error {
	if backend != model.ForwardRuntimeBackendGost || s.configService == nil {
		return nil
	}

	baseURL, err := s.configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("%s is required for NodeX forward runtime", forwardRuntimeNodeXBaseURLConfigKey)
	}

	token, err := s.configService.Get(forwardRuntimeNodeXTokenConfigKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s is required for NodeX forward runtime", forwardRuntimeNodeXTokenConfigKey)
	}

	return nil
}

func (s *PanelForwardRuntimeService) validateAnsiblePaths(backend, action string) error {
	if s.configService == nil {
		return nil
	}

	cfg, err := s.loadPanelForwardAnsibleConfig(backend, action)
	if err != nil {
		return err
	}

	// Resolve and validate inventory file exists
	inventoryPath := resolveForwardRuntimeFilePath(cfg.WorkingDir, cfg.Inventory)
	if inventoryPath == "" || !pathExists(inventoryPath) {
		return fmt.Errorf("ansible inventory file not found: %s", cfg.Inventory)
	}

	// Resolve and validate playbook file exists
	playbook := cfg.playbookForAction(backend, action)
	playbookPath := resolveForwardRuntimeFilePath(cfg.WorkingDir, playbook)
	if playbookPath == "" || !pathExists(playbookPath) {
		return fmt.Errorf("ansible playbook file not found: %s", playbook)
	}

	return nil
}

func (s *PanelForwardRuntimeService) buildExecuteRequest(backend, action string, forward *model.Forward, tunnel *model.ForwardTunnel) (nodeXForwardExecuteRequest, *uint, error) {
	if forward == nil {
		return nodeXForwardExecuteRequest{}, nil, errors.New("forward is required")
	}
	if tunnel == nil {
		return nodeXForwardExecuteRequest{}, nil, errors.New("forward tunnel is required")
	}

	allowMissingIngress := backend == model.ForwardRuntimeBackendGost &&
		(action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause)

	var node *model.ForwardNode
	var err error
	switch backend {
	case model.ForwardRuntimeBackendGost:
		node, err = s.loadIngressNode(tunnel, allowMissingIngress)
	case model.ForwardRuntimeBackendNftablesAnsible, model.ForwardRuntimeBackendIptablesAnsible:
		if err := ensureAnsibleTunnelSupportsExecution(tunnel); err != nil {
			return nodeXForwardExecuteRequest{}, nil, err
		}
		node, err = s.loadExecutionNode(tunnel)
	case model.ForwardRuntimeBackendCleanAgent:
		if err := ensureAnsibleTunnelSupportsExecution(tunnel); err != nil {
			return nodeXForwardExecuteRequest{}, nil, err
		}
		node, err = s.loadExecutionNode(tunnel)
	default:
		node, err = s.loadIngressNode(tunnel, allowMissingIngress)
	}
	if err != nil {
		return nodeXForwardExecuteRequest{}, nil, err
	}
	limiter, err := s.loadForwardLimiter(forward, tunnel)
	if err != nil {
		return nodeXForwardExecuteRequest{}, nil, err
	}
	tunnelNodeID := tunnel.InNodeID
	if isForwardRuntimeExecutionNodeBackend(backend) {
		tunnelNodeID = storedPanelTunnelExecutionNodeID(tunnel)
	}

	req := nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      backend,
		Action:       action,
		PanelForward: &nodeXPanelForwardRequest{
			Forward: nodeXPanelForwardPayload{
				ID:            forward.ID,
				UserID:        forward.UserID,
				Name:          forward.Name,
				InPort:        forward.InPort,
				RemoteAddr:    forward.RemoteAddr,
				InterfaceName: forward.InterfaceName,
				Strategy:      normalizeRuntimeStrategy(forward.Strategy, forward.RemoteAddr),
				Status:        forward.Status,
			},
			Tunnel: nodeXPanelTunnelPayload{
				ID:            tunnel.ID,
				Name:          tunnel.Name,
				InNodeID:      tunnelNodeID,
				Protocol:      normalizePanelRuntimeProtocol(tunnel.Protocol),
				TCPListenAddr: tunnel.TCPListenAddr,
				UDPListenAddr: tunnel.UDPListenAddr,
				InterfaceName: tunnel.InterfaceName,
			},
			Limiter: limiter,
		},
	}

	if node == nil {
		return req, nil, nil
	}

	if isForwardRuntimeLocalAnsibleBackend(backend) {
		ansiblePayload, err := s.buildAnsibleRuntimePayload(backend, action, forward, tunnel, node)
		if err != nil {
			return nodeXForwardExecuteRequest{}, nil, err
		}
		req.AnsibleRuntime = ansiblePayload
	}

	req.PanelForward.IngressNode = nodeXForwardNodePayload{
		ID:       node.ID,
		Name:     node.Name,
		Host:     node.Host,
		Port:     node.Port,
		APIPort:  node.APIPort,
		APIToken: node.APIToken,
	}
	return req, uintPtr(node.ID), nil
}

func (s *PanelForwardRuntimeService) loadIngressNode(tunnel *model.ForwardTunnel, allowMissing bool) (*model.ForwardNode, error) {
	if tunnel == nil {
		return nil, errors.New("forward tunnel is required")
	}
	if tunnel.InNodeID == 0 {
		if allowMissing {
			return nil, nil
		}
		return nil, errors.New("forward tunnel ingress node is not configured")
	}

	var node model.ForwardNode
	if err := s.db.First(&node, tunnel.InNodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("forward ingress node not found")
		}
		return nil, err
	}
	return &node, nil
}

func (s *PanelForwardRuntimeService) loadExecutionNode(tunnel *model.ForwardTunnel) (*model.ForwardNode, error) {
	if tunnel == nil {
		return nil, errors.New("forward tunnel is required")
	}
	executionNodeID := storedPanelTunnelExecutionNodeID(tunnel)
	if executionNodeID == 0 {
		return nil, errors.New("forward tunnel execution node is not configured")
	}

	var node model.ForwardNode
	if err := s.db.First(&node, executionNodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("forward execution node not found")
		}
		return nil, err
	}
	return &node, nil
}

func ensureAnsibleTunnelSupportsExecution(tunnel *model.ForwardTunnel) error {
	if tunnel == nil {
		return errors.New("forward tunnel is required")
	}
	if tunnel.Type != 1 {
		return errors.New("local ansible runtime only supports type 1 tunnels")
	}
	return nil
}

func (s *PanelForwardRuntimeService) loadForwardLimiter(forward *model.Forward, tunnel *model.ForwardTunnel) (*panelForwardLimiterPayload, error) {
	if forward == nil || forward.UserID == 0 {
		return nil, nil
	}

	tunnelID := forward.TunnelID
	if tunnel != nil && tunnel.ID != 0 {
		tunnelID = tunnel.ID
	}
	if tunnelID == 0 {
		return nil, nil
	}

	var permission model.ForwardUserTunnel
	if err := s.db.Where("user_id = ? AND tunnel_id = ?", forward.UserID, tunnelID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if permission.SpeedID == nil || *permission.SpeedID == 0 {
		return nil, nil
	}

	var speedLimit model.SpeedLimit
	if err := s.db.First(&speedLimit, *permission.SpeedID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("speed limit %d not found for forward %d", *permission.SpeedID, forward.ID)
		}
		return nil, err
	}
	if speedLimit.TunnelID != tunnelID {
		return nil, fmt.Errorf("speed limit %d does not belong to tunnel %d", speedLimit.ID, tunnelID)
	}

	return &panelForwardLimiterPayload{
		SpeedID: speedLimit.ID,
		Name:    speedLimit.Name,
		Speed:   speedLimit.Speed,
	}, nil
}

func (s *PanelForwardRuntimeService) buildAnsibleRuntimePayload(backend, action string, forward *model.Forward, tunnel *model.ForwardTunnel, node *model.ForwardNode) (*panelForwardAnsibleRuntimePayload, error) {
	if node == nil {
		return nil, errors.New("forward execution node is required for ansible runtime")
	}
	cfg, err := s.loadPanelForwardAnsibleConfig(backend, action)
	if err != nil {
		return nil, err
	}
	targets, err := buildPanelForwardAnsibleTargets(forward.RemoteAddr)
	if err != nil {
		return nil, err
	}
	limiter, err := s.loadForwardLimiter(forward, tunnel)
	if err != nil {
		return nil, err
	}
	return &panelForwardAnsibleRuntimePayload{
		Backend:        backend,
		FirewallDriver: forwardRuntimeLocalFirewallDriver(backend),
		Action:         action,
		Inventory:      cfg.Inventory,
		Playbook:       cfg.playbookForAction(backend, action),
		Become:         cfg.Become,
		ExtraVars:      copyStringInterfaceMap(cfg.ExtraVars),
		Command:        cfg.Command,
		WorkingDir:     cfg.WorkingDir,
		TargetPattern:  cfg.TargetPattern,
		Environment:    copyStringMap(cfg.Environment),
		TimeoutSeconds: cfg.TimeoutSeconds,
		Forward: panelForwardAnsibleForwardPayload{
			ID:            forward.ID,
			UserID:        forward.UserID,
			Name:          forward.Name,
			InPort:        forward.InPort,
			RemoteAddr:    forward.RemoteAddr,
			InterfaceName: forward.InterfaceName,
			Strategy:      normalizeRuntimeStrategy(forward.Strategy, forward.RemoteAddr),
			Status:        forward.Status,
		},
		Tunnel: panelForwardAnsibleTunnelPayload{
			ID:            tunnel.ID,
			Name:          tunnel.Name,
			InNodeID:      storedPanelTunnelExecutionNodeID(tunnel),
			Protocol:      normalizePanelRuntimeProtocol(tunnel.Protocol),
			TCPListenAddr: tunnel.TCPListenAddr,
			UDPListenAddr: tunnel.UDPListenAddr,
			InterfaceName: tunnel.InterfaceName,
		},
		Node: panelForwardAnsibleNodePayload{
			ID:      node.ID,
			Name:    node.Name,
			Host:    node.Host,
			Port:    node.Port,
			APIPort: node.APIPort,
		},
		Limiter: limiter,
		Targets: targets,
	}, nil
}

func (s *PanelForwardRuntimeService) loadPanelForwardAnsibleConfigForDiagnostics() (*panelForwardAnsibleConfig, error) {
	backend, err := s.resolveLocalAnsibleBackend()
	if err != nil {
		return nil, err
	}
	return s.loadPanelForwardAnsibleConfigForDiagnosticsWithBackend(backend)
}

func (s *PanelForwardRuntimeService) loadPanelForwardAnsibleConfigForDiagnosticsWithBackend(backend string) (*panelForwardAnsibleConfig, error) {
	cfg := &panelForwardAnsibleConfig{}
	if err := s.configService.GetJSON(forwardRuntimeAnsibleConfigJSONKey, cfg); err != nil {
		return nil, err
	}
	if isZeroPanelForwardAnsibleConfig(cfg) {
		if err := s.configService.GetJSON(legacyForwardRuntimeAnsibleConfigJSONKey, cfg); err != nil {
			return nil, err
		}
	}
	if value, err := s.configService.Get(forwardRuntimeAnsibleInventoryConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.Inventory = value
	} else if value, err := s.configService.Get(legacyForwardRuntimeAnsibleInventoryConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.Inventory = value
	}
	if value, err := s.configService.Get(forwardRuntimeAnsibleApplyPlaybookConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.ApplyPlaybook = value
	} else if value, err := s.configService.Get(legacyForwardRuntimeAnsibleApplyPlaybookConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.ApplyPlaybook = value
	}
	if value, err := s.configService.Get(forwardRuntimeAnsibleRemovePlaybookConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.RemovePlaybook = value
	} else if value, err := s.configService.Get(legacyForwardRuntimeAnsibleRemovePlaybookConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.RemovePlaybook = value
	}
	if value, err := s.configService.Get(forwardRuntimeAnsibleBecomeConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.Become = strings.EqualFold(value, "true") || strings.TrimSpace(value) == "1"
	} else if value, err := s.configService.Get(legacyForwardRuntimeAnsibleBecomeConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		cfg.Become = strings.EqualFold(value, "true") || strings.TrimSpace(value) == "1"
	}
	if value, err := s.configService.Get(forwardRuntimeAnsibleExtraVarsConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		extraVars := map[string]any{}
		if err := json.Unmarshal([]byte(value), &extraVars); err != nil {
			return nil, fmt.Errorf("%s is invalid JSON: %w", forwardRuntimeAnsibleExtraVarsConfigKey, err)
		}
		cfg.ExtraVars = extraVars
	} else if value, err := s.configService.Get(legacyForwardRuntimeAnsibleExtraVarsConfigKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		extraVars := map[string]any{}
		if err := json.Unmarshal([]byte(value), &extraVars); err != nil {
			return nil, fmt.Errorf("%s is invalid JSON: %w", legacyForwardRuntimeAnsibleExtraVarsConfigKey, err)
		}
		cfg.ExtraVars = extraVars
	}
	cfg.ensureDefaults(backend)
	return cfg, nil
}

func (s *PanelForwardRuntimeService) loadPanelForwardAnsibleConfig(backend, action string) (*panelForwardAnsibleConfig, error) {
	cfg, err := s.loadPanelForwardAnsibleConfigForDiagnosticsWithBackend(backend)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(backend, action); err != nil {
		return nil, err
	}
	return cfg, nil
}

func buildPanelForwardAnsibleTargets(remoteAddr string) ([]panelForwardAnsibleTargetPayload, error) {
	rawTargets := strings.Split(normalizeRuntimeRemoteAddr(remoteAddr), ",")
	targets := make([]panelForwardAnsibleTargetPayload, 0, len(rawTargets))
	count := 0
	for _, raw := range rawTargets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		host, port, err := splitRuntimeTarget(target)
		if err != nil {
			return nil, fmt.Errorf("invalid remote address %s: %w", target, err)
		}
		count++
		targets = append(targets, panelForwardAnsibleTargetPayload{
			Name: fmt.Sprintf("target-%d", count),
			Addr: fmt.Sprintf("%s:%d", host, port),
		})
	}
	if len(targets) == 0 {
		return nil, errors.New("no valid remote target configured")
	}
	return targets, nil
}

func copyStringInterfaceMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	result := make(map[string]any, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	result := make(map[string]string, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}

func buildPanelForwardRuntimeResult(backend, action string, execResult *nodeXForwardExecuteResult, execErr error) *panelForwardRuntimeResult {
	result := &panelForwardRuntimeResult{
		Backend: backend,
		Status:  model.ForwardRuntimeJobStatusSuccess,
	}
	if execResult != nil {
		if strings.TrimSpace(execResult.Backend) != "" {
			result.Backend = execResult.Backend
		}
		if execResult.Status != 0 {
			result.Status = execResult.Status
		} else if execResult.Async {
			result.Status = model.ForwardRuntimeJobStatusPending
		}
		result.Message = strings.TrimSpace(execResult.Message)
		result.Async = execResult.Async
	}
	if execErr != nil {
		result.Status = model.ForwardRuntimeJobStatusFailed
		if strings.TrimSpace(result.Message) == "" {
			result.Message = execErr.Error()
		}
	}
	if strings.TrimSpace(result.Message) == "" {
		result.Message = defaultPanelForwardRuntimeMessage(action)
	}
	return result
}

func defaultPanelForwardRuntimeMessage(action string) string {
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return "forward runtime removed"
	}
	return "forward runtime synchronized"
}

func queuedPanelForwardRuntimeMessage(action string) string {
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return "ansible runtime removal queued for local executor"
	}
	return "ansible runtime queued for local executor"
}

func queuedCleanAgentRuntimeMessage(action string) string {
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return "clean agent runtime removal queued"
	}
	return "clean agent runtime queued"
}

func failedPanelForwardRuntimeResult(backend string, err error) *panelForwardRuntimeResult {
	return &panelForwardRuntimeResult{
		Backend: backend,
		Status:  model.ForwardRuntimeJobStatusFailed,
		Message: err.Error(),
	}
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

func normalizeRuntimeRemoteAddr(raw string) string {
	lines := strings.Split(raw, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
	}
	return strings.Join(items, ",")
}

func normalizeRuntimeStrategy(strategy, remoteAddr string) string {
	count := 0
	for _, raw := range strings.Split(normalizeRuntimeRemoteAddr(remoteAddr), ",") {
		if strings.TrimSpace(raw) != "" {
			count++
		}
	}
	if count <= 1 {
		return "fifo"
	}

	switch strategy {
	case "fifo", "round", "rand", "hash":
		return strategy
	default:
		return "fifo"
	}
}

func splitRuntimeTarget(target string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return "", 0, errors.New("unable to parse target address")
	}
	host = strings.Trim(host, "[]")
	var port int
	if _, scanErr := fmt.Sscanf(portStr, "%d", &port); scanErr != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("unable to parse target port")
	}
	return host, port, nil
}

func isForwardRuntimeTerminalStatus(status int) bool {
	return status == model.ForwardRuntimeJobStatusSuccess || status == model.ForwardRuntimeJobStatusFailed
}

func (c *panelForwardAnsibleConfig) ensureDefaults(backend string) {
	if c.ExtraVars == nil {
		c.ExtraVars = map[string]any{}
	}
	if c.Environment == nil {
		c.Environment = map[string]string{}
	}
	backend = normalizeForwardRuntimeLocalBackendOrDefault(backend)
	if strings.TrimSpace(c.Inventory) == "" {
		c.Inventory = defaultForwardAnsibleInventoryPath
	}
	if strings.TrimSpace(c.ApplyPlaybook) == "" {
		c.ApplyPlaybook = defaultForwardApplyPlaybookPathForBackend(backend)
	}
	if strings.TrimSpace(c.RemovePlaybook) == "" {
		c.RemovePlaybook = defaultForwardRemovePlaybookPathForBackend(backend)
	}
	if strings.TrimSpace(c.WorkingDir) == "" {
		c.WorkingDir = defaultForwardAnsibleWorkingDir
	}
	if strings.TrimSpace(c.TargetPattern) == "" {
		c.TargetPattern = defaultForwardAnsibleTargetPattern
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = int(defaultForwardRuntimeJobTimeout / time.Second)
	}
	if strings.TrimSpace(c.Environment["ANSIBLE_CONFIG"]) == "" {
		c.Environment["ANSIBLE_CONFIG"] = defaultForwardAnsibleConfigPath
	}
}

func defaultForwardApplyPlaybookPathForBackend(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return defaultForwardIptablesApplyPlaybookPath
	}
	return defaultForwardNftablesApplyPlaybookPath
}

func defaultForwardRemovePlaybookPathForBackend(backend string) string {
	if backend == model.ForwardRuntimeBackendIptablesAnsible {
		return defaultForwardIptablesRemovePlaybookPath
	}
	return defaultForwardNftablesRemovePlaybookPath
}

func normalizeForwardRuntimeLocalBackendOrDefault(backend string) string {
	if normalized, ok := normalizeForwardRuntimeLocalAnsibleBackend(backend); ok {
		return normalized
	}
	return defaultForwardLocalAnsibleBackend
}

func (c *panelForwardAnsibleConfig) playbookForAction(backend, action string) string {
	_ = backend
	if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
		return strings.TrimSpace(c.RemovePlaybook)
	}
	return strings.TrimSpace(c.ApplyPlaybook)
}

func (c *panelForwardAnsibleConfig) validate(backend, action string) error {
	backend = normalizeForwardRuntimeLocalBackendOrDefault(backend)
	if strings.TrimSpace(c.Inventory) == "" {
		return fmt.Errorf("%s is required for %s runtime", forwardRuntimeAnsibleInventoryConfigKey, backend)
	}
	playbook := c.playbookForAction(backend, action)
	if strings.TrimSpace(playbook) == "" {
		if action == model.ForwardRuntimeJobActionDelete || action == model.ForwardRuntimeJobActionPause {
			return fmt.Errorf("%s is required for %s runtime", forwardRuntimeAnsibleRemovePlaybookConfigKey, backend)
		}
		return fmt.Errorf("%s is required for %s runtime", forwardRuntimeAnsibleApplyPlaybookConfigKey, backend)
	}
	return nil
}

func isZeroPanelForwardAnsibleConfig(cfg *panelForwardAnsibleConfig) bool {
	if cfg == nil {
		return true
	}
	return strings.TrimSpace(cfg.Inventory) == "" &&
		strings.TrimSpace(cfg.ApplyPlaybook) == "" &&
		strings.TrimSpace(cfg.RemovePlaybook) == "" &&
		!cfg.Become &&
		len(cfg.ExtraVars) == 0 &&
		strings.TrimSpace(cfg.Command) == "" &&
		strings.TrimSpace(cfg.WorkingDir) == "" &&
		strings.TrimSpace(cfg.TargetPattern) == "" &&
		len(cfg.Environment) == 0 &&
		cfg.TimeoutSeconds == 0
}

func uintPtr(v uint) *uint {
	return &v
}

func parseForwardRuntimeBoolValue(value, key string) (*bool, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "":
		return nil, nil
	case "1", "true", "yes", "on":
		enabled := true
		return &enabled, nil
	case "0", "false", "no", "off":
		enabled := false
		return &enabled, nil
	default:
		return nil, fmt.Errorf("invalid %s value: %s", key, value)
	}
}

func forwardRuntimeBackendForMode(enabled bool) string {
	if enabled {
		return model.ForwardRuntimeBackendGost
	}
	return defaultForwardLocalAnsibleBackend
}

// SyncForwardsToBackend re-synces all active forwards to the current backend.
// Used when the admin switches between NodeX and Local mode.
func (s *PanelForwardRuntimeService) SyncForwardsToBackend(targetBackend string) (synced int, failed int, err error) {
	var forwards []model.Forward
	if err := s.db.Where("status = ?", model.ForwardStatusActive).Find(&forwards).Error; err != nil {
		return 0, 0, err
	}

	for i := range forwards {
		fwd := &forwards[i]
		if fwd.RuntimeBackend == targetBackend {
			continue // already on the target backend
		}

		// Load tunnel for the forward
		var tunnel model.ForwardTunnel
		if err := s.db.First(&tunnel, fwd.TunnelID).Error; err != nil {
			failed++
			continue
		}

		// Apply the forward with the new backend
		// We temporarily override the forward's backend so resolveBackendForForward picks it up
		fwd.RuntimeBackend = targetBackend

		result, applyErr := s.Apply(context.Background(), model.ForwardRuntimeJobActionSync, fwd, &tunnel)
		if applyErr != nil {
			failed++
			continue
		}

		if result.Status == model.ForwardRuntimeJobStatusFailed {
			failed++
			continue
		}

		// Persist the backend change. For Ansible, the executor also updates this when the job runs.
		// For gost (synchronous), we must do it here since Apply() only updates the job record.
		if err := s.db.Model(&model.Forward{}).Where("id = ?", fwd.ID).Updates(map[string]any{
			"runtime_backend": targetBackend,
		}).Error; err != nil {
			failed++
			continue
		}

		synced++
	}

	return synced, failed, nil
}

// MarkForwardsPendingForBackend resets forwards that need re-sync to pending status.
// Call this after a backend switch to mark mismatched forwards for later re-sync.
func (s *PanelForwardRuntimeService) MarkForwardsPendingForBackend(targetBackend string) (int64, error) {
	result := s.db.Model(&model.Forward{}).
		Where("status = ? AND runtime_backend != ?", model.ForwardStatusActive, targetBackend).
		Updates(map[string]any{
			"runtime_status":  model.ForwardRuntimeJobStatusPending,
			"runtime_message": "pending re-sync to new backend",
		})
	return result.RowsAffected, result.Error
}
