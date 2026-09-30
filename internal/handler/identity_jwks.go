package handler

import (
	"net/http"

	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/gin-gonic/gin"
)

// IdentityJWKS publishes the identity module's token keys that Control
// accepts, so other services can verify identity tokens issued for Control.
func IdentityJWKS(c *gin.Context) {
	set := identitytoken.KeySet{}
	if keys := authn.DefaultIdentityKeys(); keys != nil {
		set = keys.KeySet()
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, set.Published())
}
