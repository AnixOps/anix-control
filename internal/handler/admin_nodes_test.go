package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// AdminNodesTestSuite checks the console's node list: the /api/v2 list's
// rows with each node's approval, node group and the group's name.
type AdminNodesTestSuite struct {
	HandlerTestSuite
	group    model.NodeGroup
	pending  model.Node
	approved model.Node
	manual   model.Node
}

func (s *AdminNodesTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.group = model.NodeGroup{Name: "HK", JoinTokenHash: "hash-hk", JoinTokenSealed: "sealed", ResetDay: 1, Rate: 1}
	s.Require().NoError(s.db.Create(&s.group).Error)
	groupID := s.group.ID
	s.pending = model.Node{Name: "hk-02", Host: "203.0.113.6",
		NodeGroupID: &groupID, Approval: model.NodeApprovalPending, Sort: 2}
	s.approved = model.Node{Name: "hk-01", Host: "203.0.113.5",
		NodeGroupID: &groupID, Approval: model.NodeApprovalApproved, Sort: 1}
	s.manual = model.Node{Name: "hand-made", Host: "198.51.100.9", Approval: model.NodeApprovalApproved, Sort: 3}
	for _, node := range []*model.Node{&s.pending, &s.approved, &s.manual} {
		s.Require().NoError(s.db.Create(node).Error)
	}
	reality := `{"private_key":"fake-reality-private","public_key":"pub","short_id":"ab"}`
	s.Require().NoError(s.db.Create(&model.NodeProtocol{NodeID: s.approved.ID, Name: "reality", Type: model.ProtocolVLESS, Port: 443, TLS: 2, RealitySettings: &reality}).Error)
}

type adminNodesAnswer struct {
	Data struct {
		Total int64            `json:"total"`
		List  []map[string]any `json:"list"`
	} `json:"data"`
}

func (s *AdminNodesTestSuite) list(query string) (adminNodesAnswer, *httptest.ResponseRecorder) {
	router := gin.New()
	router.GET("/nodes", NewAdminNodesHandler().List)
	request := httptest.NewRequest(http.MethodGet, "/nodes"+query, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var answer adminNodesAnswer
	if recorder.Code == http.StatusOK {
		s.Require().NoError(json.Unmarshal(recorder.Body.Bytes(), &answer), recorder.Body.String())
	}
	return answer, recorder
}

func (s *AdminNodesTestSuite) names(answer adminNodesAnswer) []string {
	names := make([]string, 0, len(answer.Data.List))
	for _, row := range answer.Data.List {
		names = append(names, row["name"].(string))
	}
	return names
}

func (s *AdminNodesTestSuite) TestRowsCarryApprovalAndNodeGroup() {
	answer, recorder := s.list("")
	s.Require().Equal(http.StatusOK, recorder.Code, recorder.Body.String())
	s.EqualValues(3, answer.Data.Total)
	s.Equal([]string{"hk-01", "hk-02", "hand-made"}, s.names(answer), "the administrator's order: sort ascending")

	byName := map[string]map[string]any{}
	for _, row := range answer.Data.List {
		byName[row["name"].(string)] = row
	}
	s.Equal("approved", byName["hk-01"]["approval"])
	s.EqualValues(s.group.ID, byName["hk-01"]["node_group_id"])
	s.Equal("HK", byName["hk-01"]["node_group_name"])
	s.Equal("pending", byName["hk-02"]["approval"])
	s.Equal("approved", byName["hand-made"]["approval"])
	s.Nil(byName["hand-made"]["node_group_id"])
	s.Equal("", byName["hand-made"]["node_group_name"])

	// The rest of each row is the /api/v2 list's: the same fields, and no
	// credential or secret.
	s.Contains(byName["hk-01"], "host")
	s.Contains(byName["hk-01"], "last_check_at")
	body := recorder.Body.String()
	for _, secret := range []string{"key-1", "key-2", "secret-1", "hash-1", "fake-reality-private", "machine_fingerprint", "api_key"} {
		s.NotContains(body, secret)
	}
	protocols, ok := byName["hk-01"]["protocols"].([]any)
	s.Require().True(ok)
	s.Len(protocols, 1)
}

func (s *AdminNodesTestSuite) TestFilters() {
	answer, _ := s.list("?approval=pending")
	s.EqualValues(1, answer.Data.Total)
	s.Equal([]string{"hk-02"}, s.names(answer))

	answer, _ = s.list("?approval=approved&sort=name&order=desc")
	s.Equal([]string{"hk-01", "hand-made"}, s.names(answer))

	answer, _ = s.list("?node_group_id=" + jsonNumber(s.group.ID))
	s.EqualValues(2, answer.Data.Total)
	s.Equal([]string{"hk-01", "hk-02"}, s.names(answer))

	answer, _ = s.list("?search=203.0.113.5&approval=approved")
	s.Equal([]string{"hk-01"}, s.names(answer))

	answer, _ = s.list("?page=2&page_size=2")
	s.EqualValues(3, answer.Data.Total)
	s.Equal([]string{"hand-made"}, s.names(answer))

	for _, query := range []string{"?approval=maybe", "?node_group_id=x", "?node_group_id=0", "?sort=password"} {
		_, recorder := s.list(query)
		s.Equal(http.StatusBadRequest, recorder.Code, query)
		s.Contains(recorder.Body.String(), "invalid_request", query)
	}
}

func (s *AdminNodesTestSuite) TestADeletedGroupStillNamesItsRejectedNodes() {
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.pending.ID).Update("approval", model.NodeApprovalRejected).Error)
	s.Require().NoError(s.db.Model(&model.Node{}).Where("id = ?", s.approved.ID).Update("node_group_id", nil).Error)
	s.Require().NoError(s.db.Delete(&s.group).Error)

	answer, _ := s.list("?approval=rejected")
	s.Require().Len(answer.Data.List, 1)
	s.Equal("rejected", answer.Data.List[0]["approval"])
	s.Equal("HK", answer.Data.List[0]["node_group_name"])
}

func jsonNumber(id uint) string {
	encoded, _ := json.Marshal(id)
	return string(encoded)
}

func TestAdminNodesTestSuite(t *testing.T) {
	suite.Run(t, new(AdminNodesTestSuite))
}
