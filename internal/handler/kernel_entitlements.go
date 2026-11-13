package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/entitlement"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EntitlementsHandler is the administrator API of feature licensing
// (internal/entitlement): the ANX licensing settings, the devices that
// asked for a license, and revoking one. Everything is off by default.
type EntitlementsHandler struct {
	db  func() *gorm.DB
	kek func() string
	now func() time.Time
}

// NewEntitlementsHandler reads the loaded database and root key on each
// request.
func NewEntitlementsHandler() *EntitlementsHandler {
	return &EntitlementsHandler{db: database.Get, now: time.Now}
}

// entitlementSettingsView is the ANX licensing state an administrator sees.
type entitlementSettingsView struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Require   bool   `json:"require"`
	TrialDays int    `json:"trial_days"`
	// PlanIDs are the plans that include the feature; empty is every plan.
	PlanIDs []uint `json:"plan_ids"`
	// CA is the hex of the license CA's public key; "" without a root key.
	CA string `json:"ca"`
	// RootKey says whether the root key is loaded: without it no license
	// can be signed and nodes run unlicensed.
	RootKey bool                     `json:"root_key"`
	Devices entitlement.DeviceCounts `json:"devices"`
	// Validity and SeatWindowDays are the fixed rules, for the page's text.
	ValidityDays   int `json:"validity_days"`
	SeatWindowDays int `json:"seat_window_days"`
	MaxTrialDays   int `json:"max_trial_days"`
}

type entitlementSettingsRequest struct {
	Enabled   *bool   `json:"enabled"`
	Require   *bool   `json:"require"`
	TrialDays *int    `json:"trial_days"`
	PlanIDs   *[]uint `json:"plan_ids"`
}

// entitlementDeviceView is one device of the list.
type entitlementDeviceView struct {
	ID        uint   `json:"id"`
	UserID    uint   `json:"user_id"`
	Email     string `json:"email"`
	DeviceID  string `json:"device_id"`
	Alg       string `json:"alg"`
	Trial     bool   `json:"trial"`
	NotAfter  int64  `json:"not_after"`
	Revoked   bool   `json:"revoked"`
	RevokedAt *int64 `json:"revoked_at"`
	CreatedAt int64  `json:"created_at"`
	// LastSeenAt is the last license request (Unix seconds).
	LastSeenAt int64 `json:"last_seen_at"`
	// Seated says whether the device takes a seat now.
	Seated bool `json:"seated"`
}

func (h *EntitlementsHandler) clock() time.Time {
	if h.now == nil {
		return time.Now().UTC()
	}
	return h.now().UTC()
}

