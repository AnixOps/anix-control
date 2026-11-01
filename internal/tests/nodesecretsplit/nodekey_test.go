package nodesecretsplit

import (
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/gin-gonic/gin"
)

// nodeKeyAuth reads a node's API key from X-API-Key through the credential
// split, as the node API key readers do: the node it names, or 401.
func nodeKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		node, err := nodesecrets.NodeByAPIKey(database.GetDB(), key, true)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if node.Status == model.NodeStatusDisabled {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Set("node_id", node.ID)
		c.Next()
	}
}
