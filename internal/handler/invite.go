package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InviteHandler 邀请处理器
type InviteHandler struct {
	inviteService *service.InviteService
}

// NewInviteHandler 创建处理器
func NewInviteHandler() *InviteHandler {
	return &InviteHandler{
		inviteService: service.NewInviteService(database.Get()),
	}
}

// ========== 用户接口 ==========

// GetInviteInfo godoc
// @Summary 获取邀请信息
// @Description 用户获取自己的邀请码和佣金信息
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /user/invite [get]
func (h *InviteHandler) GetInviteInfo(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 获取用户邀请码
	codes, err := h.inviteService.GetUserInviteCodes(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取统计
	stats, err := h.inviteService.GetInviteStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取用户佣金余额
	var user model.User
	database.Get().Select("commission_balance").First(&user, userID)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"codes":             codes,
			"commission_balance": user.CommissionBalance,
			"stats":             stats,
		},
	})
}

// GenerateCode godoc
// @Summary 生成邀请码
// @Description 用户生成新的邀请码
// @Tags 用户端
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
// @Summary 获取佣金记录
// @Description 用户获取自己的佣金记录列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
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
// @Summary 申请提现
// @Description 用户申请佣金提现
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]interface{} true "提现请求 {amount, method, account, name}"
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
// @Summary 获取提现记录
// @Description 用户获取自己的提现记录列表
// @Tags 用户端
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
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

// ========== 管理员接口 ==========

// GetConfig godoc
// @Summary 获取邀请配置
// @Description 管理员获取邀请系统配置
// @Tags 管理端-系统
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

	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// UpdateConfig godoc
// @Summary 更新邀请配置
// @Description 管理员更新邀请系统配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.InviteConfig true "邀请配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/invite/config [put]
func (h *InviteHandler) UpdateConfig(c *gin.Context) {
	var req model.InviteConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.Get()
	var cfg model.InviteConfig
	if err := db.First(&cfg).Error; err != nil {
		// 创建新配置
		db.Create(&req)
		c.JSON(http.StatusOK, gin.H{"data": req})
		return
	}

	// 更新配置
	db.Model(&cfg).Updates(req)
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// GetWithdrawals godoc
// @Summary 获取提现申请列表
// @Description 管理员获取提现申请列表
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "状态筛选"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /admin/invite/withdrawals [get]
func (h *InviteHandler) GetWithdrawals(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var records []model.CommissionWithdraw
	var total int64

	db := database.Get().Model(&model.CommissionWithdraw{})
	if status != "" {
		db = db.Where("status = ?", status)
	}

	db.Count(&total)
	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&records)

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ProcessWithdraw godoc
// @Summary 处理提现申请
// @Description 管理员处理提现申请，批准或拒绝
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "提现记录ID"
// @Param request body map[string]interface{} true "处理请求 {status, remark}"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/invite/withdrawals/{id}/process [post]
func (h *InviteHandler) ProcessWithdraw(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Status int    `json:"status" binding:"required,oneof=1 2"` // 1=已处理 2=已拒绝
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var withdraw model.CommissionWithdraw
	if err := database.Get().First(&withdraw, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "withdrawal not found"})
		return
	}

	if withdraw.Status != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "withdrawal already processed"})
		return
	}

	now := time.Now()
	withdraw.Status = req.Status
	withdraw.Remark = req.Remark
	withdraw.ProcessedAt = &now

	database.Get().Save(&withdraw)

	// 如果拒绝，退还余额
	if req.Status == 2 {
		database.Get().Model(&model.User{}).Where("id = ?", withdraw.UserID).
			Update("commission_balance", gorm.Expr("commission_balance + ?", withdraw.Amount))
	}

	c.JSON(http.StatusOK, gin.H{"data": withdraw})
}

// GetInviteStats godoc
// @Summary 获取邀请统计
// @Description 管理员获取邀请系统统计数据
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /admin/invite/stats [get]
func (h *InviteHandler) GetInviteStats(c *gin.Context) {
	var totalUsers int64
	var invitedUsers int64
	var totalCommission float64
	var pendingWithdraw float64

	database.Get().Model(&model.User{}).Count(&totalUsers)
	database.Get().Model(&model.User{}).Where("invite_user_id IS NOT NULL").Count(&invitedUsers)
	database.Get().Model(&model.CommissionRecord{}).Where("status = 1").
		Select("COALESCE(SUM(amount), 0)").Scan(&totalCommission)
	database.Get().Model(&model.CommissionWithdraw{}).Where("status = 0").
		Select("COALESCE(SUM(amount), 0)").Scan(&pendingWithdraw)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"total_users":       totalUsers,
			"invited_users":     invitedUsers,
			"total_commission":  totalCommission,
			"pending_withdraw":  pendingWithdraw,
		},
	})
}