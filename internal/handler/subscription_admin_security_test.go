package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// SubscriptionAdminWritesTestSuite holds the regression tests for group and
// template writes that saved the associations nested in their bodies.
type SubscriptionAdminWritesTestSuite struct {
	HandlerTestSuite
	node     *model.Node
	protocol *model.NodeProtocol
	groupA   *model.SubscriptionGroup
	groupB   *model.SubscriptionGroup
	template *model.SubscriptionTemplate
}

var subscriptionWritesSeeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func (s *SubscriptionAdminWritesTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.node = &model.Node{Name: "edge", Host: "edge.example.test", APIKey: "node-key", APIKeyHash: "node-key-hash"}
	s.Require().NoError(s.db.Create(s.node).Error)
	s.protocol = &model.NodeProtocol{NodeID: s.node.ID, Name: "vless", Type: model.ProtocolVLESS, Port: 443}
	s.Require().NoError(s.db.Create(s.protocol).Error)
	s.groupA = &model.SubscriptionGroup{Name: "group-a", Enable: 1}
	s.groupB = &model.SubscriptionGroup{Name: "group-b", Enable: 1}
	s.Require().NoError(s.db.Create(s.groupA).Error)
	s.Require().NoError(s.db.Create(s.groupB).Error)
	s.template = &model.SubscriptionTemplate{
		GroupID: s.groupA.ID, Name: "tpl", Type: "vless", Server: "a.example.test", Port: 443, Enable: 1,
		CreatedAt: subscriptionWritesSeeded, UpdatedAt: subscriptionWritesSeeded,
	}
	s.Require().NoError(s.db.Create(s.template).Error)
}

func (s *SubscriptionAdminWritesTestSuite) serve(method, pattern, path, body string, handler gin.HandlerFunc) map[string]any {
	router := gin.New()
	router.Handle(method, pattern, handler)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	resp := decodePanelTestResponse(s.T(), w)
	s.Require().Equal(float64(0), resp["code"], "%v", resp)
	return resp
}

func (s *SubscriptionAdminWritesTestSuite) count(table string) int64 {
	var n int64
	s.Require().NoError(s.db.Table(table).Count(&n).Error)
	return n
}

// requireNoNestedWrites checks that no node, node protocol, group link or
// template was written besides the seeded ones.
func (s *SubscriptionAdminWritesTestSuite) requireNoNestedWrites() {
	s.EqualValues(1, s.count("v2_node"), "a nested node was created")
	s.EqualValues(1, s.count("v2_node_protocol"), "a nested node protocol was created")
	s.EqualValues(0, s.count("v2_subscription_group_node_protocols"), "a nested protocol was linked")
	s.EqualValues(1, s.count("v2_subscription_template"), "a nested template was created")
	var template model.SubscriptionTemplate
	s.Require().NoError(s.db.First(&template, s.template.ID).Error)
	s.Equal(s.groupA.ID, template.GroupID, "a nested template was moved")
}

// nestedAssociations is what a crafted body nested in a group: a new node
// protocol with a new node, the seeded protocol, a new template and the
// seeded template.
const nestedAssociations = `"protocols":[{"name":"injected","port":8443,"node":{"name":"injected-node","host":"evil.example.test"}},{"id":1}],` +
	`"templates":[{"name":"injected-template","server":"evil.example.test","port":443},{"id":1,"name":"tpl"}]`

func (s *SubscriptionAdminWritesTestSuite) TestCreateGroupSavesNoNestedAssociations() {
	handler := NewSubscriptionAdminHandler()
	resp := s.serve("POST", "/groups", "/groups", `{"name":"group-c","enable":1,`+nestedAssociations+`}`, handler.CreateGroup)
	data := resp["data"].(map[string]any)
	s.Equal("group-c", data["name"])
	s.NotContains(data, "protocols", "the answer shows only what was stored")
	s.NotContains(data, "templates", "the answer shows only what was stored")
	s.EqualValues(3, s.count("v2_subscription_group"))
	s.requireNoNestedWrites()
}

func (s *SubscriptionAdminWritesTestSuite) TestUpdateGroupSavesNoNestedAssociations() {
	handler := NewSubscriptionAdminHandler()
	resp := s.serve("PUT", "/groups/:id", "/groups/2", `{"name":"group-b2","enable":1,`+nestedAssociations+`}`, handler.UpdateGroup)
	data := resp["data"].(map[string]any)
	s.Equal("group-b2", data["name"])
	s.NotContains(data, "protocols")
	s.NotContains(data, "templates")
	var group model.SubscriptionGroup
	s.Require().NoError(s.db.First(&group, s.groupB.ID).Error)
	s.Equal("group-b2", group.Name)
	s.requireNoNestedWrites()
}

// A template's nested group neither creates a group nor moves the template
// out of the group its route names.
func (s *SubscriptionAdminWritesTestSuite) TestCreateTemplateKeepsTheRouteGroup() {
	handler := NewSubscriptionAdminHandler()
	for _, nested := range []string{
		`{"id":2,"name":"group-b"}`,
		`{"name":"injected-group","protocols":[{"name":"injected","node":{"name":"injected-node"}}]}`,
	} {
		resp := s.serve("POST", "/groups/:id/templates", "/groups/1/templates",
			`{"name":"tpl-new","type":"vless","server":"b.example.test","port":443,"group":`+nested+`}`, handler.CreateTemplate)
		data := resp["data"].(map[string]any)
		s.EqualValues(s.groupA.ID, data["group_id"], nested)
		s.NotContains(data, "group")
		var stored model.SubscriptionTemplate
		s.Require().NoError(s.db.First(&stored, uint(data["id"].(float64))).Error)
		s.Equal(s.groupA.ID, stored.GroupID, nested)
	}
	s.EqualValues(2, s.count("v2_subscription_group"), "a nested group was created")
	s.EqualValues(1, s.count("v2_node"))
	s.EqualValues(1, s.count("v2_node_protocol"))
}

// The update dropped "id", "created_at" and "updated_at", but GORM also
// resolves field names, and SQLite column names in any case.
func (s *SubscriptionAdminWritesTestSuite) TestUpdateTemplateKeepsItsIDAndTimestamps() {
	handler := NewSubscriptionAdminHandler()
	resp := s.serve("PUT", "/templates/:id", "/templates/1",
		`{"name":"renamed","ID":99,"Id":98,"CreatedAt":"2001-01-01T00:00:00Z","Created_At":"2002-01-01T00:00:00Z","UPDATED_AT":"2003-01-01T00:00:00Z"}`,
		handler.UpdateTemplate)
	data := resp["data"].(map[string]any)
	s.EqualValues(1, data["id"])
	s.Equal("renamed", data["name"])
	var stored model.SubscriptionTemplate
	s.Require().NoError(s.db.First(&stored, s.template.ID).Error)
	s.Equal("renamed", stored.Name)
	s.True(stored.CreatedAt.Equal(subscriptionWritesSeeded), "created_at %s", stored.CreatedAt)
	s.True(stored.UpdatedAt.After(subscriptionWritesSeeded.Add(time.Hour)), "updated_at %s", stored.UpdatedAt)
	s.EqualValues(1, s.count("v2_subscription_template"))
}

func TestSubscriptionAdminWrites(t *testing.T) {
	suite.Run(t, new(SubscriptionAdminWritesTestSuite))
}
