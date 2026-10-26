package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/realitytarget"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The audit of Reality target probes: the audit middleware skips
// /api/v4/kernel, so the handler writes one operation log row per run
// (module reality_target, action reality_target_probe).
const (
	realityTargetAuditModule = "reality_target"
	realityTargetAuditAction = "reality_target_probe"
	realityTargetApplyAction = "reality_target_apply"
	// realityTargetRunLimit bounds a whole run: MaxCandidates probes,
	// Concurrency at a time, each at most MaxTimeout, the REALITY handshake
	// and the Agent's slack (3 rounds of 24 s).
	realityTargetRunLimit = 90 * time.Second
)

// RealityTargetsHandler is the administrator API of Reality target probing
// (目标站探测): a proxy node's Agent probes candidate REALITY targets from
// the node's network, Control ranks them and recommends one, and an
// administrator applies one to a Reality protocol of the node.
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
// @Description The node's Agent probes each candidate (reality.probe: TLS 1.3, ALPN h2, an X25519 key exchange, a certificate valid for the name, no redirect to another host, then a real REALITY handshake through it) from the node's network, 8 at a time; Control scores the answers, stores the latest per candidate and answers the node's ranked report, with a probe that gave no answer marked verdict error. candidates are host names or host:port (at most 24 in all); include_defaults (default true) adds the built-in list; timeout_ms is each probe's TLS time, 100 to 10000, default 5000 (the REALITY handshake adds up to 12 s). 409 node_offline or reality_probe_unsupported when the Agent cannot probe; 409 probe_busy while another run of the node is going. Nothing in the node's configuration changes. Audited (module reality_target).
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

type realityTargetApplyRequest struct {
	ProtocolID uint   `json:"protocol_id"`
	ServerName string `json:"server_name"`
	// Port is the target's port; 443 when absent.
	Port int `json:"port"`
	// Force applies a target this node has not probed with verdict ok.
	Force bool `json:"force"`
}

// Apply godoc
// @Summary Apply a Reality target to a protocol of a proxy node
// @Description Writes the target into one Reality protocol (tls 2) of the node: reality_settings dest (server_name:port), server_name and, where listed, server_names, and tls_settings server_name (the SNI of the subscription links). The private key and every other setting stay as stored; the node's Agent picks the change up with its configuration and clients with their next subscription refresh. The node must have probed the target with verdict ok unless force is true (409 target_not_probed). 404 protocol_not_found for a protocol that is not a Reality protocol of the node. Audited (module reality_target, action reality_target_apply).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id>"
// @Param request body realityTargetApplyRequest true "protocol_id, server_name, port, force"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/nodes/{id}/reality-targets/apply [post]
func (h *RealityTargetsHandler) Apply(c *gin.Context) {
	node, ok := proxyNodeRef(c)
	if !ok {
		return
	}
	var request realityTargetApplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.ProtocolID == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "protocol_id is required")
		return
	}
	raw := request.ServerName
	if request.Port != 0 {
		raw = net.JoinHostPort(request.ServerName, strconv.Itoa(request.Port))
	}
	target, err := realitytarget.ParseCandidate(raw)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_params", err.Error())
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
	audit, err := h.beginAction(c, db, node, realityTargetApplyAction, map[string]any{
		"node": node.String(), "protocol_id": request.ProtocolID, "target": target.String(), "force": request.Force, "outcome": "requested",
	})
	if err != nil {
		kernelError(c, http.StatusInternalServerError, "audit_unavailable", "the operation log could not be written; nothing was changed")
		return
	}
	nodes := service.NewNodeServiceFor(db)
	result, err := (&realitytarget.Service{DB: db}).Apply(c.Request.Context(), uint(node.ID),
		realitytarget.ApplyRequest{ProtocolID: request.ProtocolID, Target: target, Force: request.Force}, nodes.UpdateProtocol)
	switch {
	case errors.Is(err, realitytarget.ErrNotRealityProtocol):
		audit.end("protocol_not_found", false, nil)
		kernelError(c, http.StatusNotFound, "protocol_not_found", err.Error())
		return
	case errors.Is(err, realitytarget.ErrTargetNotProbed):
		audit.end("target_not_probed", false, nil)
		kernelError(c, http.StatusConflict, "target_not_probed", err.Error())
		return
	case errors.Is(err, service.ErrInvalidNodeProtocol):
		audit.end("invalid", false, map[string]any{"reason": truncateText(err.Error(), 200)})
		kernelError(c, http.StatusBadRequest, "invalid_params", err.Error())
		return
	case err != nil:
		audit.end("error", false, nil)
		kernelDBError(c, err)
		return
	}
	audit.end("completed", true, map[string]any{"previous": result.Previous.ServerName})
	kernelData(c, http.StatusOK, result)
}

// begin writes the operation log row of a run, as failed until it ends.
func (h *RealityTargetsHandler) begin(c *gin.Context, db *gorm.DB, node agentcontrol.AgentNode, content map[string]any) (*nodeDiagnosticAudit, error) {
	return h.beginAction(c, db, node, realityTargetAuditAction, content)
}

func (h *RealityTargetsHandler) beginAction(c *gin.Context, db *gorm.DB, node agentcontrol.AgentNode, action string, content map[string]any) (*nodeDiagnosticAudit, error) {
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
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: action, Module: realityTargetAuditModule,
		TargetType: "proxy_node", TargetID: &targetID, Content: string(encoded), IP: truncateText(c.ClientIP(), 45), Status: 2,
	}
	if err := db.WithContext(c.Request.Context()).Create(&row).Error; err != nil {
		slog.Warn("reality target audit row not written", slog.String("error", err.Error()))
		return nil, err
	}
	return &nodeDiagnosticAudit{db: db, id: row.ID, content: content}, nil
}
