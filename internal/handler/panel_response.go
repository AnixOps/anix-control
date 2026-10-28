package handler

import (
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/gin-gonic/gin"
)

// panelSuccess and panelError write the panel envelope through pkg/v2compat,
// which package-native implementations use as well.
func panelSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, v2compat.PanelSuccess(data, time.Now()))
}

func panelError(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, v2compat.PanelError(msg, time.Now()))
}
