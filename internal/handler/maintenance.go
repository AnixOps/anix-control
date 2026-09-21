package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/maintenance"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MaintenanceHandler struct{ db *gorm.DB }

func NewMaintenanceHandler() *MaintenanceHandler { return &MaintenanceHandler{db: database.Get()} }
func (h *MaintenanceHandler) authorized(c *gin.Context, owner bool) bool {
	role, err := service.MaintenanceActorRole(h.db, kernelActorID(c))
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "maintenance_unavailable"})
		return false
	}
	if role == "owner" || (!owner && role == "technician") {
		return true
	}
	s, err := service.GetMaintenanceSettings(h.db)
	if err == nil && s.OwnerID == 0 && kernelActorIsAdmin(c) {
		return true
	}
	c.AbortWithStatusJSON(403, gin.H{"error": "maintenance_access_denied"})
	return false
}
func maintenanceJSON(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return false
	}
	return true
}
func maintenanceResponse(c *gin.Context, data any, err error) {
	if err != nil {
		code := "maintenance_operation_failed"
		status := 409
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = 404
			code = "not_found"
		}
		c.JSON(status, gin.H{"error": code})
		return
	}
	c.JSON(200, gin.H{"data": data})
}
func (h *MaintenanceHandler) Me(c *gin.Context) {
	if !h.authorized(c, false) {
		return
	}
	role, _ := service.MaintenanceActorRole(h.db, kernelActorID(c))
	if role == "" {
		role = "bootstrap"
	}
	c.JSON(200, gin.H{"data": gin.H{"role": role, "user_id": kernelActorID(c)}})
}
func (h *MaintenanceHandler) Settings(c *gin.Context) {
	if !h.authorized(c, false) {
		return
	}
	s, err := service.GetMaintenanceSettings(h.db)
	maintenanceResponse(c, s, err)
}
func (h *MaintenanceHandler) SaveSettings(c *gin.Context) {
	if !h.authorized(c, true) {
		return
	}
	var input model.MaintenanceSettings
	if !maintenanceJSON(c, &input) {
		return
	}
	s, err := service.SaveMaintenanceSettings(h.db, kernelActorID(c), kernelActorIsAdmin(c), input, time.Now())
	maintenanceResponse(c, s, err)
}
func (h *MaintenanceHandler) Verify(c *gin.Context) {
	if !h.authorized(c, true) {
		return
	}
	var input struct {
		Channel string `json:"channel"`
	}
	if !maintenanceJSON(c, &input) {
		return
	}
	err := service.QueueMaintenanceVerification(h.db, kernelActorID(c), input.Channel, time.Now())
	maintenanceResponse(c, gin.H{"queued": err == nil}, err)
}
func (h *MaintenanceHandler) Confirm(c *gin.Context) {
	if !h.authorized(c, true) {
		return
	}
	var input struct {
		Channel string `json:"channel"`
		Code    string `json:"code"`
	}
	if !maintenanceJSON(c, &input) {
		return
	}
	err := service.ConfirmMaintenanceVerification(h.db, kernelActorID(c), input.Channel, input.Code, time.Now())
	maintenanceResponse(c, gin.H{"verified": err == nil}, err)
}
func (h *MaintenanceHandler) Tickets(c *gin.Context) {
	if !h.authorized(c, false) {
		return
	}
	var tickets []model.MaintenanceTicket
	query := h.db.Order("id DESC").Limit(200)
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Find(&tickets).Error
	maintenanceResponse(c, tickets, err)
}
func (h *MaintenanceHandler) Detail(c *gin.Context) {
	if !h.authorized(c, false) {
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var ticket model.MaintenanceTicket
	if err := h.db.First(&ticket, id).Error; err != nil {
		maintenanceResponse(c, nil, err)
		return
	}
	var rows []model.MaintenanceEvent
	var records []model.MaintenanceRecord
	var deliveries []model.MaintenanceDelivery
	if err := h.db.Where("ticket_id = ?", id).Order("occurred_at DESC").Limit(500).Find(&rows).Error; err != nil {
		maintenanceResponse(c, nil, err)
		return
	}
	events := []maintenance.Event{}
	for _, row := range rows {
		var e maintenance.Event
		if json.Unmarshal([]byte(row.Body), &e) == nil {
			events = append(events, e.Safe())
		}
	}
	if err := h.db.Where("ticket_id = ?", id).Order("id DESC").Limit(500).Find(&records).Error; err != nil {
		maintenanceResponse(c, nil, err)
		return
	}
	if err := h.db.Where("ticket_id = ?", id).Order("id DESC").Limit(500).Find(&deliveries).Error; err != nil {
		maintenanceResponse(c, nil, err)
		return
	}
	maintenanceResponse(c, gin.H{"ticket": ticket, "events": events, "records": records, "deliveries": deliveries}, nil)
}
func (h *MaintenanceHandler) Claim(c *gin.Context) { h.mutateTicket(c, "claim") }
func (h *MaintenanceHandler) Close(c *gin.Context) { h.mutateTicket(c, "close") }
func (h *MaintenanceHandler) Note(c *gin.Context)  { h.mutateTicket(c, "note") }

var noteSecrets = regexp.MustCompile(`(?i)(?:https?://[^\s]+|(?:password|passwd|token|secret|api[_-]?key|authorization)\s*[:=]\s*\S+|-----BEGIN[\s\S]*?-----END[^\n]+-----)`)

func (h *MaintenanceHandler) mutateTicket(c *gin.Context, kind string) {
	if !h.authorized(c, false) {
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if kind != "claim" {
		if !maintenanceJSON(c, &input) {
			return
		}
		input.Note = strings.TrimSpace(input.Note)
		if len(input.Note) == 0 || len(input.Note) > 4000 {
			c.JSON(400, gin.H{"error": "processing_note_required"})
			return
		}
		input.Note = noteSecrets.ReplaceAllString(input.Note, "[已隐藏敏感信息]")
	}
	actor := kernelActorID(c)
	now := time.Now()
	var ticket model.MaintenanceTicket
	err := service.WithRetryableTransaction(h.db, func(tx *gorm.DB) error {
		var initial model.MaintenanceTicket
		if err := tx.First(&initial, id).Error; err != nil {
			return err
		}
		var slot model.MaintenanceStream
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, "key = ?", initial.StreamKey).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, id).Error; err != nil {
			return err
		}
		if ticket.Status == "closed" {
			return errors.New("ticket_closed")
		}
		switch kind {
		case "claim":
			if ticket.ClaimedBy != nil {
				if *ticket.ClaimedBy == actor {
					return nil
				}
				return errors.New("already_claimed")
			}
			ticket.ClaimedBy = &actor
			ticket.ClaimedAt = &now
		case "close":
			if ticket.Status != "recovered" {
				return errors.New("recovery_required")
			}
			ticket.Status = "closed"
			ticket.ClosedAt = &now
		}
		if err := tx.Save(&ticket).Error; err != nil {
			return err
		}
		return tx.Create(&model.MaintenanceRecord{TicketID: id, ActorID: actor, Kind: kind, Note: input.Note, CreatedAt: now}).Error
	})
	maintenanceResponse(c, ticket, err)
}
func (h *MaintenanceHandler) SetNodeState(c *gin.Context) {
	if !h.authorized(c, true) {
		return
	}
	node, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || node == 0 {
		c.JSON(400, gin.H{"error": "invalid_node"})
		return
	}
	var input struct {
		Maintenance bool `json:"maintenance"`
		Disabled    bool `json:"disabled"`
	}
	if !maintenanceJSON(c, &input) {
		return
	}
	state := model.MaintenanceNodeState{NodeID: uint(node), Maintenance: input.Maintenance, Disabled: input.Disabled, UpdatedAt: time.Now()}
	err = service.WithRetryableTransaction(h.db, func(tx *gorm.DB) error {
		var n model.Node
		if err := tx.First(&n, node).Error; err != nil {
			return err
		}
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return tx.Create(&model.MaintenanceRecord{ActorID: kernelActorID(c), Kind: "node_exclusion", Note: "节点 " + strconv.FormatUint(node, 10) + " 运维排除设置变更", CreatedAt: time.Now()}).Error
	})
	maintenanceResponse(c, state, err)
}
