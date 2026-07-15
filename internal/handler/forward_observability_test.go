package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
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
	g.GET("/forward/observability/targets", h.ListObservabilityTargets)
	g.GET("/forward/observability/trend", h.GetObservabilityTrend)
	g.GET("/forward/observability/topology", h.GetObservabilityTopology)
	g.GET("/forward/observability/multi-ingress", h.GetObservabilityMultiIngress)
	return r
}

func TestGetObservabilityTrend_ReturnsOrderedPoints(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	base := time.Now().Truncate(time.Minute)
	node := model.Node{
		Name:   "proxy-parent",
		Host:   "1.1.1.1",
		Port:   80,
		APIKey: "observability-trend-parent-key",
		Status: model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(&node).Error)
	key := fmt.Sprintf("node:%d:1.1.1.1:80", node.ID)
	// insert out of order to verify ASC ordering; all in the past so the default
	// trend window (to=now, from=now-1h) includes them.
	for _, offset := range []int{2, 0, 1} {
		require.NoError(t, db.Create(&model.ForwardLatencyBucket{
			TargetKey:       key,
			TargetType:      model.LatencyTargetTypeNode,
			TargetID:        node.ID,
			Label:           "proxy-parent",
			Host:            "1.1.1.1",
			Port:            80,
			BucketAt:        base.Add(-time.Duration(offset) * time.Minute),
			IntervalSeconds: 60,
			SampleCount:     5,
			SuccessCount:    5,
			MinRTT:          10,
			AvgRTT:          float64(20 + offset),
			MaxRTT:          30,
			P95RTT:          28,
		}).Error)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/trend?targetKey="+key, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			TargetKey string `json:"targetKey"`
			Label     string `json:"label"`
			Points    []struct {
				BucketAt int64   `json:"bucketAt"`
				Avg      float64 `json:"avg"`
			} `json:"points"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.NotContains(t, w.Body.String(), "\"error\"")
	assert.Equal(t, key, resp.Data.TargetKey)
	assert.Equal(t, "proxy-parent", resp.Data.Label)
	require.Len(t, resp.Data.Points, 3)
	// ascending by bucketAt
	assert.True(t, resp.Data.Points[0].BucketAt <= resp.Data.Points[1].BucketAt)
	assert.True(t, resp.Data.Points[1].BucketAt <= resp.Data.Points[2].BucketAt)
}

func TestGetObservabilityTrend_MissingTargetKeyReturnsError(t *testing.T) {
	r := setupObservabilityRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/trend", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, -1, resp.Code)
	assert.NotContains(t, w.Body.String(), "\"error\"")
}

func TestListObservabilityTargets_ReturnsLatestPerTarget(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	node := model.Node{
		Name:   "node-a",
		Host:   "3.3.3.3",
		Port:   443,
		APIKey: "observability-target-parent-key",
		Status: model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(&node).Error)
	key := fmt.Sprintf("node:%d:3.3.3.3:443", node.ID)
	base := time.Now().Truncate(time.Minute)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: key, TargetType: model.LatencyTargetTypeNode, TargetID: node.ID,
		Label: "node-a", Host: "3.3.3.3", Port: 443,
		BucketAt: base.Add(-2 * time.Minute), IntervalSeconds: 60, SampleCount: 5, SuccessCount: 0, LossPct: 100,
	}).Error)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: key, TargetType: model.LatencyTargetTypeNode, TargetID: node.ID,
		Label: "node-a", Host: "3.3.3.3", Port: 443,
		BucketAt: base, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5, AvgRTT: 12,
	}).Error)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: "node:999:9.9.9.9:443", TargetType: model.LatencyTargetTypeNode, TargetID: 999,
		Label: "stale-node", Host: "9.9.9.9", Port: 443,
		BucketAt: base, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5, AvgRTT: 99,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/targets", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				TargetKey string  `json:"targetKey"`
				Online    bool    `json:"online"`
				AvgRTT    float64 `json:"latestAvgRtt"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.NotContains(t, w.Body.String(), "\"error\"")

	var found bool
	var foundStale bool
	for _, item := range resp.Data.List {
		if item.TargetKey == key {
			found = true
			assert.True(t, item.Online, "latest bucket has successes")
			assert.InDelta(t, 12, item.AvgRTT, 0.001)
		}
		if item.TargetKey == "node:999:9.9.9.9:443" {
			foundStale = true
		}
	}
	assert.True(t, found, "target should appear in catalog")
	assert.False(t, foundStale, "stale proxy node should not appear in catalog")
}

func TestObservabilityLatencyTrend_SkipsChildProxyNodes(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	parent := model.Node{
		Name:   "proxy-parent",
		Host:   "5.5.5.5",
		Port:   443,
		APIKey: "observability-parent-key",
		Status: model.NodeStatusOnline,
	}
	require.NoError(t, db.Create(&parent).Error)
	parentID := parent.ID
	child := model.Node{
		Name:     "proxy-child",
		Host:     "6.6.6.6",
		Port:     443,
		APIKey:   "observability-child-key",
		Status:   model.NodeStatusOnline,
		ParentID: &parentID,
	}
	require.NoError(t, db.Create(&child).Error)

	base := time.Now().Truncate(time.Minute)
	parentKey := fmt.Sprintf("node:%d:5.5.5.5:443", parent.ID)
	childKey := fmt.Sprintf("node:%d:6.6.6.6:443", child.ID)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: parentKey, TargetType: model.LatencyTargetTypeNode, TargetID: parent.ID,
		Label: "proxy-parent", Host: "5.5.5.5", Port: 443,
		BucketAt: base, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5, AvgRTT: 11,
	}).Error)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: childKey, TargetType: model.LatencyTargetTypeNode, TargetID: child.ID,
		Label: "proxy-child", Host: "6.6.6.6", Port: 443,
		BucketAt: base, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5, AvgRTT: 22,
	}).Error)

	targetReq := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/targets", nil)
	targetW := httptest.NewRecorder()
	r.ServeHTTP(targetW, targetReq)

	var targetsResp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				TargetKey string `json:"targetKey"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(targetW.Body.Bytes(), &targetsResp))
	assert.Equal(t, 0, targetsResp.Code)
	assert.NotContains(t, targetW.Body.String(), "\"error\"")

	var foundParent, foundChild bool
	for _, item := range targetsResp.Data.List {
		if item.TargetKey == parentKey {
			foundParent = true
		}
		if item.TargetKey == childKey {
			foundChild = true
		}
	}
	assert.True(t, foundParent, "parent proxy node should remain in latency targets")
	assert.False(t, foundChild, "child proxy node should not appear in latency targets")

	trendReq := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/trend?targetKey="+url.QueryEscape(childKey), nil)
	trendW := httptest.NewRecorder()
	r.ServeHTTP(trendW, trendReq)

	var trendResp struct {
		Code int `json:"code"`
		Data struct {
			TargetKey string `json:"targetKey"`
			Points    []struct {
				Avg float64 `json:"avg"`
			} `json:"points"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(trendW.Body.Bytes(), &trendResp))
	assert.Equal(t, 0, trendResp.Code)
	assert.NotContains(t, trendW.Body.String(), "\"error\"")
	assert.Equal(t, childKey, trendResp.Data.TargetKey)
	assert.Empty(t, trendResp.Data.Points)
}

