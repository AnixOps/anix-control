package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultForwardCleanAgentHeartbeatInterval = 10
	defaultForwardCleanAgentTaskLimit         = 10
)

var (
	ErrForwardCleanAgentUnauthorized = errors.New("invalid forward clean agent credentials")
	ErrForwardCleanAgentRevoked      = errors.New("forward clean agent revoked")
	// ErrForwardCleanAgentNodeMismatch refuses an agent's registration
	// under a node other than the one its token is bound to.
	ErrForwardCleanAgentNodeMismatch = errors.New("forward clean agent is bound to another node")
	// ErrForwardCleanAgentNodeRequired refuses to issue a token without the
	// forward node it is for.
	ErrForwardCleanAgentNodeRequired = errors.New("nodeId is required: a clean agent token is issued for one forward node")
	// ErrForwardCleanAgentNodeNotFound refuses to issue a token for a forward
	// node that does not exist.
	ErrForwardCleanAgentNodeNotFound = errors.New("forward node not found")
)

type ForwardCleanAgentService struct {
	db *gorm.DB
}

type ForwardCleanAgentCreateInput struct {
	Name   string `json:"name"`
	NodeID *uint  `json:"nodeId"`
}

type ForwardCleanAgentRegisterInput struct {
	Name         string   `json:"name"`
	Token        string   `json:"token"`
	NodeID       *uint    `json:"nodeId"`
	Version      string   `json:"version"`
	Hostname     string   `json:"hostname"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Kernel       string   `json:"kernel"`
	PublicIP     string   `json:"publicIp"`
	PrivateIP    string   `json:"privateIp"`
	Capabilities []string `json:"capabilities"`
}

type ForwardCleanAgentHeartbeatInput struct {
	AgentID      uint     `json:"agentId"`
	Token        string   `json:"token"`
	Version      string   `json:"version"`
	Hostname     string   `json:"hostname"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Kernel       string   `json:"kernel"`
	PublicIP     string   `json:"publicIp"`
	PrivateIP    string   `json:"privateIp"`
	Capabilities []string `json:"capabilities"`
	Limit        int      `json:"limit"`
}

type ForwardCleanAgentReportInput struct {
	AgentID  uint   `json:"agentId"`
	Token    string `json:"token"`
	JobID    uint   `json:"jobId"`
	Status   int    `json:"status"`
	Success  *bool  `json:"success"`
	Result   string `json:"result"`
	Error    string `json:"error"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type ForwardCleanAgentTokenResult struct {
	Agent *model.ForwardCleanAgent `json:"agent"`
	Token string                   `json:"token"`
}

type ForwardCleanAgentAction struct {
	JobID        uint            `json:"jobId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   *uint           `json:"resourceId"`
	ForwardID    *uint           `json:"forwardId"`
	TunnelID     *uint           `json:"tunnelId"`
	NodeID       *uint           `json:"nodeId"`
	Payload      json.RawMessage `json:"payload"`
}

func NewForwardCleanAgentService(db *gorm.DB) *ForwardCleanAgentService {
	return &ForwardCleanAgentService{db: db}
}

// CreateCleanAgentTokenTx is CreateToken in the caller's transaction: the
// agent row and its split row are written in it, and IssueCleanAgent runs
// it in the operation's transaction. A token that collides with another
// (never, in practice) is generated again under a savepoint.
func CreateCleanAgentTokenTx(tx *gorm.DB, input ForwardCleanAgentCreateInput) (*ForwardCleanAgentTokenResult, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "v2forward-agent"
	}
	nodeID := normalizeOptionalUint(input.NodeID)
	if nodeID == nil {
		return nil, ErrForwardCleanAgentNodeRequired
	}
	var nodes int64
	if err := tx.Model(&model.ForwardNode{}).Where("id = ?", *nodeID).Count(&nodes).Error; err != nil {
		return nil, err
	}
	if nodes == 0 {
		return nil, ErrForwardCleanAgentNodeNotFound
	}

	for attempt := 0; attempt < 3; attempt++ {
		token, err := generateForwardCleanAgentToken()
		if err != nil {
			return nil, err
		}

		agent := &model.ForwardCleanAgent{
			Name:   name,
			NodeID: nodeID,
			Token:  token,
			Status: model.ForwardCleanAgentStatusOffline,
		}
		if err := tx.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(agent).Error; err != nil {
				return err
			}
			return nodesecrets.Sync(tx, nodesecrets.TableCleanAgent, agent.ID)
		}); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return nil, err
		}
		return &ForwardCleanAgentTokenResult{Agent: agent, Token: token}, nil
	}

	return nil, errors.New("failed to generate unique clean agent token")
}

