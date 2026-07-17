package handler

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestKernelTopologyPreviewHandlerIsReadOnlyAndReturnsStructuredIssues(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	node := model.Node{Name: "preview-handler-node", Host: "127.0.0.1", APIKey: "preview-handler-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "preview-handler-topology", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("c", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID,
		PluginID: "missing-handler-plugin", Role: "entry", ConfigJSON: `{"mtu":500}`,
	}).Error)

	path := "/topologies/" + strconv.FormatUint(uint64(topology.ID), 10) + "/revisions/" + strconv.FormatUint(uint64(revision.ID), 10) + "/preview"
	handler := (&KernelHandler{db: db}).PreviewTopologyDeployment
	recorder := performKernelHandlerRequest(t, http.MethodPost, path, `{"rollout_group":"canary"}`, "/topologies/:id/revisions/:revision_id/preview", handler)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"valid":false`)
	require.Contains(t, recorder.Body.String(), `"assignment_missing"`)
	require.Contains(t, recorder.Body.String(), `"invalid_mtu"`)
	require.Contains(t, recorder.Body.String(), `"steps"`)

	var deploymentCount int64
	require.NoError(t, db.Model(&model.TopologyDeployment{}).Count(&deploymentCount).Error)
	require.Zero(t, deploymentCount)
}

func TestKernelTopologyPreviewBodyEndpointRequiresRevision(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	recorder := performKernelHandlerRequest(t, http.MethodPost, "/deployments/preview", `{"topology_id":1}`, "/deployments/preview", (&KernelHandler{db: db}).PreviewDeployment)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "topology_id and revision_id are required")
}
