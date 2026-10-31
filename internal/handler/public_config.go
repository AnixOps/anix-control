package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// PublicConfig is what the web app reads before anyone signs in: whether
// registration asks for an invite code, and the product edition. This binary
// is the community edition: payment, order and affiliate (the commercial set,
// config/commercial-boundary.json) are not offered, so Edition is
// "community" and HiddenPackages names those packages. The web app then shows
// no commercial menu, page or extension entry. The commercial build serves
// "commercial" and an empty list.
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

// publicConfigEdition is the edition this binary reports.
const publicConfigEdition = "community"

// publicConfigHiddenPackages are the packages of the commercial set; the web
// app hides their entry points in the community edition.
var publicConfigHiddenPackages = []string{"affiliate", "order", "payment"}

// NewPublicConfig returns the public configuration of cfg.
func NewPublicConfig(cfg *config.Config) PublicConfig {
	registration := service.ResolveRegistrationPolicy(cfg)
	return PublicConfig{
		Edition:        publicConfigEdition,
		HiddenPackages: append([]string(nil), publicConfigHiddenPackages...),
		Registration:   PublicRegistrationConfig{Enabled: registration.Enabled, RequireInvite: registration.RequireInvite},
	}
}

// PublicConfigHandler serves GET /api/v4/public/config.
//
// @Summary 公开配置
// @Description 注册是否需要邀请码；edition 为 community，hidden_packages 为商业集合的包（affiliate、order、payment），网页据此隐藏商业入口
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
