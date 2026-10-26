package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRealityTargetsEnv(t *testing.T) (*diagnosticsEnv, *gin.Engine) {
	t.Helper()
	env := newDiagnosticsEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.NodeProtocol{}, &model.RealityTargetProbe{}, &model.RealityTargetAuto{}))
	require.NoError(t, env.db.Model(&model.Node{}).Where("id = ?", 5).Update("last_public_address", "82.139.194.175").Error)
	handler := &RealityTargetsHandler{db: func() *gorm.DB { return env.db }, probes: func() realityTargetProber {
		return kernelnodeops.RealityProbes{Sources: func() kernelnodeops.AgentSources { return kernelnodeops.AgentSources{Streams: env.streams} }}
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/nodes/:id/reality-targets", handler.List)
	router.POST("/nodes/:id/reality-targets/probe", handler.Probe)
	router.POST("/nodes/:id/reality-targets/apply", handler.Apply)
	router.PUT("/nodes/:id/reality-targets/auto", handler.SetAuto)
	return env, router
}

func serveJSON(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

// A run sends one reality.probe per candidate on the node's stream, ranks
// the answers, stores them and audits the run; nothing else changes.
func TestRealityTargetsProbe(t *testing.T) {
	env, router := newRealityTargetsEnv(t)

	// Offline, then without the capability: refused before anything is sent.
	response := serveJSON(router, http.MethodPost, "/nodes/proxy-5/reality-targets/probe", `{"candidates":["near.example.com"],"include_defaults":false}`)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "node_offline", errorCode(t, response))
	env.streams.connect(proxy(5), diagnosticsCapabilities)
	response = serveJSON(router, http.MethodPost, "/nodes/proxy-5/reality-targets/probe", `{"candidates":["near.example.com"],"include_defaults":false}`)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Equal(t, "reality_probe_unsupported", errorCode(t, response))
	assert.Empty(t, env.streams.operations())

	for _, bad := range []string{`{"candidates":["10.0.0.1"]}`, `{"timeout_ms":50}`, `{"candidates":[],"include_defaults":false}`} {
		response := serveJSON(router, http.MethodPost, "/nodes/proxy-5/reality-targets/probe", bad)
		assert.Equal(t, http.StatusBadRequest, response.Code, bad)
	}
	assert.Equal(t, http.StatusBadRequest, serveJSON(router, http.MethodPost, "/nodes/forward-5/reality-targets/probe", `{}`).Code)

	env.streams.connect(proxy(5), diagnosticsCapabilities, kernelnodeops.RealityProbeCapability)
	env.streams.answer = func(operation *agentv1pb.DesiredOperation) *agentv1pb.ObservedState {
		var payload struct {
			Task struct {
				Action string         `json:"action"`
				Params map[string]any `json:"params"`
			} `json:"task"`
		}
		_ = json.Unmarshal(operation.GetPayloadJson(), &payload)
		name, _ := payload.Task.Params["server_name"].(string)
		if payload.Task.Action != "reality.probe" {
			return nil
		}
		switch name {
		case "near.example.com":
			return succeeded(`{"success":true,"output":"ok","result":{"server_name":"near.example.com","port":443,"address":"82.139.194.9:443","verdict":"ok","handshake_ms":4,"curve":"X25519MLKEM768"}}`)
		default:
			return succeeded(`{"success":false,"error":"failed","result":{"server_name":"` + name + `","port":443,"verdict":"failed","failures":["h2"]}}`)
		}
	}
	response = serveJSON(router, http.MethodPost, "/nodes/proxy-5/reality-targets/probe", `{"candidates":["near.example.com","far.example.com:443"],"include_defaults":false,"timeout_ms":2000}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var answer struct {
		Data struct {
			Report struct {
				NodeAddress string `json:"node_address"`
				Recommended *struct {
					ServerName string `json:"server_name"`
					Score      int    `json:"score"`
				} `json:"recommended"`
				Targets []struct {
					ServerName string `json:"server_name"`
					Verdict    string `json:"verdict"`
				} `json:"targets"`
			} `json:"report"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &answer))
	report := answer.Data.Report
	require.NotNil(t, report.Recommended)
	assert.Equal(t, "near.example.com", report.Recommended.ServerName)
	assert.Equal(t, 70, report.Recommended.Score, "same /24 30 + fast 30 + post-quantum 10")
	require.Len(t, report.Targets, 2)
	assert.Equal(t, "failed", report.Targets[1].Verdict)
	assert.Equal(t, "82.139.194.175", report.NodeAddress)

	operations := env.streams.operations()
	require.Len(t, operations, 2)
	for _, operation := range operations {
		assert.Equal(t, kernelnodeops.AgentDiagnosticOperation, operation.GetKind())
		assert.Contains(t, string(operation.GetPayloadJson()), `"timeout_ms":2000`)
	}
	var audits []model.OperationLog
	require.NoError(t, env.db.Where("module = ?", "reality_target").Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, 1, audits[0].Status)
	assert.Contains(t, audits[0].Content, `"recommended":"near.example.com"`)

	listed := serveJSON(router, http.MethodGet, "/nodes/proxy-5/reality-targets", "")
	require.Equal(t, http.StatusOK, listed.Code)
	assert.Contains(t, listed.Body.String(), `"capable":true`)
	assert.Contains(t, listed.Body.String(), `"near.example.com"`)
	assert.Equal(t, http.StatusNotFound, serveJSON(router, http.MethodGet, "/nodes/proxy-6/reality-targets", "").Code)
}

