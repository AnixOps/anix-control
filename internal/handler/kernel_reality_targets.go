package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/realitytarget"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The audit of Reality target probes: the audit middleware skips
// /api/v4/kernel, so the handler writes one operation log row per run
// (module reality_target, action reality_target_probe).
const (
	realityTargetAuditModule = "reality_target"
	realityTargetAuditAction = "reality_target_probe"
	// realityTargetRunLimit bounds a whole run: MaxCandidates probes,
	// Concurrency at a time, each at most MaxTimeout plus the Agent's slack.
	realityTargetRunLimit = 90 * time.Second
)

// RealityTargetsHandler is the administrator API of Reality target probing
// (目标站探测, phase 1): a proxy node's Agent probes candidate REALITY
// targets from the node's network, and Control ranks them and recommends
// one. It changes no node configuration.
type RealityTargetsHandler struct {
	db       func() *gorm.DB
	probes   func() realityTargetProber
	inflight sync.Map
}

// realityTargetProber runs reality.probe (kernelnodeops.RealityProbes).
type realityTargetProber interface {
	realitytarget.Prober
	Support(node agentcontrol.AgentNode) kernelnodeops.RealityProbeSupport
}

// NewRealityTargetsHandler reads the loaded database and the kernel's live
// Agent Control streams on each request.
func NewRealityTargetsHandler() *RealityTargetsHandler {
	return &RealityTargetsHandler{db: database.Get, probes: func() realityTargetProber { return kernelnodeops.RealityProbes{} }}
}

type realityTargetProbeRequest struct {
	// Candidates are host names, or host:port, probed before the defaults.
	Candidates []string `json:"candidates"`
	// IncludeDefaults adds realitytarget.DefaultCandidates; true when absent.
	IncludeDefaults *bool `json:"include_defaults"`
	// TimeoutMS is each probe's time on the Agent: 100 to 10000, default 5000.
	TimeoutMS *int `json:"timeout_ms"`
}

// proxyNodeRef parses the node segment and accepts only a proxy node: only
// a proxy node serves REALITY.
func proxyNodeRef(c *gin.Context) (agentcontrol.AgentNode, bool) {
	node, ok := parseNodeRef(c)
	if !ok {
		return node, false
	}
	if node.Kind != agentcontrol.NodeKindProxy {
		kernelError(c, http.StatusBadRequest, "invalid_request", "Reality targets are probed from a proxy node (proxy-<id>)")
		return node, false
	}
	return node, true
}

// List godoc
// @Summary A proxy node's Reality targets
// @Description The node's latest Reality target probes, ranked (targets that passed first by score, then failed ones), the target its Reality protocols use now (current), the recommended target, and whether its Agent runs reality.probe now (support).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/reality-targets [get]
func (h *RealityTargetsHandler) List(c *gin.Context) {
	node, ok := proxyNodeRef(c)
	if !ok {
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	if !h.requireNode(c, db, node) {
		return
	}
	probes := h.probes()
	report, err := (&realitytarget.Service{DB: db, Prober: probes}).Report(c.Request.Context(), uint(node.ID))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"report": report, "support": probes.Support(node), "default_candidates": realitytarget.DefaultCandidates})
}

func (h *RealityTargetsHandler) requireNode(c *gin.Context, db *gorm.DB, node agentcontrol.AgentNode) bool {
	exists, err := nodeExists(db, node)
	if err != nil {
		kernelDBError(c, err)
		return false
	}
	if !exists {
		kernelError(c, http.StatusNotFound, "node_not_found", "the node does not exist")
		return false
	}
	return true
}

