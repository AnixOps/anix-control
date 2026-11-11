package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// NodeAdminWritesTestSuite holds the regression tests for node and node
// protocol writes that saved the associations nested in their bodies, or
// columns the routes refuse under another spelling.
type NodeAdminWritesTestSuite struct {
	HandlerTestSuite
	nodeA     *model.Node
	nodeB     *model.Node
	protocolA *model.NodeProtocol
}

func (s *NodeAdminWritesTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.nodeA = &model.Node{Name: "edge-a", Host: "a.example.test"}
	s.nodeB = &model.Node{Name: "edge-b", Host: "b.example.test"}
	s.Require().NoError(s.db.Create(s.nodeA).Error)
	s.Require().NoError(s.db.Create(s.nodeB).Error)
	s.protocolA = &model.NodeProtocol{NodeID: s.nodeA.ID, Name: "vless-a", Type: model.ProtocolVLESS, Port: 443}
	s.Require().NoError(s.db.Create(s.protocolA).Error)
}

func (s *NodeAdminWritesTestSuite) serve(method, pattern, path, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	router := gin.New()
	router.Handle(method, pattern, handler)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func (s *NodeAdminWritesTestSuite) ok(w *httptest.ResponseRecorder) map[string]any {
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	resp := decodePanelTestResponse(s.T(), w)
	s.Require().Equal(float64(0), resp["code"], "%v", resp)
	return resp
}

func (s *NodeAdminWritesTestSuite) count(table string) int64 {
	var n int64
	s.Require().NoError(s.db.Table(table).Count(&n).Error)
	return n
}

func (s *NodeAdminWritesTestSuite) node(id uint) model.Node {
	var node model.Node
	s.Require().NoError(s.db.First(&node, id).Error)
	return node
}

// requireSeededIDs checks that the seeded nodes kept their ids.
func (s *NodeAdminWritesTestSuite) requireSeededIDs() {
	for _, seeded := range []*model.Node{s.nodeA, s.nodeB} {
		s.node(seeded.ID)
	}
}

// A node body's protocols, the nodes and subscription groups nested in them,
// and its id are not saved: a protocol of another node is not moved, no
// nested node and no group is created, and the new node gets only its
// default protocol.
func (s *NodeAdminWritesTestSuite) TestCreateNodeSavesNoNestedAssociations() {
	handler := NewNodeHandler()
	body := fmt.Sprintf(`{"id":77,"name":"edge-c","host":"c.example.test","protocols":[{"id":%d},`+
		`{"name":"injected","type":"vless","port":8443,"node":{"name":"keyless","host":"evil.example.test"},"subscription_groups":[{"name":"injected-group"}]}]}`,
		s.protocolA.ID)
	resp := s.ok(s.serve("POST", "/nodes", "/nodes", body, handler.CreateNode))
	data := resp["data"].(map[string]any)
	created := uint(data["node_id"].(float64))
	s.NotEqual(uint(77), created, "the body chose the node's id")

	s.EqualValues(3, s.count("v2_node"), "a nested node was created")
	s.EqualValues(0, s.count("v2_subscription_group"), "a nested subscription group was created")
	s.EqualValues(0, s.count("v2_subscription_group_node_protocols"), "a nested protocol was linked")
	var protocolA model.NodeProtocol
	s.Require().NoError(s.db.First(&protocolA, s.protocolA.ID).Error)
	s.Equal(s.nodeA.ID, protocolA.NodeID, "another node's protocol was moved")
	var names []string
	s.Require().NoError(s.db.Model(&model.NodeProtocol{}).Where("node_id = ?", created).Pluck("name", &names).Error)
	s.Equal([]string{"Default VMess"}, names)
}

// The update refused id, but GORM also resolves field names (ID) and
// SQLite column names in any case (Id), which renumbered a node.
func (s *NodeAdminWritesTestSuite) TestUpdateNodeKeepsItsID() {
	handler := NewNodeHandler()
	for _, body := range []string{
		`{"name":"renamed","ID":99}`,
		`{"name":"renamed","Id":98}`,
	} {
		s.ok(s.serve("PUT", "/nodes/:id", fmt.Sprintf("/nodes/%d", s.nodeA.ID), body, handler.UpdateNode))
		s.Equal("renamed", s.node(s.nodeA.ID).Name, body)
		s.requireSeededIDs()
		s.EqualValues(2, s.count("v2_node"), body)
	}
}

// The parent check ran only for the key "parent_id" with a number; other
// spellings and a number in a string set a node as its own parent or made
// a cycle.
func (s *NodeAdminWritesTestSuite) TestUpdateNodeChecksTheParentInEverySpelling() {
	handler := NewNodeHandler()
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.nodeB.ID).Update("parent_id", s.nodeA.ID).Error)
	pathA := fmt.Sprintf("/nodes/%d", s.nodeA.ID)
	for _, body := range []string{
		fmt.Sprintf(`{"ParentID":%d}`, s.nodeA.ID),
		fmt.Sprintf(`{"parent_id":"%d"}`, s.nodeA.ID),
		fmt.Sprintf(`{"Parent_Id":%d}`, s.nodeB.ID),
		fmt.Sprintf(`{"PARENTID":"%d"}`, s.nodeB.ID),
	} {
		w := s.serve("PUT", "/nodes/:id", pathA, body, handler.UpdateNode)
		s.Equal(http.StatusInternalServerError, w.Code, "%s: %s", body, w.Body.String())
		s.Nil(s.node(s.nodeA.ID).ParentID, body)
	}

	// A valid parent is still accepted in any spelling GORM writes.
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.nodeB.ID).Update("parent_id", nil).Error)
	s.ok(s.serve("PUT", "/nodes/:id", pathA, fmt.Sprintf(`{"ParentID":%d}`, s.nodeB.ID), handler.UpdateNode))
	parent := s.node(s.nodeA.ID).ParentID
	s.Require().NotNil(parent)
	s.Equal(s.nodeB.ID, *parent)
}

func TestNodeAdminWrites(t *testing.T) {
	suite.Run(t, new(NodeAdminWritesTestSuite))
}