// RevokeCleanAgentTx is RevokeAgent in the caller's transaction: the agent
// is marked revoked and its split row follows. RevokeCredential runs it in
// the operation's transaction. An agent that does not exist is
// gorm.ErrRecordNotFound; one already revoked is revoked again.
func RevokeCleanAgentTx(tx *gorm.DB, id uint) error {
	if id == 0 {
		return errors.New("agent id is required")
	}
	now := time.Now()
	result := tx.Model(&model.ForwardCleanAgent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     model.ForwardCleanAgentStatusRevoked,
			"revoked_at": &now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nodesecrets.Sync(tx, nodesecrets.TableCleanAgent, id)
}

// Register marks an agent online and binds its token to a node.
//
// A token is bound to the node it was issued for (CreateToken's nodeId) or,
// for a token an earlier build issued without one, to the node of its first
// registration that names one. A heartbeat claims the pending jobs of the agent's node,
// and their payloads carry the node's API token, so a token that could
// register again under another node could take that node's jobs and
// credentials. A registration naming another node is refused
// (ErrForwardCleanAgentNodeMismatch) and changes nothing; one naming no node
// keeps the binding. Binding to another node takes a new token.
func (s *ForwardCleanAgentService) Register(input ForwardCleanAgentRegisterInput) (*model.ForwardCleanAgent, error) {
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return nil, ErrForwardCleanAgentUnauthorized
	}

	// The token is looked up through the node credential split, which
	// never matches a tombstone or the placeholder.
	found, err := nodesecrets.CleanAgentByToken(s.db, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForwardCleanAgentUnauthorized
		}
		return nil, err
	}
	agent := *found
	if isForwardCleanAgentRevoked(&agent) {
		return nil, ErrForwardCleanAgentRevoked
	}

	requested := normalizeOptionalUint(input.NodeID)
	if requested != nil && normalizeOptionalUint(agent.NodeID) == nil {
		// The first registration that names a node binds the token. Only an
		// unbound row is updated, so of two concurrent first registrations
		// one binds and the other is refused below.
		if err := s.db.Model(&model.ForwardCleanAgent{}).
			Where("id = ? AND (node_id IS NULL OR node_id = 0)", agent.ID).
			Update("node_id", *requested).Error; err != nil {
			return nil, err
		}
		if err := s.db.First(&agent, agent.ID).Error; err != nil {
			return nil, err
		}
	}
	if bound := normalizeOptionalUint(agent.NodeID); requested != nil && (bound == nil || *bound != *requested) {
		return nil, ErrForwardCleanAgentNodeMismatch
	}

	now := time.Now()
	updates := cleanAgentInfoUpdates(input.Name, input.Version, input.Hostname, input.OS, input.Arch, input.Kernel, input.PublicIP, input.PrivateIP, input.Capabilities)
	updates["status"] = model.ForwardCleanAgentStatusOnline
	updates["last_seen"] = &now
	updates["last_error"] = ""

	if err := s.db.Model(&model.ForwardCleanAgent{}).Where("id = ?", agent.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&agent, agent.ID).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *ForwardCleanAgentService) Heartbeat(input ForwardCleanAgentHeartbeatInput) ([]ForwardCleanAgentAction, error) {
	agent, err := s.authenticate(input.AgentID, input.Token)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 || limit > defaultForwardCleanAgentTaskLimit {
		limit = defaultForwardCleanAgentTaskLimit
	}
	now := time.Now()

	// The runtime jobs are gone once the v4.2 upgrade dropped the flux
	// tables (forwardlegacy.Drop): the agent is still known and seen, and
	// has nothing to run. Only the database saying the table does not exist
	// means that; any other failure is the error it was before. The jobs
	// are read ahead of the transaction because PostgreSQL aborts the
	// transaction a statement fails in, and the agent's own update must not
	// go with it. The claims below stay atomic (status = pending), so two
	// heartbeats never take one job twice.
	query := s.db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
		Where("backend = ? AND status = ?", model.ForwardRuntimeBackendCleanAgent, model.ForwardRuntimeJobStatusPending)
	if agent.NodeID == nil || *agent.NodeID == 0 {
		query = query.Where("node_id IS NULL")
	} else {
		query = query.Where("node_id = ?", *agent.NodeID)
	}
	var jobs []model.ForwardRuntimeJob
	if err := query.Order("id ASC").Limit(limit).Find(&jobs).Error; err != nil {
		if !forwardlegacy.MissingTable(err, model.ForwardRuntimeJob{}.TableName()) {
			return nil, err
		}
		jobs = nil
	}

	var actions []ForwardCleanAgentAction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		updates := cleanAgentInfoUpdates("", input.Version, input.Hostname, input.OS, input.Arch, input.Kernel, input.PublicIP, input.PrivateIP, input.Capabilities)
		updates["status"] = model.ForwardCleanAgentStatusOnline
		updates["last_seen"] = &now
		updates["last_error"] = ""
		if err := tx.Model(&model.ForwardCleanAgent{}).Where("id = ?", agent.ID).Updates(updates).Error; err != nil {
			return err
		}

		for idx := range jobs {
			job := jobs[idx]
			result := tx.Model(&model.ForwardRuntimeJob{}).
				Where("id = ? AND status = ?", job.ID, model.ForwardRuntimeJobStatusPending).
				Updates(map[string]any{
					"status":     model.ForwardRuntimeJobStatusRunning,
					"agent_id":   agent.ID,
					"started_at": &now,
					"claimed_at": &now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			if err := updateForwardRuntimeRunningStateTx(tx, &job, &now); err != nil {
				return err
			}
			actions = append(actions, buildForwardCleanAgentAction(&job))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *ForwardCleanAgentService) Report(input ForwardCleanAgentReportInput) error {
	if input.JobID == 0 {
		return errors.New("jobId is required")
	}
	if input.Upload < 0 || input.Download < 0 {
		return errors.New("traffic values must be non-negative")
	}

	agent, err := s.authenticate(input.AgentID, input.Token)
	if err != nil {
		return err
	}

	var job model.ForwardRuntimeJob
	if err := s.db.Where("id = ? AND backend = ?", input.JobID, model.ForwardRuntimeBackendCleanAgent).First(&job).Error; err != nil {
		return err
	}
	if job.AgentID == nil || *job.AgentID != agent.ID {
		return ErrForwardCleanAgentUnauthorized
	}

	status, message := normalizeForwardCleanAgentReportStatus(input)
	now := time.Now()
	updateValues := map[string]any{
		"status":       status,
		"result":       strings.TrimSpace(input.Result),
		"error":        "",
		"completed_at": &now,
	}
	if status == model.ForwardRuntimeJobStatusFailed {
		updateValues["error"] = message
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ForwardRuntimeJob{}).Where("id = ?", job.ID).Updates(updateValues).Error; err != nil {
			return err
		}

		agentUpdates := map[string]any{
			"status":    model.ForwardCleanAgentStatusOnline,
			"last_seen": &now,
		}
		if status == model.ForwardRuntimeJobStatusFailed {
			agentUpdates["last_error"] = message
		} else {
			agentUpdates["last_error"] = ""
		}
		if err := tx.Model(&model.ForwardCleanAgent{}).Where("id = ?", agent.ID).Updates(agentUpdates).Error; err != nil {
			return err
		}

		if status == model.ForwardRuntimeJobStatusSuccess {
			if err := cleanupPanelForwardAfterRuntimeDeleteTx(tx, &job); err != nil {
				return err
			}
			if job.Action == model.ForwardRuntimeJobActionDelete {
				return nil
			}
		}
		return s.updateForwardRuntimeStateTx(tx, &job, status, message, &now)
	})
	if err != nil {
		return err
	}

	if status == model.ForwardRuntimeJobStatusSuccess && job.Action != model.ForwardRuntimeJobActionDelete && job.ForwardID != nil && (input.Upload > 0 || input.Download > 0) {
		return NewPanelForwardService(s.db).RecordForwardTraffic([]PanelForwardTrafficRecord{{
			ForwardID: *job.ForwardID,
			Upload:    input.Upload,
			Download:  input.Download,
		}})
	}
	return nil
}

func (s *ForwardCleanAgentService) authenticate(agentID uint, token string) (*model.ForwardCleanAgent, error) {
	if agentID == 0 || strings.TrimSpace(token) == "" {
		return nil, ErrForwardCleanAgentUnauthorized
	}

	agent, err := nodesecrets.CleanAgentWithToken(s.db, agentID, strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForwardCleanAgentUnauthorized
		}
		return nil, err
	}
	if isForwardCleanAgentRevoked(agent) {
		return nil, ErrForwardCleanAgentRevoked
	}
	return agent, nil
}

