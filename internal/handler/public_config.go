package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/edition"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// PublicConfig is what the web app reads before anyone signs in: the
// product edition, the packages the edition hides (their menus and pages
// are not shown), and whether registration asks for an invite code.
type PublicConfig struct {
	Edition        string                   `json:"edition"`
	HiddenPackages []string                 `json:"hidden_packages"`
	Registration   PublicRegistrationConfig `json:"registration"`
}

// PublicRegistrationConfig is the public part of the registration policy.
type PublicRegistrationConfig struct {
	Enabled       bool `json:"enabled"`
	RequireInvite bool `json:"require_invite"`
}

// NewPublicConfig returns the public configuration of cfg.
func NewPublicConfig(cfg *config.Config) PublicConfig {
	policy := edition.For(cfg)
	registration := service.ResolveRegistrationPolicy(cfg)
	return PublicConfig{
		Edition:        policy.Name(),
		HiddenPackages: policy.HiddenPackages(),
		Registration:   PublicRegistrationConfig{Enabled: registration.Enabled, RequireInvite: registration.RequireInvite},
	}
}

// PublicConfigHandler serves GET /api/v4/public/config.
//
// @Summary 公开配置
// @Description 产品版本 (community / commercial)、该版本隐藏的包，以及注册是否需要邀请码
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v4/public/config [get]
func PublicConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		kernelData(c, http.StatusOK, NewPublicConfig(cfg))
	}
}
