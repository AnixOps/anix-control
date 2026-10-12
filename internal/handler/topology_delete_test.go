package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type topologyDeleteHandlerFixture struct {
	db       *gorm.DB
	handler  *KernelHandler
	topology model.Topology
	revision model.TopologyRevision
}

// newTopologyDeleteHandlerFixture builds a topology with one plugin vertex on
// an assigned node; active makes its revision the active one.
func newTopologyDeleteHandlerFixture(t *testing.T, active bool) topologyDeleteHandlerFixture {
	t.Helper()
	db := newKernelHandlerTestDB(t, &model.Node{})
	previousConfig := config.Get()
	config.Set(nil)
	t.Cleanup(func() { config.Set(previousConfig) })
	node := model.Node{Name: "topology-delete-node", Host: "127.0.0.1", APIKey: "topology-delete-key"}
	require.NoError(t, db.Create(&node).Error)
	pluginID := "topology-delete-runtime"
	require.NoError(t, db.Create(&model.Plugin{ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true}).Error)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test",
	}).Error)
	topology := model.Topology{Name: "topology-delete", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: pluginID, Role: "entry", DesiredVersion: "1.0.0", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: pluginID, Role: "entry", ConfigJSON: `{}`,
	}).Error)
	if active {
		require.NoError(t, db.Model(&topology).Update("active_revision_id", revision.ID).Error)
		require.NoError(t, db.Model(&revision).Update("state", "active").Error)
		topology.ActiveRevisionID = &revision.ID
	}
	return topologyDeleteHandlerFixture{db: db, handler: &KernelHandler{db: db}, topology: topology, revision: revision}
}

func asKernelAdmin(c *gin.Context) {
	c.Set("user_id", uint(1))
	c.Set("is_admin", true)
}

func enableTopologyExecution(t *testing.T) {
	t.Helper()
	config.Set(&config.Config{Plugins: config.PluginConfig{DispatchEnabled: true, TopologyExecutionEnabled: true}})
}

func (f topologyDeleteHandlerFixture) topologyPath(suffix string) string {
	return "/topologies/" + strconv.FormatUint(uint64(f.topology.ID), 10) + suffix
}

func (f topologyDeleteHandlerFixture) revisionPath(suffix string) string {
	return f.topologyPath("/revisions/" + strconv.FormatUint(uint64(f.revision.ID), 10) + suffix)
}

func (f topologyDeleteHandlerFixture) planBody() string {
	return `{"topology_id":` + strconv.FormatUint(uint64(f.topology.ID), 10) + `,"revision_id":` + strconv.FormatUint(uint64(f.revision.ID), 10) + `}`
}

type topologyDeleteResponse struct {
	Data struct {
		Topology   map[string]any `json:"topology"`
		Deleted    bool           `json:"deleted"`
		Deployment map[string]any `json:"deployment"`
	} `json:"data"`
}

func decodeTopologyDelete(t *testing.T, body []byte) topologyDeleteResponse {
	t.Helper()
	var response topologyDeleteResponse
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func requireKernelErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &response), string(body))
	require.Equal(t, code, response.Error.Code, string(body))
	require.NotEmpty(t, response.Error.Message)
}