func TestGetObservabilityTopology_UsesPanelEnvelope(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	relay := model.ForwardNode{
		Name:    "topology-relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.10.10.1",
		Port:    8080,
		Status:  model.ForwardNodeStatusOnline,
		Latency: 15,
		Enabled: true,
	}
	exit := model.ForwardNode{
		Name:    "topology-exit",
		Type:    model.ForwardNodeTypeExit,
		Host:    "10.10.10.2",
		Port:    8080,
		Status:  model.ForwardNodeStatusOnline,
		Latency: 25,
		Enabled: true,
	}
	require.NoError(t, db.Create(&relay).Error)
	require.NoError(t, db.Create(&exit).Error)

	outNodeID := exit.ID
	tunnel := model.ForwardTunnel{
		Name:      "topology-tunnel",
		InNodeID:  relay.ID,
		OutNodeID: &outNodeID,
		InIP:      relay.Host,
		OutIP:     exit.Host,
		Status:    model.ForwardTunnelStatusActive,
	}
	require.NoError(t, db.Create(&tunnel).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/admin/forward/observability/topology", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodePanelTestResponse(t, w)
	assert.Equal(t, float64(0), resp["code"])
	data := resp["data"].(map[string]any)
	assert.NotEmpty(t, data["nodes"])
	assert.NotEmpty(t, data["edges"])
	assert.NotContains(t, w.Body.String(), "\"error\"")
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