func (s *ForwardCleanAgentService) updateForwardRuntimeStateTx(tx *gorm.DB, job *model.ForwardRuntimeJob, runtimeStatus int, message string, syncedAt *time.Time) error {
	if job.ForwardID == nil || *job.ForwardID == 0 {
		return nil
	}

	updates := map[string]any{
		"runtime_backend":      model.ForwardRuntimeBackendCleanAgent,
		"runtime_status":       runtimeStatus,
		"runtime_message":      strings.TrimSpace(message),
		"runtime_last_sync_at": syncedAt,
	}
	switch runtimeStatus {
	case model.ForwardRuntimeJobStatusSuccess:
		if forwardStatus := successForwardStatusForAction(job.Action); forwardStatus != nil {
			updates["status"] = *forwardStatus
		}
	case model.ForwardRuntimeJobStatusFailed:
		if forwardStatus := failedForwardStatusForAction(job.Action); forwardStatus != nil {
			updates["status"] = *forwardStatus
		}
	}

	return tx.Model(&model.Forward{}).Where("id = ?", *job.ForwardID).Updates(updates).Error
}

func normalizeForwardCleanAgentReportStatus(input ForwardCleanAgentReportInput) (int, string) {
	status := input.Status
	if input.Success != nil {
		if *input.Success {
			status = model.ForwardRuntimeJobStatusSuccess
		} else {
			status = model.ForwardRuntimeJobStatusFailed
		}
	}
	if status != model.ForwardRuntimeJobStatusSuccess && status != model.ForwardRuntimeJobStatusFailed {
		if strings.TrimSpace(input.Error) != "" {
			status = model.ForwardRuntimeJobStatusFailed
		} else {
			status = model.ForwardRuntimeJobStatusSuccess
		}
	}

	message := strings.TrimSpace(input.Error)
	if status == model.ForwardRuntimeJobStatusSuccess {
		message = strings.TrimSpace(input.Result)
		if message == "" {
			message = "clean agent runtime synchronized"
		}
	} else if message == "" {
		message = "clean agent runtime execution failed"
	}
	return status, message
}