// feature answers the database for a known feature, after answering an
// error itself. Only "anx" exists for now.
func (h *EntitlementsHandler) feature(c *gin.Context) (*gorm.DB, bool) {
	if c.Param("feature") != entitlement.FeatureANX {
		kernelError(c, http.StatusNotFound, "feature_not_found", "unknown licensed feature (only anx)")
		return nil, false
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	return db.WithContext(c.Request.Context()), true
}

func (h *EntitlementsHandler) view(db *gorm.DB, settings entitlement.Settings) (entitlementSettingsView, error) {
	counts, err := entitlement.Counts(db, entitlement.FeatureANX, h.clock())
	if err != nil {
		return entitlementSettingsView{}, err
	}
	svc := &entitlement.Service{DB: db, KEK: h.kek}
	ca, _ := svc.LicenseCAHex()
	planIDs := settings.PlanIDs
	if planIDs == nil {
		planIDs = []uint{}
	}
	return entitlementSettingsView{
		Feature: entitlement.FeatureANX, Enabled: settings.Enabled, Require: settings.Require,
		TrialDays: settings.TrialDays, PlanIDs: planIDs, CA: ca, RootKey: ca != "", Devices: counts,
		ValidityDays:   int(entitlement.LicenseValidity / (24 * time.Hour)),
		SeatWindowDays: int(entitlement.SeatWindow / (24 * time.Hour)),
		MaxTrialDays:   entitlement.MaxTrialDays,
	}, nil
}

// Get godoc
// @Summary A licensed feature's settings
// @Description The ANX licensing settings (anx spec §2.4.1): whether licensing is on, whether nodes require a license, the trial length, the plans that include ANX (empty = every plan), the license CA's public key (empty without a root key) and the device counts. Off by default.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param feature path string true "anx"
// @Success 200 {object} entitlementSettingsView
// @Router /api/v4/kernel/entitlements/{feature} [get]
func (h *EntitlementsHandler) Get(c *gin.Context) {
	db, ok := h.feature(c)
	if !ok {
		return
	}
	settings, err := entitlement.LoadSettings(db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	view, err := h.view(db, settings)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// Set godoc
// @Summary Change a licensed feature's settings
// @Description Changes the given fields only. enabled turns ANX licensing on: subscriptions answer license requests and ANX nodes trust the license CA. require has the nodes refuse unlicensed sessions (not beside wg_listen_port). trial_days is 0 to 30 (0 = no trial). plan_ids lists the plans that include ANX ([] = every plan). Nodes get the change with their next configuration. Audited (module entitlement, action entitlement_settings_set).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param feature path string true "anx"
// @Param request body entitlementSettingsRequest true "fields to change"
// @Success 200 {object} entitlementSettingsView
// @Router /api/v4/kernel/entitlements/{feature} [put]
func (h *EntitlementsHandler) Set(c *gin.Context) {
	db, ok := h.feature(c)
	if !ok {
		return
	}
	var request entitlementSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "the body must be a JSON object")
		return
	}
	if request.Enabled == nil && request.Require == nil && request.TrialDays == nil && request.PlanIDs == nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "nothing to change")
		return
	}
	if request.TrialDays != nil && (*request.TrialDays < 0 || *request.TrialDays > entitlement.MaxTrialDays) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "trial_days must be 0 to "+strconv.Itoa(entitlement.MaxTrialDays))
		return
	}
	if request.PlanIDs != nil {
		for _, id := range *request.PlanIDs {
			if id == 0 {
				kernelError(c, http.StatusBadRequest, "invalid_request", "plan_ids must be positive")
				return
			}
		}
		if len(*request.PlanIDs) > 0 {
			var found int64
			if err := db.Model(&model.Plan{}).Where("id IN ?", *request.PlanIDs).Distinct("id").Count(&found).Error; err != nil {
				kernelDBError(c, err)
				return
			}
			if int(found) != len(uniqueIDs(*request.PlanIDs)) {
				kernelError(c, http.StatusBadRequest, "plan_not_found", "a plan in plan_ids does not exist")
				return
			}
		}
	}
	var settings entitlement.Settings
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		settings, err = entitlement.SaveSettings(tx, entitlement.SettingsPatch{
			Enabled: request.Enabled, Require: request.Require, TrialDays: request.TrialDays, PlanIDs: request.PlanIDs,
		})
		if err != nil {
			return err
		}
		content, _ := json.Marshal(map[string]any{
			"feature": entitlement.FeatureANX, "enabled": settings.Enabled, "require": settings.Require,
			"trial_days": settings.TrialDays, "plan_ids": settings.PlanIDs,
		})
		return tx.Create(entitlementAudit(c, "entitlement_settings_set", "entitlement_feature", nil, string(content))).Error
	})
	if errors.Is(err, entitlement.ErrInvalidSettings) {
		kernelError(c, http.StatusBadRequest, "invalid_request", "invalid settings")
		return
	}
	if err != nil {
		kernelDBError(c, err)
		return
	}
	view, err := h.view(db, settings)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

