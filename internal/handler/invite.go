package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InviteHandler 閭€璇峰鐞嗗櫒
type InviteHandler struct {
	inviteService *service.InviteService
	configService *service.SystemConfigService
}

const inviteFrontendConfigKey = "invite.frontend.config"

type inviteFrontendConfig struct {
	CodePrefix      string   `json:"code_prefix"`
	CodeLength      int      `json:"code_length"`
	WithdrawFee     float64  `json:"withdraw_fee"`
	WithdrawMethods []string `json:"withdraw_methods"`
}

func defaultInviteFrontendConfig() inviteFrontendConfig {
	return inviteFrontendConfig{
		CodePrefix:      "INV",
		CodeLength:      8,
		WithdrawFee:     0,
		WithdrawMethods: []string{"alipay"},
	}
}

func defaultInviteConfig() *model.InviteConfig {
	return &model.InviteConfig{
		Enabled:             true,
		AutoGenerate:        true,
		CodeCount:           5,
		CodeExpireDays:      0,
		CommissionEnabled:   true,
		CommissionType:      1,
		CommissionRate:      0.1,
		CommissionFixed:     0,
		CommissionMinAmount: 10,
	}
}

func inviteBoolFromAny(v interface{}) (bool, bool) {
	switch raw := v.(type) {
	case bool:
		return raw, true
	case float64:
		if raw == 1 {
			return true, true
		}
		if raw == 0 {
			return false, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "y", "on":
			return true, true
		case "0", "false", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

func inviteIntFromAny(v interface{}) (int, bool) {
	switch raw := v.(type) {
	case int:
		return raw, true
	case int32:
		return int(raw), true
	case int64:
		return int(raw), true
	case float64:
		return int(raw), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func inviteFloatFromAny(v interface{}) (float64, bool) {
	switch raw := v.(type) {
	case float64:
		return raw, true
	case float32:
		return float64(raw), true
	case int:
		return float64(raw), true
	case int32:
		return float64(raw), true
	case int64:
		return float64(raw), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func inviteCommissionTypeToText(t int) string {
	if t == 2 {
		return "fixed"
	}
	return "percent"
}

func inviteConfigResponse(cfg *model.InviteConfig, frontendCfg inviteFrontendConfig) gin.H {
	if cfg == nil {
		cfg = defaultInviteConfig()
	}

	commissionRatePercent := cfg.CommissionRate
	if commissionRatePercent <= 1 {
		commissionRatePercent = commissionRatePercent * 100
	}

	return gin.H{
		"id":                    cfg.ID,
		"enabled":               cfg.Enabled,
		"auto_generate":         cfg.AutoGenerate,
		"code_count":            cfg.CodeCount,
		"code_expire_days":      cfg.CodeExpireDays,
		"commission_enabled":    cfg.CommissionEnabled,
		"commission_type":       inviteCommissionTypeToText(cfg.CommissionType),
		"commission_type_code":  cfg.CommissionType,
		"commission_rate":       commissionRatePercent,
		"commission_rate_ratio": cfg.CommissionRate,
		"commission_fixed":      cfg.CommissionFixed,
		"commission_min":        cfg.CommissionMinAmount,
		"min_withdraw":          cfg.CommissionMinAmount,
		"first_order_bonus":     cfg.FirstOrderBonus,
		"first_traffic_bonus":   cfg.FirstTrafficBonus,
		"created_at":            cfg.CreatedAt,
		"updated_at":            cfg.UpdatedAt,
		// Frontend aliases used by Invite.vue.
		"code_prefix":      frontendCfg.CodePrefix,
		"code_length":      frontendCfg.CodeLength,
		"withdraw_fee":     frontendCfg.WithdrawFee,
		"withdraw_methods": frontendCfg.WithdrawMethods,
	}
}

func inviteStringSliceFromAny(v interface{}) ([]string, bool) {
	switch raw := v.(type) {
	case []string:
		return raw, true
	case []interface{}:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			trimmed := strings.TrimSpace(str)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, true
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return []string{}, true
		}
		var parsed []string
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed, true
		}
		parts := strings.Split(trimmed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p != "" {
				out = append(out, p)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func inviteWithdrawalStatusToText(status int) string {
	switch status {
	case 1:
		return "approved"
	case 2:
		return "rejected"
	default:
		return "pending"
	}
}

func inviteWithdrawalStatusFromText(status string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return 0, true
	case "approved":
		return 1, true
	case "rejected":
		return 2, true
	default:
		return 0, false
	}
}

func inviteWithdrawalResponse(record model.CommissionWithdraw) gin.H {
	return gin.H{
		"id":           record.ID,
		"user_id":      record.UserID,
		"amount":       record.Amount,
		"method":       record.Method,
		"account":      record.Account,
		"name":         record.Name,
		"remark":       record.Remark,
		"status":       inviteWithdrawalStatusToText(record.Status),
		"status_code":  record.Status,
		"processed_at": record.ProcessedAt,
		"created_at":   record.CreatedAt,
		"updated_at":   record.UpdatedAt,
	}
}

// NewInviteHandler 鍒涘缓澶勭悊鍣?
func NewInviteHandler() *InviteHandler {
	db := database.Get()
	return &InviteHandler{
		inviteService: service.NewInviteService(db),
		configService: service.NewSystemConfigService(db),
	}
}

func (h *InviteHandler) loadFrontendConfig() inviteFrontendConfig {
	cfg := defaultInviteFrontendConfig()
	if h.configService == nil {
		return cfg
	}
	var stored inviteFrontendConfig
	if err := h.configService.GetJSON(inviteFrontendConfigKey, &stored); err != nil {
		return cfg
	}
	if strings.TrimSpace(stored.CodePrefix) != "" {
		cfg.CodePrefix = strings.TrimSpace(stored.CodePrefix)
	}
	if stored.CodeLength > 0 {
		cfg.CodeLength = stored.CodeLength
	}
	if stored.WithdrawMethods != nil && len(stored.WithdrawMethods) > 0 {
		cfg.WithdrawMethods = stored.WithdrawMethods
	}
	cfg.WithdrawFee = stored.WithdrawFee
	return cfg
}

func (h *InviteHandler) saveFrontendConfig(cfg inviteFrontendConfig) error {
	if h.configService == nil {
		return nil
	}
	if strings.TrimSpace(cfg.CodePrefix) == "" {
		cfg.CodePrefix = "INV"
	}
	if cfg.CodeLength <= 0 {
		cfg.CodeLength = 8
	}
	if len(cfg.WithdrawMethods) == 0 {
		cfg.WithdrawMethods = []string{"alipay"}
	}
	return h.configService.SetJSON(
		inviteFrontendConfigKey,
		cfg,
		"invite",
		"Invite frontend configuration fields",
	)
}

// ========== 鐢ㄦ埛鎺ュ彛 ==========

// GetInviteInfo godoc
// @Summary 鑾峰彇閭€璇蜂俊鎭?
// @Description 鐢ㄦ埛鑾峰彇鑷繁鐨勯個璇风爜鍜屼剑閲戜俊鎭?
// @Tags 鐢ㄦ埛绔?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/invite [get]
func (h *InviteHandler) GetInviteInfo(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 鑾峰彇鐢ㄦ埛閭€璇风爜
	codes, err := h.inviteService.GetUserInviteCodes(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 鑾峰彇缁熻
	stats, err := h.inviteService.GetInviteStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 鑾峰彇鐢ㄦ埛浣ｉ噾浣欓
	var user model.User
	database.Get().Select("commission_balance").First(&user, userID)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"codes":              codes,
			"commission_balance": user.CommissionBalance,
			"stats":              stats,
		},
	})
}

// GenerateCode godoc
// @Summary 鐢熸垚閭€璇风爜
// @Description 鐢ㄦ埛鐢熸垚鏂扮殑閭€璇风爜
// @Tags 鐢ㄦ埛绔?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/invite/generate [post]
func (h *InviteHandler) GenerateCode(c *gin.Context) {
	userID := c.GetUint("user_id")

	code, err := h.inviteService.GenerateInviteCode(&userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": code})
}

// GetCommissionRecords godoc
// @Summary 鑾峰彇浣ｉ噾璁板綍
// @Description 鐢ㄦ埛鑾峰彇鑷繁鐨勪剑閲戣褰曞垪琛?
// @Tags 鐢ㄦ埛绔?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "椤电爜" default(1)
// @Param page_size query int false "姣忛〉鏁伴噺" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/invite/commissions [get]
func (h *InviteHandler) GetCommissionRecords(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	records, total, err := h.inviteService.GetCommissionRecords(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// RequestWithdraw godoc
// @Summary 鐢宠鎻愮幇
// @Description 鐢ㄦ埛鐢宠浣ｉ噾鎻愮幇
// @Tags 鐢ㄦ埛绔?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "鎻愮幇璇锋眰 {amount, method, account, name}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/invite/withdraw [post]
func (h *InviteHandler) RequestWithdraw(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req struct {
		Amount  float64 `json:"amount" binding:"required,min=1"`
		Method  string  `json:"method" binding:"required,oneof=alipay wechat bank"`
		Account string  `json:"account" binding:"required"`
		Name    string  `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	withdraw, err := h.inviteService.RequestWithdraw(userID, req.Amount, req.Method, req.Account, req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": withdraw})
}

// GetWithdrawRecords godoc
// @Summary 鑾峰彇鎻愮幇璁板綍
// @Description 鐢ㄦ埛鑾峰彇鑷繁鐨勬彁鐜拌褰曞垪琛?
// @Tags 鐢ㄦ埛绔?
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "椤电爜" default(1)
// @Param page_size query int false "姣忛〉鏁伴噺" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/invite/withdrawals [get]
func (h *InviteHandler) GetWithdrawRecords(c *gin.Context) {
	userID := c.GetUint("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	records, total, err := h.inviteService.GetWithdrawRecords(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ========== 绠＄悊鍛樻帴鍙?==========

// GetConfig godoc
// @Summary 鑾峰彇閭€璇烽厤缃?
// @Description 绠＄悊鍛樿幏鍙栭個璇风郴缁熼厤缃?
// @Tags 绠＄悊绔?绯荤粺
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/invite/config [get]
func (h *InviteHandler) GetConfig(c *gin.Context) {
	cfg, err := h.inviteService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cfg == nil {
		cfg = defaultInviteConfig()
	}

	frontendCfg := h.loadFrontendConfig()
	c.JSON(http.StatusOK, gin.H{"data": inviteConfigResponse(cfg, frontendCfg)})
}

// UpdateConfig godoc
// @Summary 鏇存柊閭€璇烽厤缃?
// @Description 绠＄悊鍛樻洿鏂伴個璇风郴缁熼厤缃?
// @Tags 绠＄悊绔?绯荤粺
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.InviteConfig true "閭€璇烽厤缃?
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/invite/config [put]
func (h *InviteHandler) UpdateConfig(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	currentCfg, err := h.inviteService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if currentCfg == nil {
		currentCfg = defaultInviteConfig()
	}
	cfg := *currentCfg
	frontendCfg := h.loadFrontendConfig()

	if raw, ok := req["enabled"]; ok {
		enabled, valid := inviteBoolFromAny(raw)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be boolean"})
			return
		}
		cfg.Enabled = enabled
	}
	if raw, ok := req["commission_enabled"]; ok {
		enabled, valid := inviteBoolFromAny(raw)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_enabled must be boolean"})
			return
		}
		cfg.CommissionEnabled = enabled
	}
	if raw, ok := req["auto_generate"]; ok {
		autoGenerate, valid := inviteBoolFromAny(raw)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "auto_generate must be boolean"})
			return
		}
		cfg.AutoGenerate = autoGenerate
	}
	if raw, ok := req["code_count"]; ok {
		codeCount, valid := inviteIntFromAny(raw)
		if !valid || codeCount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code_count must be non-negative integer"})
			return
		}
		cfg.CodeCount = codeCount
	}
	if raw, ok := req["code_expire_days"]; ok {
		expireDays, valid := inviteIntFromAny(raw)
		if !valid || expireDays < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code_expire_days must be non-negative integer"})
			return
		}
		cfg.CodeExpireDays = expireDays
	}
	if raw, ok := req["commission_type"]; ok {
		switch v := raw.(type) {
		case string:
			normalized := strings.ToLower(strings.TrimSpace(v))
			if normalized == "" {
				break
			}
			switch normalized {
			case "percent", "percentage", "rate":
				cfg.CommissionType = 1
			case "fixed", "fixed_amount":
				cfg.CommissionType = 2
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "commission_type must be percent/fixed or 1/2"})
				return
			}
		default:
			commissionType, valid := inviteIntFromAny(raw)
			if !valid {
				c.JSON(http.StatusBadRequest, gin.H{"error": "commission_type must be percent/fixed or 1/2"})
				return
			}
			if commissionType == 0 {
				break
			}
			if commissionType != 1 && commissionType != 2 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "commission_type must be percent/fixed or 1/2"})
				return
			}
			cfg.CommissionType = commissionType
		}
	}
	if raw, ok := req["commission_type_code"]; ok {
		commissionType, valid := inviteIntFromAny(raw)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_type_code must be 1 or 2"})
			return
		}
		if commissionType == 0 {
			// treat zero from fully serialized struct payload as "unspecified"
		} else if commissionType != 1 && commissionType != 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_type_code must be 1 or 2"})
			return
		} else {
			cfg.CommissionType = commissionType
		}
	}
	commissionRateRatioProvided := false
	if raw, ok := req["commission_rate_ratio"]; ok {
		rateRatio, valid := inviteFloatFromAny(raw)
		if !valid || rateRatio < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_rate_ratio must be non-negative number"})
			return
		}
		cfg.CommissionRate = rateRatio
		commissionRateRatioProvided = true
	}
	if raw, ok := req["commission_rate"]; ok && !commissionRateRatioProvided {
		rate, valid := inviteFloatFromAny(raw)
		if !valid || rate < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_rate must be non-negative number"})
			return
		}
		if rate > 1 {
			rate = rate / 100
		}
		cfg.CommissionRate = rate
	}
	if raw, ok := req["commission_fixed"]; ok {
		commissionFixed, valid := inviteFloatFromAny(raw)
		if !valid || commissionFixed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_fixed must be non-negative number"})
			return
		}
		cfg.CommissionFixed = commissionFixed
	}
	if raw, ok := req["commission_min"]; ok {
		minWithdraw, valid := inviteFloatFromAny(raw)
		if !valid || minWithdraw < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commission_min must be non-negative number"})
			return
		}
		cfg.CommissionMinAmount = minWithdraw
	}
	if raw, ok := req["min_withdraw"]; ok {
		minWithdraw, valid := inviteFloatFromAny(raw)
		if !valid || minWithdraw < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "min_withdraw must be non-negative number"})
			return
		}
		cfg.CommissionMinAmount = minWithdraw
	}
	if raw, ok := req["first_order_bonus"]; ok {
		firstOrderBonus, valid := inviteFloatFromAny(raw)
		if !valid || firstOrderBonus < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "first_order_bonus must be non-negative number"})
			return
		}
		cfg.FirstOrderBonus = firstOrderBonus
	}
	if raw, ok := req["first_traffic_bonus"]; ok {
		firstTrafficBonus, valid := inviteIntFromAny(raw)
		if !valid || firstTrafficBonus < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "first_traffic_bonus must be non-negative integer"})
			return
		}
		cfg.FirstTrafficBonus = int64(firstTrafficBonus)
	}
	if raw, ok := req["code_prefix"]; ok {
		codePrefix, ok := raw.(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code_prefix must be string"})
			return
		}
		frontendCfg.CodePrefix = strings.TrimSpace(codePrefix)
	}
	if raw, ok := req["code_length"]; ok {
		codeLength, valid := inviteIntFromAny(raw)
		if !valid || codeLength <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code_length must be positive integer"})
			return
		}
		frontendCfg.CodeLength = codeLength
	}
	if raw, ok := req["withdraw_fee"]; ok {
		withdrawFee, valid := inviteFloatFromAny(raw)
		if !valid || withdrawFee < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_fee must be non-negative number"})
			return
		}
		frontendCfg.WithdrawFee = withdrawFee
	}
	if raw, ok := req["withdraw_methods"]; ok {
		methods, valid := inviteStringSliceFromAny(raw)
		if !valid || len(methods) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_methods must be non-empty string array"})
			return
		}
		frontendCfg.WithdrawMethods = methods
	}

	if err := database.Get().Save(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.saveFrontendConfig(frontendCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.inviteService.SetConfig(&cfg)
	c.JSON(http.StatusOK, gin.H{"data": inviteConfigResponse(&cfg, frontendCfg)})
}

// GetWithdrawals godoc
// @Summary 鑾峰彇鎻愮幇鐢宠鍒楄〃
// @Description 绠＄悊鍛樿幏鍙栨彁鐜扮敵璇峰垪琛?
// @Tags 绠＄悊绔?绯荤粺
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "鐘舵€佺瓫閫?
// @Param page query int false "椤电爜" default(1)
// @Param page_size query int false "姣忛〉鏁伴噺" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /admin/invite/withdrawals [get]
func (h *InviteHandler) GetWithdrawals(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var records []model.CommissionWithdraw
	var total int64

	db := database.Get().Model(&model.CommissionWithdraw{})
	if status != "" {
		if numericStatus, err := strconv.Atoi(status); err == nil {
			db = db.Where("status = ?", numericStatus)
		} else if mappedStatus, ok := inviteWithdrawalStatusFromText(status); ok {
			db = db.Where("status = ?", mappedStatus)
		}
	}

	db.Count(&total)
	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records)

	list := make([]gin.H, 0, len(records))
	for _, record := range records {
		list = append(list, inviteWithdrawalResponse(record))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ProcessWithdraw godoc
// @Summary 澶勭悊鎻愮幇鐢宠
// @Description 绠＄悊鍛樺鐞嗘彁鐜扮敵璇凤紝鎵瑰噯鎴栨嫆缁?
// @Tags 绠＄悊绔?绯荤粺
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "鎻愮幇璁板綍ID"
// @Param request body map[string]interface{} true "澶勭悊璇锋眰 {status, remark}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/invite/withdrawals/{id}/process [post]
func (h *InviteHandler) ProcessWithdraw(c *gin.Context) {
	id := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := 0
	if approveRaw, ok := req["approve"]; ok {
		approve, ok := inviteBoolFromAny(approveRaw)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "approve must be boolean"})
			return
		}
		if approve {
			status = 1
		} else {
			status = 2
		}
	} else if approvedRaw, ok := req["approved"]; ok {
		approved, ok := inviteBoolFromAny(approvedRaw)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "approved must be boolean"})
			return
		}
		if approved {
			status = 1
		} else {
			status = 2
		}
	} else if statusRaw, ok := req["status"]; ok {
		switch v := statusRaw.(type) {
		case float64:
			status = int(v)
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				status = parsed
			} else if mapped, ok := inviteWithdrawalStatusFromText(v); ok {
				status = mapped
			}
		}
	}

	if status != 1 && status != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be approved/rejected or 1/2"})
		return
	}

	remark, _ := req["remark"].(string)

	var withdraw model.CommissionWithdraw
	if err := database.Get().First(&withdraw, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "withdrawal not found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "withdrawal not found"})
		return
	}

	if withdraw.Status != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "withdrawal already processed"})
		return
	}

	now := time.Now()
	withdraw.Status = status
	withdraw.Remark = remark
	withdraw.ProcessedAt = &now

	database.Get().Save(&withdraw)

	// If rejected, return the amount to user commission balance.
	if status == 2 {
		database.Get().Model(&model.User{}).Where("id = ?", withdraw.UserID).
			Update("commission_balance", gorm.Expr("commission_balance + ?", withdraw.Amount))
	}

	c.JSON(http.StatusOK, gin.H{"data": inviteWithdrawalResponse(withdraw)})
}

// GetInviteStats godoc
// @Summary 鑾峰彇閭€璇风粺璁?
// @Description 绠＄悊鍛樿幏鍙栭個璇风郴缁熺粺璁℃暟鎹?
// @Tags 绠＄悊绔?绯荤粺
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/invite/stats [get]
func (h *InviteHandler) GetInviteStats(c *gin.Context) {
	var totalUsers int64
	var invitedUsers int64
	var totalCommission float64
	var pendingCommission float64
	var withdrawnCommission float64
	var pendingWithdraw float64

	database.Get().Model(&model.User{}).Count(&totalUsers)
	database.Get().Model(&model.User{}).Where("invite_user_id IS NOT NULL").Count(&invitedUsers)
	database.Get().Model(&model.CommissionRecord{}).Where("status IN ?", []int{1, 2}).
		Select("COALESCE(SUM(amount), 0)").Scan(&totalCommission)
	database.Get().Model(&model.CommissionRecord{}).Where("status = 0").
		Select("COALESCE(SUM(amount), 0)").Scan(&pendingCommission)
	database.Get().Model(&model.CommissionWithdraw{}).Where("status = 1").
		Select("COALESCE(SUM(amount), 0)").Scan(&withdrawnCommission)
	database.Get().Model(&model.CommissionWithdraw{}).Where("status = 0").
		Select("COALESCE(SUM(amount), 0)").Scan(&pendingWithdraw)

	type topInviterRow struct {
		UserID      uint    `json:"user_id"`
		InviteCount int64   `json:"invite_count"`
		Commission  float64 `json:"commission"`
	}

	var topInviters []topInviterRow
	database.Get().Table("v2_user").
		Select("invite_user_id AS user_id, COUNT(*) AS invite_count").
		Where("invite_user_id IS NOT NULL").
		Group("invite_user_id").
		Order("invite_count DESC").
		Limit(10).
		Scan(&topInviters)

	var commissions []struct {
		UserID     uint
		Commission float64
	}
	database.Get().Model(&model.CommissionRecord{}).
		Select("user_id, COALESCE(SUM(amount), 0) AS commission").
		Where("status IN ?", []int{1, 2}).
		Group("user_id").
		Scan(&commissions)

	commissionByUser := make(map[uint]float64, len(commissions))
	for _, row := range commissions {
		commissionByUser[row.UserID] = row.Commission
	}
	for i := range topInviters {
		topInviters[i].Commission = commissionByUser[topInviters[i].UserID]
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"total_invites":        invitedUsers,
			"total_commission":     totalCommission,
			"pending_commission":   pendingCommission,
			"withdrawn_commission": withdrawnCommission,
			"top_inviters":         topInviters,
			"total_users":          totalUsers,
			"invited_users":        invitedUsers,
			"pending_withdraw":     pendingWithdraw,
		},
	})
}
