package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNodeTrafficHandlerAnswersSeries(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.TrafficLog{}, &model.StatServer{})
	require.NoError(t, db.Create(&model.Node{ID: 7, Name: "seven", APIKey: "k7"}).Error)
	current := time.Now().UTC().Truncate(time.Hour).Unix()
	today := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	require.NoError(t, db.Create(&[]model.TrafficLog{
		{UserID: 1, ServerID: 7, ServerType: "node", U: 100, D: 1000, Rate: 1, LogAt: current + 5},
		{UserID: 2, ServerID: 7, ServerType: "node", U: 10, D: 20, Rate: 2, LogAt: current - 3600 + 5},
		{UserID: 2, ServerID: 8, ServerType: "node", U: 5, D: 5, Rate: 1, LogAt: current + 5},
	}).Error)
	require.NoError(t, db.Create(&model.StatServer{ServerID: 7, ServerType: "node", U: 30, D: 40, RecordType: "d", RecordAt: today.Unix()}).Error)
	handler := &NodeTrafficHandler{db: func() *gorm.DB { return db }}
	route := "/api/v4/kernel/nodes/:id/traffic"
	type answer struct {
		Data struct {
			NodeID      uint   `json:"node_id"`
			Granularity string `json:"granularity"`
			Since       int64  `json:"since_unix_ms"`
			Until       int64  `json:"until_unix_ms"`
			Points      []struct {
				Start int64 `json:"start_unix_ms"`
				Up    int64 `json:"up_bytes"`
				Down  int64 `json:"down_bytes"`
			} `json:"points"`
			Total struct {
				Up   int64 `json:"up_bytes"`
				Down int64 `json:"down_bytes"`
			} `json:"total"`
		} `json:"data"`
	}
	get := func(path string) (*answer, int, string) {
		recorder := performKernelHandlerRequest(t, http.MethodGet, path, "", route, handler.Get)
		var body answer
		if recorder.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		}
		return &body, recorder.Code, recorder.Body.String()
	}

	body, code, raw := get("/api/v4/kernel/nodes/7/traffic")
	require.Equal(t, http.StatusOK, code, raw)
	require.Equal(t, uint(7), body.Data.NodeID)
	require.Equal(t, "hour", body.Data.Granularity, "the default")
	require.Len(t, body.Data.Points, 24)
	require.Equal(t, (current+3600)*1000, body.Data.Until)
	require.Equal(t, (current-23*3600)*1000, body.Data.Since)
	require.Equal(t, current*1000, body.Data.Points[23].Start)
	require.Equal(t, int64(100), body.Data.Points[23].Up)
	require.Equal(t, int64(1000), body.Data.Points[23].Down)
	require.Equal(t, int64(20), body.Data.Points[22].Up, "the rate applies")
	require.Equal(t, int64(40), body.Data.Points[22].Down)
	require.Equal(t, int64(120), body.Data.Total.Up)
	require.Equal(t, int64(1040), body.Data.Total.Down)
	require.Contains(t, raw, `"points":[{"start_unix_ms"`)

	since := strconv.FormatInt((current-2*3600)*1000, 10)
	until := strconv.FormatInt((current+1)*1000, 10)
	body, code, raw = get("/api/v4/kernel/nodes/7/traffic?granularity=hour&since=" + since + "&until=" + until)
	require.Equal(t, http.StatusOK, code, raw)
	require.Len(t, body.Data.Points, 3, "until is rounded up to the end of its hour")

	body, code, raw = get("/api/v4/kernel/nodes/7/traffic?granularity=day")
	require.Equal(t, http.StatusOK, code, raw)
	require.Equal(t, "day", body.Data.Granularity)
	require.Len(t, body.Data.Points, 30)
	require.Equal(t, int64(30), body.Data.Points[29].Up)
	require.Equal(t, int64(40), body.Data.Points[29].Down)
	require.Equal(t, today.UnixMilli(), body.Data.Points[29].Start)
	require.Equal(t, int64(30), body.Data.Total.Up)

	for path, want := range map[string]string{
		"/api/v4/kernel/nodes/7/traffic?granularity=month":                  "granularity",
		"/api/v4/kernel/nodes/7/traffic?since=abc":                          "since must be a positive Unix time",
		"/api/v4/kernel/nodes/7/traffic?until=-5":                           "until must be a positive Unix time",
		"/api/v4/kernel/nodes/7/traffic?since=0":                            "since must be a positive Unix time",
		"/api/v4/kernel/nodes/7/traffic?since=" + until + "&until=" + since: "since must be before until",
		"/api/v4/kernel/nodes/7/traffic?since=1000":                         "at most 720 hour buckets",
		"/api/v4/kernel/nodes/7/traffic?granularity=day&since=1000":         "at most 366 day buckets",
	} {
		_, code, raw = get(path)
		require.Equal(t, http.StatusBadRequest, code, path)
		require.Contains(t, raw, `"invalid_request"`, path)
		require.Contains(t, raw, want, path)
	}

	_, code, raw = get("/api/v4/kernel/nodes/99/traffic")
	require.Equal(t, http.StatusNotFound, code)
	require.Contains(t, raw, `"not_found"`)
	for _, id := range []string{"x", "0", "-1", "4294967296"} {
		_, code, raw = get("/api/v4/kernel/nodes/" + id + "/traffic")
		require.Equal(t, http.StatusBadRequest, code, id)
		require.Contains(t, raw, `"invalid_id"`)
	}

	handler.db = func() *gorm.DB { return nil }
	_, code, _ = get("/api/v4/kernel/nodes/7/traffic")
	require.Equal(t, http.StatusServiceUnavailable, code)
	broken := newKernelHandlerTestDB(t)
	handler.db = func() *gorm.DB { return broken }
	_, code, raw = get("/api/v4/kernel/nodes/7/traffic")
	require.Equal(t, http.StatusInternalServerError, code, "the tables are missing")
	require.Contains(t, raw, `"database_error"`)

	require.NotNil(t, NewNodeTrafficHandler())
}
