package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The node credential split leaves tombstones (!moved:<id>) and the
// placeholder (********) in the legacy columns once a table is finalized.
// The node middleware never accepts one as a node's key, not even through
// the plain-key fallback of rows without a hash, and a placeholder secret
// signs nothing.
func TestNodeMiddlewareRefusesTombstonesAndThePlaceholder(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	db := database.GetDB()
	tombstoned := &model.Node{Name: "tombstoned", Host: "127.0.0.1", Port: 443, APIKey: "!moved:1", Status: model.NodeStatusOnline}
	masked := &model.Node{Name: "masked", Host: "127.0.0.2", Port: 443, APIKey: "********", Status: model.NodeStatusOnline}
	signed := &model.Node{Name: "signed", Host: "127.0.0.3", Port: 443, APIKey: "node-key-3", APIKeyHash: sha256Hash("node-key-3"),
		Secret: "********", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(tombstoned).Error)
	require.NoError(t, db.Create(masked).Error)
	require.NoError(t, db.Create(signed).Error)

	router := gin.New()
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	router.GET("/uniproxy", NodeAuth(), ok)
	router.GET("/node", NodeAPIKeyAuth(), ok)
	router.GET("/package", NodeAPIKeyHeaderAuth(), ok)
	router.POST("/signed", NodeAPIKeyAuth(), SignatureAuth(), ok)
	serve := func(method, path string, nodeID uint, key string, header map[string]string, body string) int {
		req := httptest.NewRequest(method, path+"?node_id="+strconv.FormatUint(uint64(nodeID), 10), strings.NewReader(body))
		req.Header.Set("X-API-Key", key)
		for name, value := range header {
			req.Header.Set(name, value)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, node := range []*model.Node{tombstoned, masked} {
		for _, key := range []string{node.APIKey, "!moved:" + strconv.FormatUint(uint64(node.ID), 10), "********"} {
			for _, path := range []string{"/uniproxy", "/node", "/package"} {
				require.Equal(t, http.StatusUnauthorized, serve(http.MethodGet, path, node.ID, key, nil, ""), "%s %s %q", node.Name, path, key)
			}
		}
	}

	// A usable key passes; a signature made with the placeholder does not.
	require.Equal(t, http.StatusOK, serve(http.MethodGet, "/node", signed.ID, "node-key-3", nil, ""))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("********"))
	mac.Write([]byte(timestamp + http.MethodPost + "/signed" + "{}"))
	header := map[string]string{SignatureHeader: hex.EncodeToString(mac.Sum(nil)), TimestampHeader: timestamp}
	require.Equal(t, http.StatusUnauthorized, serve(http.MethodPost, "/signed", signed.ID, "node-key-3", header, "{}"))
}
