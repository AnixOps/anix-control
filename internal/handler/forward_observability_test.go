package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupObservabilityRouter(t *testing.T) *gin.Engine {
	t.Helper()
	initTestDB()

	r := gin.New()
	h := NewForwardHandler()
	g := r.Group("/api/v2/admin")
	g.GET("/forward/observability/multi-ingress", h.GetObservabilityMultiIngress)
	return r
}

func TestGetObservabilityMultiIngress_UsesPanelEnvelope(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	relay := model.ForwardNode{
		Name:    "multi-ingress-relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.20.20.1",
		Port:    8080,
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	exit := model.ForwardNode{
		Name:    "multi-ingress-exit",
		Type:    model.ForwardNodeTypeExit,
		Host:    "10.20.20.2",
		Port:    8080,
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	require.NoError(t, db.Create(&relay).Error)
	require.NoError(t, db.Create(&exit).Error)

	outNodeID := exit.ID
	tunnel := model.ForwardTunnel{
		Name:      "multi-ingress-tunnel",
		InNodeID:  relay.ID,
		OutNodeID: &outNodeID,
		InIP:      relay.Host,
		OutIP:     exit.Host,
		Status:    model.ForwardTunnelStatusActive,
	}
	require.NoError(t, db.Create(&tunnel).Error)

	forward := model.Forward{
		UserID:     1,
		UserName:   "observability-user",
		Name:       "multi-ingress-forward",
		TunnelID:   tunnel.ID,
		InPort:     19000,
		OutPort:    9000,
		RemoteAddr: "192.0.2.10",
		Status:     model.ForwardStatusActive,
	}
	require.NoError(t, db.Create(&forward).Error)

	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey:       fmt.Sprintf("tunnel-node:%d", relay.ID),
		TargetType:      model.LatencyTargetTypeTunnelNode,
		TargetID:        relay.ID,
		Label:           "multi-ingress-relay",
		Host:            relay.Host,
		Port:            relay.Port,
		BucketAt:        time.Now().Truncate(time.Minute),
		IntervalSeconds: 60,
		SampleCount:     5,
		SuccessCount:    5,
		AvgRTT:          18.5,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v2/admin/forward/observability/multi-ingress?targetId=%d", forward.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodePanelTestResponse(t, w)
	assert.Equal(t, float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	list := data["list"].([]any)
	require.NotEmpty(t, list)
	row := list[0].(map[string]any)
	assert.Equal(t, "multi-ingress-tunnel", row["tunnelName"])
	assert.Equal(t, true, row["online"])
	assert.NotContains(t, w.Body.String(), "\"error\"")
}