func TestKernelDeleteTopologyWithoutActiveRevisionHidesItEverywhere(t *testing.T) {
	f := newTopologyDeleteHandlerFixture(t, false)
	h := f.handler
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/deployments", f.planBody(), "/deployments", h.PlanDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	var planned model.TopologyDeployment
	require.NoError(t, f.db.First(&planned, "topology_id = ?", f.topology.ID).Error)

	recorder = performKernelHandlerRequest(t, http.MethodDelete, f.topologyPath(""), "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeTopologyDelete(t, recorder.Body.Bytes())
	require.True(t, response.Data.Deleted)
	require.Nil(t, response.Data.Deployment)
	require.EqualValues(t, f.topology.ID, response.Data.Topology["id"])
	require.Equal(t, "topology-delete", response.Data.Topology["name"])
	require.NotNil(t, response.Data.Topology["deleted_at"])
	require.Nil(t, response.Data.Topology["delete_requested_at"])
	require.NotContains(t, response.Data.Topology, "deleted", "the flag is stored, not answered")

	recorder = performKernelHandlerRequest(t, http.MethodGet, "/topologies", "", "/topologies", h.ListTopologies)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":[]}`, recorder.Body.String())

	notFound := []struct {
		method, path, route, body string
		handle                    gin.HandlerFunc
	}{
		{http.MethodGet, f.topologyPath("/revisions"), "/topologies/:id/revisions", "", h.ListTopologyRevisions},
		{http.MethodPost, f.topologyPath("/revisions"), "/topologies/:id/revisions", `{"vertices":[{"key":"a","kind":"endpoint","config":"{}"}],"edges":[]}`, h.CreateTopologyRevision},
		{http.MethodGet, f.revisionPath(""), "/topologies/:id/revisions/:revision_id", "", h.GetTopologyRevision},
		{http.MethodPost, f.revisionPath("/preview"), "/topologies/:id/revisions/:revision_id/preview", "", h.PreviewTopologyDeployment},
		{http.MethodPost, f.revisionPath("/diagnose"), "/topologies/:id/revisions/:revision_id/diagnose", "", h.DiagnoseTopologyDeployment},
		{http.MethodPost, "/deployments", "/deployments", f.planBody(), h.PlanDeployment},
		{http.MethodDelete, f.topologyPath(""), "/topologies/:id", "", h.DeleteTopology},
	}
	for _, probe := range notFound {
		recorder = performKernelHandlerRequestWithSetup(t, probe.method, probe.path, probe.body, probe.route, probe.handle, asKernelAdmin)
		require.Equal(t, http.StatusNotFound, recorder.Code, probe.method+" "+probe.path+" "+recorder.Body.String())
		requireKernelErrorCode(t, recorder.Body.Bytes(), "not_found")
	}

	// The planned deployment was cancelled and cannot be applied; it is kept
	// for audit.
	enableTopologyExecution(t)
	deploymentPath := "/deployments/" + strconv.FormatUint(uint64(planned.ID), 10)
	recorder = performKernelHandlerRequest(t, http.MethodPost, deploymentPath+"/apply", "", "/deployments/:id/apply", h.ApplyDeployment)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_deleted")
	recorder = performKernelHandlerRequest(t, http.MethodGet, deploymentPath, "", "/deployments/:id", h.GetDeploymentStatus)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"state":"cancelled"`)
	recorder = performKernelHandlerRequest(t, http.MethodGet, "/deployments", "", "/deployments", h.ListDeployments)
	require.Contains(t, recorder.Body.String(), `"state":"cancelled"`)

	// The name is free again, once.
	create := `{"name":"topology-delete","service_scope":"forward"}`
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/topologies", create, "/topologies", h.CreateTopology)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/topologies", create, "/topologies", h.CreateTopology)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_name_conflict")
}

func TestKernelDeleteTopologyWithActiveRevisionStartsATeardown(t *testing.T) {
	f := newTopologyDeleteHandlerFixture(t, true)
	h := f.handler

	recorder := performKernelHandlerRequest(t, http.MethodDelete, f.topologyPath(""), "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_execution_disabled")
	require.Contains(t, recorder.Body.String(), "plugins.topology_execution_enabled")
	var count int64
	require.NoError(t, f.db.Model(&model.TopologyDeployment{}).Count(&count).Error)
	require.Zero(t, count)

	enableTopologyExecution(t)
	recorder = performKernelHandlerRequest(t, http.MethodDelete, f.topologyPath(""), "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	response := decodeTopologyDelete(t, recorder.Body.Bytes())
	require.False(t, response.Data.Deleted)
	require.NotNil(t, response.Data.Topology["delete_requested_at"])
	require.Nil(t, response.Data.Topology["deleted_at"])
	require.Equal(t, true, response.Data.Deployment["teardown"])
	require.Equal(t, "applying", response.Data.Deployment["state"])
	require.EqualValues(t, f.revision.ID, response.Data.Deployment["revision_id"])
	require.NotContains(t, response.Data.Deployment, "steps")

	// Listed as being deleted until the teardown ends; nothing else may
	// change it meanwhile.
	recorder = performKernelHandlerRequest(t, http.MethodGet, "/topologies", "", "/topologies", h.ListTopologies)
	require.Contains(t, recorder.Body.String(), `"delete_requested_at":"`)
	recorder = performKernelHandlerRequest(t, http.MethodDelete, f.topologyPath(""), "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_deleting")
	recorder = performKernelHandlerRequest(t, http.MethodPost, "/deployments", f.planBody(), "/deployments", h.PlanDeployment)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_deleting")
	recorder = performKernelHandlerRequestWithSetup(t, http.MethodPost, f.topologyPath("/revisions"), `{"vertices":[{"key":"a","kind":"endpoint","config":"{}"}],"edges":[]}`, "/topologies/:id/revisions", h.CreateTopologyRevision, asKernelAdmin)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_deleting")
}

func TestKernelDeleteTopologyRefusesWhileADeploymentRuns(t *testing.T) {
	f := newTopologyDeleteHandlerFixture(t, false)
	h := f.handler
	enableTopologyExecution(t)
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/deployments", f.planBody(), "/deployments", h.PlanDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	var deployment model.TopologyDeployment
	require.NoError(t, f.db.First(&deployment, "topology_id = ?", f.topology.ID).Error)
	path := "/deployments/" + strconv.FormatUint(uint64(deployment.ID), 10)
	recorder = performKernelHandlerRequest(t, http.MethodPost, path+"/apply", "", "/deployments/:id/apply", h.ApplyDeployment)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	recorder = performKernelHandlerRequest(t, http.MethodDelete, f.topologyPath(""), "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	requireKernelErrorCode(t, recorder.Body.Bytes(), "topology_deployment_busy")
	var topology model.Topology
	require.NoError(t, f.db.First(&topology, f.topology.ID).Error, "the topology stays")

	recorder = performKernelHandlerRequest(t, http.MethodDelete, "/topologies/zero", "", "/topologies/:id", h.DeleteTopology)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	requireKernelErrorCode(t, recorder.Body.Bytes(), "invalid_id")
}
