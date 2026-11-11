package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// TelegramHandler Telegram Bot处理器
type TelegramHandler struct {
	botService *service.TelegramBotService
}

// NewTelegramHandler 创建处理器
func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		botService: service.NewTelegramBotService(database.Get()),
	}
}

func parseTelegramBotAdminIDs(raw string) []int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []int64{}
	}

	var ids []int64
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		return ids
	}

	for _, part := range strings.Split(trimmed, ",") {
		parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			ids = append(ids, parsed)
		}
	}
	return ids
}

func parseTelegramID(raw any) (int64, error) {
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid telegram_id")
	}
}

// TelegramWebhook godoc
// @Summary Telegram Webhook回调
// @Description 接收Telegram Bot的Webhook回调
// @Tags Telegram
// @Accept json
// @Produce json
// @Param request body service.TelegramUpdate true "Telegram Update"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /telegram/webhook [post]
func (h *TelegramHandler) TelegramWebhook(c *gin.Context) {
	// Only Telegram knows the secret_token registered with setWebhook. Reject
	// anything else before reading the body.
	if !h.botService.ValidWebhookSecret(c.GetHeader("X-Telegram-Bot-Api-Secret-Token")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid webhook secret"})
		return
	}
	var update service.TelegramUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 异步处理更新
	go func() {
		if err := h.botService.HandleUpdate(&update); err != nil {
			log.Printf("telegram webhook update handling failed: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
