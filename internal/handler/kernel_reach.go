package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/reachprobe"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ReachHandler serves the reachability probe (internal/reachprobe): which
// proxy nodes probe the others from their region, and what they found.
type ReachHandler struct {
	db     func() *gorm.DB
	prober func() reachprobe.Prober
}

// NewReachHandler reads the loaded database and the live Agent streams on
// each request.
func NewReachHandler() *ReachHandler {
	return &ReachHandler{db: database.Get, prober: func() reachprobe.Prober { return kernelnodeops.ReachProbes{} }}
}

type reachAnswer struct {
	Vantages []reachprobe.Vantage    `json:"vantages"`
	Nodes    []reachprobe.NodeStatus `json:"nodes"`
}

type reachVantagesRequest struct {
	Vantages []reachprobe.Vantage `json:"vantages"`
}

func (h *ReachHandler) service(c *gin.Context) (*reachprobe.Service, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	return &reachprobe.Service{DB: db, Prober: h.prober()}, true
}

func (h *ReachHandler) answer(c *gin.Context, s *reachprobe.Service) {
	vantages, err := s.Vantages(c.Request.Context())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	nodes, err := s.Statuses(c.Request.Context())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, reachAnswer{Vantages: vantages, Nodes: nodes})
}

// Get godoc
// @Summary Node reachability by region
// @Description The vantages (proxy nodes whose Agents probe the other nodes from their region, domestic or overseas, and whether each runs reach.probe now) and each probed node's status: ok, region_unreachable (reachable from overseas only), overseas_unreachable, unreachable or unknown, with each region's view. Answers older than 30 minutes do not count. The probe only opens and closes TCP connections to the ports of the node's enabled protocols, every 10 minutes.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} reachAnswer
// @Router /api/v4/kernel/reach [get]
func (h *ReachHandler) Get(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	h.answer(c, s)
}

// SetVantages godoc
// @Summary Set the reachability vantages
// @Description Replaces the vantages: each proxy node with its region (domestic or overseas). A node that is no longer a vantage, or changed region, loses its answers. Audited (module reach_probe, action reach_vantages_set).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body reachVantagesRequest true "vantages"
// @Success 200 {object} reachAnswer
// @Router /api/v4/kernel/reach/vantages [put]
func (h *ReachHandler) SetVantages(c *gin.Context) {
	var request reachVantagesRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Vantages == nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "vantages is required")
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	if err := s.SetVantages(c.Request.Context(), request.Vantages); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var userID *uint
	if id := kernelActorID(c); id != 0 {
		userID = &id
	}
	content, _ := json.Marshal(request.Vantages)
	if err := s.DB.WithContext(c.Request.Context()).Create(&model.OperationLog{
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: "reach_vantages_set", Module: "reach_probe",
		Content: string(content), IP: truncateText(c.ClientIP(), 45), Status: 1,
	}).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	h.answer(c, s)
}