// Apply writes a probed target into a Reality protocol of the node and
// keeps every other setting, the private key included.
func TestRealityTargetsApply(t *testing.T) {
	env, router := newRealityTargetsEnv(t)
	reality := `{"dest":"www.microsoft.com:443","server_names":["www.microsoft.com"],"private_key":"cPrivateKeyValue0000000000000000000000000000","short_id":"b772bed5"}`
	tlsSettings := `{"server_name":"www.microsoft.com","fingerprint":"chrome"}`
	settings := `{"flow":"xtls-rprx-vision"}`
	protocol := model.NodeProtocol{NodeID: 5, Name: "DE Reality", Type: model.ProtocolType("vless"), Port: 443, TLS: 2, RealitySettings: &reality, TLSSettings: &tlsSettings, Settings: &settings}
	require.NoError(t, env.db.Create(&protocol).Error)
	plain := model.NodeProtocol{NodeID: 5, Name: "SS", Type: model.ProtocolType("shadowsocks"), Port: 8388}
	require.NoError(t, env.db.Create(&plain).Error)
	apply := func(body string) *httptest.ResponseRecorder {
		return serveJSON(router, http.MethodPost, "/nodes/proxy-5/reality-targets/apply", body)
	}

	response := apply(`{"protocol_id":` + itoa(protocol.ID) + `,"server_name":"near.example.com"}`)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Equal(t, "target_not_probed", errorCode(t, response))
	assert.Equal(t, http.StatusNotFound, apply(`{"protocol_id":`+itoa(plain.ID)+`,"server_name":"near.example.com"}`).Code)
	assert.Equal(t, http.StatusNotFound, apply(`{"protocol_id":999,"server_name":"near.example.com"}`).Code)
	assert.Equal(t, http.StatusBadRequest, apply(`{"protocol_id":`+itoa(protocol.ID)+`,"server_name":"1.2.3.4"}`).Code)

	require.NoError(t, env.db.Create(&model.RealityTargetProbe{NodeID: 5, ServerName: "near.example.com", Port: 443, Verdict: "ok", Score: 60, Result: "{}", ProbedAt: time.Now()}).Error)
	response = apply(`{"protocol_id":` + itoa(protocol.ID) + `,"server_name":"near.example.com"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"previous":{"protocol_id"`)

	var stored model.NodeProtocol
	require.NoError(t, env.db.First(&stored, protocol.ID).Error)
	var gotReality, gotTLS map[string]any
	require.NoError(t, json.Unmarshal([]byte(*stored.RealitySettings), &gotReality))
	require.NoError(t, json.Unmarshal([]byte(*stored.TLSSettings), &gotTLS))
	assert.Equal(t, "near.example.com:443", gotReality["dest"])
	assert.Equal(t, "near.example.com", gotReality["server_name"])
	assert.Equal(t, []any{"near.example.com"}, gotReality["server_names"])
	assert.Equal(t, "cPrivateKeyValue0000000000000000000000000000", gotReality["private_key"])
	assert.Equal(t, "b772bed5", gotReality["short_id"])
	assert.Equal(t, "near.example.com", gotTLS["server_name"])
	assert.Equal(t, "chrome", gotTLS["fingerprint"])

	// force applies a target the node has not probed.
	response = apply(`{"protocol_id":` + itoa(protocol.ID) + `,"server_name":"other.example.com","port":8443,"force":true}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, env.db.First(&stored, protocol.ID).Error)
	assert.Contains(t, *stored.RealitySettings, `"dest":"other.example.com:8443"`)

	var audits []model.OperationLog
	require.NoError(t, env.db.Where("action = ?", "reality_target_apply").Order("id").Find(&audits).Error)
	require.Len(t, audits, 5, "a malformed request is not a call")
	assert.Equal(t, 1, audits[4].Status)
}

// Automatic selection is off until turned on; turning it on makes the node
// due at once and is audited; the report answers it.
func TestRealityTargetsAuto(t *testing.T) {
	env, router := newRealityTargetsEnv(t)

	response := serveJSON(router, http.MethodGet, "/nodes/proxy-5/reality-targets", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"auto":{"enabled":false,"fail_streak":0}`)

	for _, bad := range []string{`{}`, `{"enabled":"yes"}`} {
		response = serveJSON(router, http.MethodPut, "/nodes/proxy-5/reality-targets/auto", bad)
		require.Equal(t, http.StatusBadRequest, response.Code, bad)
	}
	response = serveJSON(router, http.MethodPut, "/nodes/proxy-404/reality-targets/auto", `{"enabled":true}`)
	require.Equal(t, http.StatusNotFound, response.Code)

	response = serveJSON(router, http.MethodPut, "/nodes/proxy-5/reality-targets/auto", `{"enabled":true}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var row model.RealityTargetAuto
	require.NoError(t, env.db.First(&row, "node_id = ?", 5).Error)
	assert.True(t, row.Enabled)
	assert.Nil(t, row.NextRunAt)

	response = serveJSON(router, http.MethodGet, "/nodes/proxy-5/reality-targets", "")
	assert.Contains(t, response.Body.String(), `"auto":{"enabled":true`)

	response = serveJSON(router, http.MethodPut, "/nodes/proxy-5/reality-targets/auto", `{"enabled":false}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, env.db.First(&row, "node_id = ?", 5).Error)
	assert.False(t, row.Enabled)

	var audits []model.OperationLog
	require.NoError(t, env.db.Where("action = ?", realityTargetAutoAction).Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, 1, audits[0].Status)
}

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }
