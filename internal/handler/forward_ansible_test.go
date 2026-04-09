package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardAnsibleHandlerTestSuite struct {
	HandlerTestSuite
	ansibleNode *model.ForwardNode
	nodeXRelay  *model.ForwardNode
}

func (s *ForwardAnsibleHandlerTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()

	s.ansibleNode = &model.ForwardNode{
		Name:    "Ansible Exec 01",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "192.0.2.10",
		Port:    22,
		Weight:  1,
		Status:  model.ForwardNodeStatusOffline,
		Enabled: true,
	}
	s.db.Create(s.ansibleNode)

	s.nodeXRelay = &model.ForwardNode{
		Name:     "NodeX Relay 01",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "198.51.100.10",
		Port:     8443,
		APIPort:  18080,
		APIToken: "relay-token",
		Weight:   1,
		Status:   model.ForwardNodeStatusOnline,
		Enabled:  true,
	}
	s.db.Create(s.nodeXRelay)

	s.router = gin.New()
}

func (s *ForwardAnsibleHandlerTestSuite) TestListAnsibleMachines() {
	handler := NewForwardHandler()
	s.router.GET("/forward/ansible-machines", handler.ListAnsibleMachines)

	req, _ := http.NewRequest("GET", "/forward/ansible-machines", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Contains(s.T(), w.Body.String(), "Ansible Exec 01")
	assert.NotContains(s.T(), w.Body.String(), "NodeX Relay 01")

	var refreshed model.ForwardNode
	assert.NoError(s.T(), s.db.First(&refreshed, s.ansibleNode.ID).Error)
	assert.Contains(s.T(), refreshed.Tags, service.ForwardNodeInventoryTagAnsibleMachine)
}

func (s *ForwardAnsibleHandlerTestSuite) TestCreateAnsibleMachine() {
	handler := NewForwardHandler()
	s.router.POST("/forward/ansible-machines", handler.CreateAnsibleMachine)

	body := map[string]interface{}{
		"name":   "Ansible Exec 02",
		"host":   "203.0.113.20",
		"port":   22,
		"region": "JP",
		"isp":    "NTT",
		"weight": 3,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/forward/ansible-machines", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var created model.ForwardNode
	assert.NoError(s.T(), s.db.Where("name = ?", "Ansible Exec 02").First(&created).Error)
	assert.Equal(s.T(), model.ForwardNodeTypeRelay, created.Type)
	assert.Equal(s.T(), 0, created.APIPort)
	assert.Equal(s.T(), "", created.APIToken)
	assert.Contains(s.T(), created.Tags, service.ForwardNodeInventoryTagAnsibleMachine)
}

func (s *ForwardAnsibleHandlerTestSuite) TestListNodes_NodeXScopeExcludesAnsibleMachines() {
	handler := NewForwardHandler()
	s.router.GET("/forward/nodes", handler.ListNodes)

	req, _ := http.NewRequest("GET", "/forward/nodes?scope=nodex", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.NotContains(s.T(), w.Body.String(), "Ansible Exec 01")
	assert.Contains(s.T(), w.Body.String(), "NodeX Relay 01")
}

func (s *ForwardAnsibleHandlerTestSuite) TestGetAnsibleMachine_RejectsNodeXRelay() {
	handler := NewForwardHandler()
	s.router.GET("/forward/ansible-machines/:id", handler.GetAnsibleMachine)

	req, _ := http.NewRequest("GET", "/forward/ansible-machines/"+strconv.FormatUint(uint64(s.nodeXRelay.ID), 10), nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *ForwardAnsibleHandlerTestSuite) TestSyncAnsibleMachineStats() {
	handler := NewForwardHandler()
	s.router.POST("/forward/ansible-machines/:id/sync-stats", handler.SyncAnsibleMachineStats)

	req, _ := http.NewRequest("POST", "/forward/ansible-machines/"+strconv.FormatUint(uint64(s.ansibleNode.ID), 10)+"/sync-stats", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Contains(s.T(), w.Body.String(), "panel-side counters")
}

func TestForwardAnsibleHandler(t *testing.T) {
	suite.Run(t, new(ForwardAnsibleHandlerTestSuite))
}
