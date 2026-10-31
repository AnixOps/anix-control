package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/settingscatalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SettingsCatalogHandler answers the options of 系统设置.
type SettingsCatalogHandler struct {
	db func() *gorm.DB
}

// NewSettingsCatalogHandler reads the process's database.
func NewSettingsCatalogHandler() *SettingsCatalogHandler {
	return &SettingsCatalogHandler{db: database.Get}
}

// List godoc
// @Summary The options of the system settings
// @Description The settings the console offers as options (internal/settingscatalog), in data.items: key, section (general, data), kind (domains, switch, choice, link), choices, min, max, default, value (the value in force now, for a switch or a choice), link (the console route of a link) and sensitive. Any administrator.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v5/settings/catalog [get]
func (h *SettingsCatalogHandler) List(c *gin.Context) {
	items, err := settingscatalog.Items(h.db())
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, gin.H{"items": items})
}
