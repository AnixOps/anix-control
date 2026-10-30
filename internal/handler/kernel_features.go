package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/gin-gonic/gin"
)

// ListFeatures godoc
// @Summary List the gated features
// @Description The running build's channel (dev for a dev build built from main, stable for a release) and the features gated on it (internal/features): a feature in the dev stage runs in dev builds only. The admin UI hides what is off.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any "data.channel, data.version and data.features [{name, description, stage, enabled}]"
// @Router /api/v4/kernel/features [get]
func ListFeatures(c *gin.Context) {
	kernelData(c, http.StatusOK, gin.H{
		"channel": features.Channel(), "version": ReleaseVersion, "features": features.List(),
	})
}
