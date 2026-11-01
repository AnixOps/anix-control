package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agenttransport"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAgentTransportsHandlerListsTheInventory(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{}, &model.AgentTransport{}, &model.AgentCertificate{})
	now := time.Now().UTC()
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "enrolled"}, {ID: 2, Name: "silent"}}).Error)
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: "websocket", FirstSeenAt: now, LastSeenAt: now},
	}).Error)
	live := agenttransport.NewRecorder(nil)
	handler := &AgentTransportsHandler{
		db:   func() *gorm.DB { return db },
		live: func() *agenttransport.Recorder { return live },
	}
	route := "/api/v4/kernel/agents/transports"

	recorder := performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body struct {
		Data agenttransport.Inventory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	summary := body.Data.Summary
	require.Equal(t, [3]int{2, 1, 1}, [3]int{summary.Total, summary.MTLS, summary.Unseen}, "a retired websocket row is not a sighting")
	require.False(t, summary.ReadyForRequired)
	require.Len(t, summary.RequiredReasons, 1)
	require.Contains(t, summary.RequiredReasons[0], "1 enabled node(s) never enrolled")
	require.Len(t, summary.RequiredBlockers, 1)
	require.Equal(t, "proxy-2", summary.RequiredBlockers[0].Node)
	require.Contains(t, recorder.Body.String(), `"ready_for_required":false`)

	handler.db = func() *gorm.DB { return nil }
	recorder = performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	broken := newKernelHandlerTestDB(t)
	handler.db = func() *gorm.DB { return broken }
	recorder = performKernelHandlerRequest(t, http.MethodGet, route, "", route, handler.List)
	require.Equal(t, http.StatusInternalServerError, recorder.Code, "the tables are missing")

	require.NotNil(t, NewAgentTransportsHandler())
}

// TestAgentTransportsHandlerShowsCertificatesAndConnections: the inventory
// answer carries each node's certificate state, certificate times and
// connection type, and node=... narrows it to the named nodes.
func TestAgentTransportsHandlerShowsCertificatesAndConnections(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{}, &model.ForwardNode{}, &model.AgentTransport{}, &model.AgentCertificate{})
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&[]model.Node{{ID: 1, Name: "enrolled"}, {ID: 2, Name: "revoked"}, {ID: 3, Name: "silent"}}).Error)
	revokedAt := now.Add(-time.Hour)
	require.NoError(t, db.Create(&[]model.AgentCertificate{
		{Serial: "live", NodeKind: "proxy", NodeID: 1, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-24 * time.Hour), NotAfter: now.Add(6 * 24 * time.Hour)},
		{Serial: "ended", NodeKind: "proxy", NodeID: 2, Cluster: "prod", EnrollmentID: "e", IssuerKeyID: "k", CreatedAt: now.Add(-24 * time.Hour), NotAfter: now.Add(6 * 24 * time.Hour), RevokedAt: &revokedAt, RevokeReason: "node_disabled"},
	}).Error)
	require.NoError(t, db.Create(&[]model.AgentTransport{
		{NodeKind: "proxy", NodeID: 1, Transport: model.AgentTransportMTLSStream, AgentVersion: "2.0.0", FirstSeenAt: now, LastSeenAt: now},
		{NodeKind: "proxy", NodeID: 2, Transport: "websocket", FirstSeenAt: now, LastSeenAt: now.Add(-time.Minute)},
	}).Error)
	handler := &AgentTransportsHandler{
		db:   func() *gorm.DB { return db },
		live: func() *agenttransport.Recorder { return agenttransport.NewRecorder(nil) },
	}
	route := "/api/v4/kernel/agents/transports"
	type answer struct {
		Data struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"data"`
	}
	nodes := func(query string) map[string]map[string]any {
		t.Helper()
		recorder := performKernelHandlerRequest(t, http.MethodGet, route+query, "", route, handler.List)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var body answer
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		byNode := map[string]map[string]any{}
		for _, node := range body.Data.Nodes {
			byNode[node["node"].(string)] = node
		}
		return byNode
	}

	all := nodes("")
	require.Len(t, all, 3)
	live := all["proxy-1"]
	require.Equal(t, "valid", live["certificate_state"])
	certificate := live["certificate"].(map[string]any)
	require.Equal(t, "live", certificate["serial"])
	require.Equal(t, now.Add(-24*time.Hour).Format(time.RFC3339), certificate["issued_at"])
	require.Equal(t, now.Add(6*24*time.Hour).Format(time.RFC3339), certificate["not_after"])
	// Seven days of lifetime, issued a day ago: renewed after 4 days 16 hours.
	require.Equal(t, now.Add(-24*time.Hour+7*24*time.Hour*2/3).Format(time.RFC3339), certificate["renew_after"])
	require.Nil(t, certificate["revoked_at"])
	require.Equal(t, "mtls_stream", live["connection"].(map[string]any)["type"], "no session provider: the stream sighting decides")

	revoked := all["proxy-2"]
	require.Equal(t, "revoked", revoked["certificate_state"])
	require.Nil(t, revoked["certificate"])
	last := revoked["last_certificate"].(map[string]any)
	require.Equal(t, "ended", last["serial"])
	require.Equal(t, "node_disabled", last["revoke_reason"])
	require.NotNil(t, last["revoked_at"])
	require.Equal(t, "offline", revoked["connection"].(map[string]any)["type"], "a retired channel is no connection")

	silent := all["proxy-3"]
	require.Equal(t, "none", silent["certificate_state"])
	require.Nil(t, silent["last_certificate"])
	require.Equal(t, "offline", silent["connection"].(map[string]any)["type"])
	require.Nil(t, silent["connection"].(map[string]any)["last_seen_at"])

	one := nodes("?node=proxy-2")
	require.Len(t, one, 1)
	require.Contains(t, one, "proxy-2")
	two := nodes("?node=proxy-1,proxy-3&node=proxy-99")
	require.Len(t, two, 2, "a repeated and a comma separated value; an unknown node is not listed")
	require.Contains(t, two, "proxy-1")
	require.Contains(t, two, "proxy-3")
	require.Len(t, nodes("?node="), 3, "an empty value names nobody")

	for _, query := range []string{"?node=node-1", "?node=proxy-0", "?node=proxy-x", "?node=proxy-1,,,bad"} {
		recorder := performKernelHandlerRequest(t, http.MethodGet, route+query, "", route, handler.List)
		require.Equal(t, http.StatusBadRequest, recorder.Code, query)
		require.Contains(t, recorder.Body.String(), "invalid_request")
	}
	many := make([]string, 0, maxNodeFilter+1)
	for i := 1; i <= maxNodeFilter+1; i++ {
		many = append(many, "proxy-"+strconv.Itoa(i))
	}
	recorder := performKernelHandlerRequest(t, http.MethodGet, route+"?node="+strings.Join(many, ","), "", route, handler.List)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "at most 200")
}
