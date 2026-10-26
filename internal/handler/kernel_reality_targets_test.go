package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	require.NoError(t, env.db.AutoMigrate(&model.NodeProtocol{}, &model.RealityTargetProbe{}))
	require.NoError(t, env.db.Model(&model.Node{}).Where("id = ?", 5).Update("last_public_address", "82.139.194.175").Error)
	handler := &RealityTargetsHandler{db: func() *gorm.DB { return env.db }, probes: func() realityTargetProber {
		return kernelnodeops.RealityProbes{Sources: func() kernelnodeops.AgentSources { return kernelnodeops.AgentSources{Streams: env.streams} }}
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)); c.Set("email", "root@example.com") })
	router.GET("/nodes/:id/reality-targets", handler.List)
	router.POST("/nodes/:id/reality-targets/probe", handler.Probe)
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
