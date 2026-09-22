package handler

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxPluginSecretRequestBody = 6 << 20

type pluginSecretFileRequest struct {
	Name          string `json:"name" binding:"required"`
	ContentBase64 string `json:"content_base64" binding:"required"`
}

type pluginSecretCreateRequest struct {
	ID          string                    `json:"id" binding:"required"`
	Name        string                    `json:"name" binding:"required"`
	Description string                    `json:"description"`
	Files       []pluginSecretFileRequest `json:"files" binding:"required"`
}

type pluginSecretVersionRequest struct {
	Files []pluginSecretFileRequest `json:"files" binding:"required"`
}

func (h *KernelHandler) pluginSecretStore() (*service.PluginSecretStore, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, service.ErrPluginSecretKeyringUnavailable
	}
	return service.NewPluginSecretStore(h.db, cfg.Plugins.SecretEncryption)
}

func (h *KernelHandler) ListPluginSecrets(c *gin.Context) {
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	rows, err := store.List()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func (h *KernelHandler) GetPluginSecret(c *gin.Context) {
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	row, err := store.Get(c.Param("secret_id"))
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}

func (h *KernelHandler) CreatePluginSecret(c *gin.Context) {
	var req pluginSecretCreateRequest
	if !bindPluginSecretRequest(c, &req) {
		return
	}
	files, err := decodePluginSecretFiles(req.Files)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_secret_material", err.Error())
		return
	}
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	result, err := store.Create(service.PluginSecretCreateInput{
		ID: req.ID, Name: req.Name, Description: req.Description,
		Files: files, ActorID: kernelActorID(c),
	})
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, result)
}

func (h *KernelHandler) CreatePluginSecretVersion(c *gin.Context) {
	var req pluginSecretVersionRequest
	if !bindPluginSecretRequest(c, &req) {
		return
	}
	files, err := decodePluginSecretFiles(req.Files)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_secret_material", err.Error())
		return
	}
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	result, err := store.AddVersion(c.Param("secret_id"), files, kernelActorID(c))
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, result)
}

func (h *KernelHandler) DeletePluginSecretVersion(c *gin.Context) {
	version, err := strconv.ParseUint(strings.TrimSpace(c.Param("version")), 10, 64)
	if err != nil || version == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_secret_version", "version must be a positive integer")
		return
	}
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	if err := store.DeleteVersion(c.Param("secret_id"), version, kernelActorID(c)); err != nil {
		pluginSecretError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *KernelHandler) DeletePluginSecret(c *gin.Context) {
	store, err := h.pluginSecretStore()
	if err != nil {
		pluginSecretError(c, err)
		return
	}
	if err := store.Delete(c.Param("secret_id"), kernelActorID(c)); err != nil {
		pluginSecretError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *KernelHandler) ListPluginSecretAudit(c *gin.Context) {
	secretID := strings.TrimSpace(c.Param("secret_id"))
	if secretID == "" {
		kernelError(c, http.StatusBadRequest, "invalid_secret_id", "secret_id is required")
		return
	}
	var rows []model.PluginSecretAudit
	if err := h.db.Where("secret_id = ?", secretID).Order("id DESC").Limit(500).Find(&rows).Error; err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

func bindPluginSecretRequest(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPluginSecretRequestBody)
	if err := c.ShouldBindJSON(target); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		kernelError(c, status, "invalid_request", "plugin secret request is invalid")
		return false
	}
	return true
}

func decodePluginSecretFiles(input []pluginSecretFileRequest) ([]service.PluginSecretFileInput, error) {
	files := make([]service.PluginSecretFileInput, 0, len(input))
	for _, file := range input {
		content, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.ContentBase64))
		if err != nil {
			return nil, errors.New("content_base64 must use canonical base64 encoding")
		}
		if base64.StdEncoding.EncodeToString(content) != strings.TrimSpace(file.ContentBase64) {
			return nil, errors.New("content_base64 must use canonical base64 encoding")
		}
		files = append(files, service.PluginSecretFileInput{Name: file.Name, Content: content})
	}
	return files, nil
}

func pluginSecretError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPluginSecretKeyringUnavailable):
		kernelError(c, http.StatusServiceUnavailable, "plugin_secret_keyring_unavailable", err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		kernelError(c, http.StatusNotFound, "plugin_secret_not_found", "plugin secret or version was not found")
	case errors.Is(err, service.ErrPluginSecretConflict):
		kernelError(c, http.StatusConflict, "plugin_secret_conflict", err.Error())
	case errors.Is(err, service.ErrPluginSecretReferenced):
		kernelError(c, http.StatusConflict, "plugin_secret_referenced", err.Error())
	case errors.Is(err, service.ErrPluginSecretVersionActive):
		kernelError(c, http.StatusConflict, "plugin_secret_version_active", err.Error())
	case errors.Is(err, service.ErrPluginSecretDeleted):
		kernelError(c, http.StatusGone, "plugin_secret_deleted", err.Error())
	case errors.Is(err, service.ErrPluginSecretIntegrity):
		kernelError(c, http.StatusConflict, "plugin_secret_integrity_failed", err.Error())
	default:
		kernelError(c, http.StatusUnprocessableEntity, "plugin_secret_rejected", err.Error())
	}
}