// Probe godoc
// @Summary Probe Reality targets from a proxy node
// @Description The node's Agent probes each candidate (reality.probe: TLS 1.3, ALPN h2, an X25519 key exchange, a certificate valid for the name, no redirect to another host) from the node's network, 4 at a time; Control scores the answers, stores the latest per candidate and answers the node's ranked report, with a probe that gave no answer marked verdict error. candidates are host names or host:port (at most 24 in all); include_defaults (default true) adds the built-in list; timeout_ms is each probe's time, 100 to 10000, default 5000. 409 node_offline or reality_probe_unsupported when the Agent cannot probe; 409 probe_busy while another run of the node is going. Nothing in the node's configuration changes. Audited (module reality_target).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param request body realityTargetProbeRequest false "candidates, include_defaults, timeout_ms"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/reality-targets/probe [post]
func (h *RealityTargetsHandler) Probe(c *gin.Context) {
	node, ok := proxyNodeRef(c)
	if !ok {
		return
	}
	var request realityTargetProbeRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	withDefaults := request.IncludeDefaults == nil || *request.IncludeDefaults
	candidates, err := realitytarget.Candidates(request.Candidates, withDefaults)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_params", err.Error())
		return
	}
	timeout := realitytarget.DefaultTimeout
	if request.TimeoutMS != nil {
		timeout = time.Duration(*request.TimeoutMS) * time.Millisecond
		if timeout < realitytarget.MinTimeout || timeout > realitytarget.MaxTimeout {
			kernelError(c, http.StatusBadRequest, "invalid_params", "timeout_ms must be between 100 and 10000")
			return
		}
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	if !h.requireNode(c, db, node) {
		return
	}
	probes := h.probes()
	support := probes.Support(node)
	if !support.Connected {
		kernelError(c, http.StatusConflict, "node_offline", "the node's Agent is not connected")
		return
	}
	if !support.Capable {
		message := "the node's Agent does not advertise reality.probe"
		if support.AgentVersion != "" {
			message += " (Agent " + support.AgentVersion + ")"
		}
		kernelError(c, http.StatusConflict, "reality_probe_unsupported", message)
		return
	}
	if _, busy := h.inflight.LoadOrStore(node.String(), struct{}{}); busy {
		kernelError(c, http.StatusConflict, "probe_busy", "another probe run of this node is going: wait for it to finish")
		return
	}
	defer h.inflight.Delete(node.String())

	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.String())
	}
	audit, err := h.begin(c, db, node, map[string]any{"node": node.String(), "candidates": names, "outcome": "requested"})
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "audit_unavailable", "the operation log could not be written; nothing was sent")
		return
	}

	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(realityTargetRunLimit + nodeDiagnosticWriteSlack))
	ctx, cancel := context.WithTimeout(c.Request.Context(), realityTargetRunLimit)
	defer cancel()
	report, err := (&realitytarget.Service{DB: db, Prober: probes}).Run(ctx, uint(node.ID), candidates, timeout)
	if err != nil {
		audit.end("error", false, nil)
		if errors.Is(err, context.Canceled) {
			c.Abort()
			return
		}
		kernelDBError(c, err)
		return
	}
	extra := map[string]any{}
	if report.Recommended != nil {
		extra["recommended"] = report.Recommended.ServerName
	}
	audit.end("completed", true, extra)
	kernelData(c, http.StatusOK, gin.H{"report": report, "support": support})
}

// begin writes the operation log row of a run, as failed until it ends.
func (h *RealityTargetsHandler) begin(c *gin.Context, db *gorm.DB, node agentcontrol.AgentNode, content map[string]any) (*nodeDiagnosticAudit, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var userID *uint
	if id := kernelActorID(c); id != 0 {
		userID = &id
	}
	targetID := uint(node.ID)
	row := model.OperationLog{
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: realityTargetAuditAction, Module: realityTargetAuditModule,
		TargetType: "proxy_node", TargetID: &targetID, Content: string(encoded), IP: truncateText(c.ClientIP(), 45), Status: 2,
	}
	if err := db.WithContext(c.Request.Context()).Create(&row).Error; err != nil {
		slog.Warn("reality target audit row not written", slog.String("error", err.Error()))
		return nil, err
	}
	return &nodeDiagnosticAudit{db: db, id: row.ID, content: content}, nil
}