func uniqueIDs(ids []uint) map[uint]struct{} {
	set := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// Devices godoc
// @Summary The devices of a licensed feature
// @Description Devices that asked for an ANX license, newest activity first, with the user's email. user_id limits the list to one user. page (from 1) and page_size (1 to 100, default 20) page it.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param feature path string true "anx"
// @Param user_id query int false "one user's devices"
// @Param page query int false "page, from 1"
// @Param page_size query int false "1 to 100, default 20"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entitlements/{feature}/devices [get]
func (h *EntitlementsHandler) Devices(c *gin.Context) {
	db, ok := h.feature(c)
	if !ok {
		return
	}
	query := entitlement.DeviceQuery{Feature: entitlement.FeatureANX, Limit: 20}
	if raw := c.Query("user_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "user_id must be a positive integer")
			return
		}
		query.UserID = uint(id)
	}
	page := 1
	if raw := c.Query("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "page must be a positive integer")
			return
		}
		page = n
	}
	if raw := c.Query("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			kernelError(c, http.StatusBadRequest, "invalid_request", "page_size must be 1 to 100")
			return
		}
		query.Limit = n
	}
	query.Offset = (page - 1) * query.Limit
	rows, total, err := entitlement.ListDevices(db, query)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	emails := map[uint]string{}
	if len(rows) > 0 {
		ids := make([]uint, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UserID)
		}
		var users []model.User
		if err := db.Select("id", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
			kernelDBError(c, err)
			return
		}
		for _, user := range users {
			emails[user.ID] = user.Email
		}
	}
	now := h.clock()
	items := make([]entitlementDeviceView, 0, len(rows))
	for _, row := range rows {
		items = append(items, h.deviceView(row, emails[row.UserID], now))
	}
	kernelData(c, http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": query.Limit})
}

func (h *EntitlementsHandler) deviceView(row model.EntitlementDevice, email string, now time.Time) entitlementDeviceView {
	view := entitlementDeviceView{
		ID: row.ID, UserID: row.UserID, Email: email, DeviceID: row.DeviceID, Alg: row.Alg, Trial: row.Trial,
		NotAfter: row.NotAfter, Revoked: row.RevokedAt != nil, CreatedAt: row.CreatedAt.Unix(), LastSeenAt: row.LastSeenAt.Unix(),
		Seated: row.RevokedAt == nil && row.LastSeenAt.After(now.Add(-entitlement.SeatWindow)),
	}
	if row.RevokedAt != nil {
		at := row.RevokedAt.Unix()
		view.RevokedAt = &at
	}
	return view
}

// Revoke godoc
// @Summary Revoke a device's license
// @Description The device gets no new license and frees its seat; the license it holds runs out within 7 days. Audited (module entitlement, action entitlement_device_revoke).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param feature path string true "anx"
// @Param id path int true "device row id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entitlements/{feature}/devices/{id}/revoke [post]
func (h *EntitlementsHandler) Revoke(c *gin.Context) { h.setRevoked(c, true) }

// Restore godoc
// @Summary Lift a device's revocation
// @Description The device gets a license again at its next subscription refresh, if a seat is free. Audited (module entitlement, action entitlement_device_restore).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param feature path string true "anx"
// @Param id path int true "device row id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/entitlements/{feature}/devices/{id}/restore [post]
func (h *EntitlementsHandler) Restore(c *gin.Context) { h.setRevoked(c, false) }

func (h *EntitlementsHandler) setRevoked(c *gin.Context, revoked bool) {
	db, ok := h.feature(c)
	if !ok {
		return
	}
	id, ok := parseKernelID(c, "id")
	if !ok {
		return
	}
	action := "entitlement_device_restore"
	if revoked {
		action = "entitlement_device_revoke"
	}
	var (
		row     model.EntitlementDevice
		changed bool
	)
	now := h.clock()
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		row, changed, err = entitlement.SetRevoked(tx, entitlement.FeatureANX, id, revoked, now)
		if err != nil || !changed {
			return err
		}
		content, _ := json.Marshal(map[string]any{"feature": entitlement.FeatureANX, "device_id": row.DeviceID, "user_id": row.UserID})
		return tx.Create(entitlementAudit(c, action, "entitlement_device", &row.ID, string(content))).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		kernelError(c, http.StatusNotFound, "device_not_found", "the device does not exist")
		return
	}
	if err != nil {
		kernelDBError(c, err)
		return
	}
	var user model.User
	_ = db.Select("id", "email").Where("id = ?", row.UserID).Limit(1).Find(&user).Error
	kernelData(c, http.StatusOK, gin.H{"device": h.deviceView(row, user.Email, now), "changed": changed})
}

func entitlementAudit(c *gin.Context, action, targetType string, targetID *uint, content string) *model.OperationLog {
	var userID *uint
	if id := kernelActorID(c); id != 0 {
		userID = &id
	}
	return &model.OperationLog{
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: action, Module: "entitlement",
		TargetType: targetType, TargetID: targetID, Content: content, IP: truncateText(c.ClientIP(), 45), Status: 1,
	}
}
