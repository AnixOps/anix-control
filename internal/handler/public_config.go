package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// PublicConfig is what the web app reads before anyone signs in: whether
// registration asks for an invite code, and the fields of the retired
// product editions. There is one product: every official package, orders,
// payments and affiliate included, is built into Control and installed, so
// Edition is always "commercial" (the value with which the web app shows
// every entry point) and HiddenPackages is always empty, until the web
// app's edition switch is removed.
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

// publicConfigEdition is the edition value the web app shows every entry
// point for.
const publicConfigEdition = "commercial"

// NewPublicConfig returns the public configuration of cfg.
func NewPublicConfig(cfg *config.Config) PublicConfig {
	registration := service.ResolveRegistrationPolicy(cfg)
	return PublicConfig{
		Edition:        publicConfigEdition,
		HiddenPackages: []string{},
		Registration:   PublicRegistrationConfig{Enabled: registration.Enabled, RequireInvite: registration.RequireInvite},
	}
}

// PublicConfigHandler serves GET /api/v4/public/config.
//
// @Summary 公开配置
// @Description 注册是否需要邀请码；edition 固定为 commercial、hidden_packages 固定为空（版本区分已取消，全部官方包内置）
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