// buildForwardCleanAgentAction is a claimed job as the agent receives it.
// The payload is served without a node token: a payload written before
// NO-7 is scrubbed here until the start-up pass has rewritten it.
func buildForwardCleanAgentAction(job *model.ForwardRuntimeJob) ForwardCleanAgentAction {
	payload := json.RawMessage("{}")
	stored, _ := ScrubForwardRuntimeJobPayload(job.Payload)
	if trimmed := strings.TrimSpace(stored); trimmed != "" {
		payload = json.RawMessage(trimmed)
	}
	return ForwardCleanAgentAction{
		JobID:        job.ID,
		Action:       job.Action,
		ResourceType: job.ResourceType,
		ResourceID:   job.ResourceID,
		ForwardID:    job.ForwardID,
		TunnelID:     job.TunnelID,
		NodeID:       job.NodeID,
		Payload:      payload,
	}
}

func cleanAgentInfoUpdates(name, version, hostname, osName, arch, kernel, publicIP, privateIP string, capabilities []string) map[string]any {
	updates := map[string]any{}
	if strings.TrimSpace(name) != "" {
		updates["name"] = strings.TrimSpace(name)
	}
	if strings.TrimSpace(version) != "" {
		updates["version"] = strings.TrimSpace(version)
	}
	if strings.TrimSpace(hostname) != "" {
		updates["hostname"] = strings.TrimSpace(hostname)
	}
	if strings.TrimSpace(osName) != "" {
		updates["os"] = strings.TrimSpace(osName)
	}
	if strings.TrimSpace(arch) != "" {
		updates["arch"] = strings.TrimSpace(arch)
	}
	if strings.TrimSpace(kernel) != "" {
		updates["kernel"] = strings.TrimSpace(kernel)
	}
	if strings.TrimSpace(publicIP) != "" {
		updates["public_ip"] = strings.TrimSpace(publicIP)
	}
	if strings.TrimSpace(privateIP) != "" {
		updates["private_ip"] = strings.TrimSpace(privateIP)
	}
	if capabilities != nil {
		data, _ := json.Marshal(capabilities)
		updates["capabilities"] = string(data)
	}
	return updates
}

func generateForwardCleanAgentToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate clean agent token: %w", err)
	}
	return "v2fa_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeOptionalUint(value *uint) *uint {
	if value == nil || *value == 0 {
		return nil
	}
	v := *value
	return &v
}

func isForwardCleanAgentRevoked(agent *model.ForwardCleanAgent) bool {
	return agent == nil || agent.Status == model.ForwardCleanAgentStatusRevoked || agent.RevokedAt != nil
}
