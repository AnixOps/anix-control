package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The node credential split leaves tombstones (!moved:<id>) and the
// placeholder (********) in the legacy columns once a table is finalized.
// The node middleware never accepts one as a node's key, not even through
// the plain-key fallback of rows without a hash.
func TestNodeMiddlewareRefusesTombstonesAndThePlaceholder(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	db := database.GetDB()
	tombstoned := &model.Node{Name: "tombstoned", Host: "127.0.0.1", Port: 443, APIKey: "!moved:1", Status: model.NodeStatusOnline}
	masked := &model.Node{Name: "masked", Host: "127.0.0.2", Port: 443, APIKey: "********", Status: model.NodeStatusOnline}
	usable := &model.Node{Name: "usable", Host: "127.0.0.3", Port: 443, APIKey: "node-key-3", APIKeyHash: keyHash("node-key-3"), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(tombstoned).Error)
	require.NoError(t, db.Create(masked).Error)
	require.NoError(t, db.Create(usable).Error)

	router := gin.New()
	router.GET("/package", NodeAPIKeyHeaderAuth(), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	serve := func(key string) int {
		req := httptest.NewRequest(http.MethodGet, "/package", nil)
		req.Header.Set("X-API-Key", key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, node := range []*model.Node{tombstoned, masked} {
		for _, key := range []string{node.APIKey, "!moved:" + strconv.FormatUint(uint64(node.ID), 10), "********"} {
			require.Equal(t, http.StatusUnauthorized, serve(key), "%s %q", node.Name, key)
		}
	}
	require.Equal(t, http.StatusOK, serve("node-key-3"), "a usable key passes")
}
