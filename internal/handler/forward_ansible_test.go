package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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

func TestForwardAnsibleHandler(t *testing.T) {
	suite.Run(t, new(ForwardAnsibleHandlerTestSuite))
}

func decodePanelTestResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "code")
	require.Contains(t, resp, "msg")
	require.Contains(t, resp, "ts")
	return resp
}
