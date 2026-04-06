package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardPanelHandlerTestSuite struct {
	HandlerTestSuite
	handler *ForwardHandler
}

func (s *ForwardPanelHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.handler = NewForwardHandler()
}

func (s *ForwardPanelHandlerTestSuite) TestListPanelTunnels_SucceedsAsAdmin() {
	ctx, w := s.newAdminContext("POST", "/admin/tunnel/list", nil)
	s.handler.ListPanelTunnels(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	assert.IsType(s.T(), []interface{}{}, resp["data"])
}

func (s *ForwardPanelHandlerTestSuite) TestCreatePanelTunnel_WorksWithValidNode() {
	node := model.ForwardNode{
		Name:    "Ingress Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.1",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)

	payload := map[string]interface{}{
		"name":          "Create Tunnel",
		"inNodeId":      node.ID,
		"type":          1,
		"flow":          1,
		"trafficRatio":  1.2,
		"tcpListenAddr": "[::]",
		"udpListenAddr": "[::]",
		"protocol":      "tcp",
	}

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/create", payload)
	s.handler.CreatePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), "Create Tunnel", data["name"])
}

func (s *ForwardPanelHandlerTestSuite) TestUpdatePanelTunnel_AllowsRuntimeFieldsToChange() {
	node := model.ForwardNode{
		Name:    "Init Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.2",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Update Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Flow:          1,
		Protocol:      "tcp",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	payload := map[string]interface{}{
		"id":            tunnel.ID,
		"name":          "Updated Name",
		"flow":          2,
		"trafficRatio":  2.5,
		"protocol":      "ws",
		"tcpListenAddr": "0.0.0.0",
		"udpListenAddr": "[::1]",
		"interfaceName": "eth1",
	}
	ctx, w := s.newAdminContext("POST", "/admin/tunnel/update", payload)
	s.handler.UpdatePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	var updated model.ForwardTunnel
	assert.NoError(s.T(), database.Get().First(&updated, tunnel.ID).Error)
	assert.Equal(s.T(), "Updated Name", updated.Name)
	assert.Equal(s.T(), 2, updated.Flow)
	assert.Equal(s.T(), "ws", updated.Protocol)
}

func (s *ForwardPanelHandlerTestSuite) TestDeletePanelTunnel_RemovesRecord() {
	node := model.ForwardNode{
		Name:    "Delete Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.3",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Delete Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Flow:          1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/delete", map[string]interface{}{"id": tunnel.ID})
	s.handler.DeletePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(s.T(), float64(0), resp["code"])
	var count int64
	assert.NoError(s.T(), database.Get().Model(&model.ForwardTunnel{}).Where("id = ?", tunnel.ID).Count(&count).Error)
	assert.Zero(s.T(), count)
}

func (s *ForwardPanelHandlerTestSuite) TestDiagnosePanelTunnel_ReturnsReport() {
	node := model.ForwardNode{
		Name:    "Diag Relay",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "10.0.0.4",
		Status:  model.ForwardNodeStatusOnline,
		Enabled: true,
		Port:    9000,
	}
	assert.NoError(s.T(), database.Get().Create(&node).Error)
	tunnel := model.ForwardTunnel{
		Name:          "Diag Tunnel",
		InNodeID:      node.ID,
		InIP:          node.Host,
		Type:          1,
		Flow:          1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(s.T(), database.Get().Create(&tunnel).Error)

	ctx, w := s.newAdminContext("POST", "/admin/tunnel/diagnose", map[string]interface{}{"tunnelId": tunnel.ID})
	s.handler.DiagnosePanelTunnel(ctx)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(s.T(), json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), float64(tunnel.ID), data["tunnelId"])
}

func (s *ForwardPanelHandlerTestSuite) newAdminContext(method, path string, payload interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	var body bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&body).Encode(payload)
	}
	req := httptest.NewRequest(method, path, &body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	ctx.Set("user_id", uint(1))
	ctx.Set("is_admin", true)
	return ctx, rec
}

func TestForwardPanelHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(ForwardPanelHandlerTestSuite))
}
