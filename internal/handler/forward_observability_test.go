package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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
	key := "forward:1:1.1.1.1:80"
	// insert out of order to verify ASC ordering; all in the past so the default
	// trend window (to=now, from=now-1h) includes them.
	for _, offset := range []int{2, 0, 1} {
		require.NoError(t, db.Create(&model.ForwardLatencyBucket{
			TargetKey:       key,
			TargetType:      model.LatencyTargetTypeForward,
			TargetID:        1,
			Label:           "fwd-a",
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
	assert.Equal(t, key, resp.Data.TargetKey)
	assert.Equal(t, "fwd-a", resp.Data.Label)
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
}

func TestListObservabilityTargets_ReturnsLatestPerTarget(t *testing.T) {
	r := setupObservabilityRouter(t)
	db := database.Get()

	key := "forward_node:7:3.3.3.3:443"
	base := time.Now().Truncate(time.Minute)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: key, TargetType: model.LatencyTargetTypeForwardNode, TargetID: 7,
		Label: "node-a", Host: "3.3.3.3", Port: 443,
		BucketAt: base.Add(-2 * time.Minute), IntervalSeconds: 60, SampleCount: 5, SuccessCount: 0, LossPct: 100,
	}).Error)
	require.NoError(t, db.Create(&model.ForwardLatencyBucket{
		TargetKey: key, TargetType: model.LatencyTargetTypeForwardNode, TargetID: 7,
		Label: "node-a", Host: "3.3.3.3", Port: 443,
		BucketAt: base, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5, AvgRTT: 12,
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

	var found bool
	for _, item := range resp.Data.List {
		if item.TargetKey == key {
			found = true
			assert.True(t, item.Online, "latest bucket has successes")
			assert.InDelta(t, 12, item.AvgRTT, 0.001)
		}
	}
	assert.True(t, found, "target should appear in catalog")
}
