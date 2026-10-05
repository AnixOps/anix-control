package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// AdminListSortTestSuite covers the sort and order query values of the
// administrator's user, order and node lists: the order they answer, that
// the default order is untouched without them, and that a column or
// direction outside the list is refused in each list's own error shape.
type AdminListSortTestSuite struct {
	HandlerTestSuite
}

// TearDownSuite closes the shared database as the other suites do, so the
// next suite opens its own, and drops the StatsService NewAdminHandler built
// on it.
func (s *AdminListSortTestSuite) TearDownSuite() {
	s.Require().NoError(database.Close())
	service.ResetStatsServiceForTest()
}

func (s *AdminListSortTestSuite) get(handler gin.HandlerFunc, path, query string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET(path, handler)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+query, nil))
	return w
}

// listIDs reads the ids of a panel list answer.
func (s *AdminListSortTestSuite) listIDs(w *httptest.ResponseRecorder) []uint {
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	var answer struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID uint `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &answer), w.Body.String())
	s.Require().Zero(answer.Code, w.Body.String())
	ids := make([]uint, 0, len(answer.Data.List))
	for _, item := range answer.Data.List {
		ids = append(ids, item.ID)
	}
	return ids
}

func (s *AdminListSortTestSuite) TestUsersSortByTheColumnsAsked() {
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	late, early := int64(2_000_000_000), int64(1_900_000_000)
	users := []model.User{
		{ID: 1, Email: "c@example.test", Password: "x", Token: "t1", UUID: "u1", CreatedAt: base.Add(2 * time.Hour), ExpiredAt: &late, U: 5, D: 5, TransferEnable: 50},
		{ID: 2, Email: "a@example.test", Password: "x", Token: "t2", UUID: "u2", CreatedAt: base.Add(time.Hour), ExpiredAt: &early, U: 1, D: 1, TransferEnable: 90},
		{ID: 3, Email: "b@example.test", Password: "x", Token: "t3", UUID: "u3", CreatedAt: base, U: 1, D: 1, TransferEnable: 10},
		{ID: 4, Email: "d@example.test", Password: "x", Token: "t4", UUID: "u4", CreatedAt: base.Add(time.Hour), U: 9, D: 0, TransferEnable: 90},
	}
	s.Require().NoError(s.db.Create(&users).Error)
	handler := NewAdminHandler().GetUserList
	for query, want := range map[string][]uint{
		"":                                {1, 4, 2, 3},
		"?sort=email":                     {2, 3, 1, 4},
		"?sort=email&order=desc":          {4, 1, 3, 2},
		"?sort=created_at":                {3, 4, 2, 1},
		"?sort=created_at&order=DESC":     {1, 4, 2, 3},
		"?sort=expired_at":                {2, 1, 4, 3},
		"?sort=expired_at&order=desc":     {4, 3, 1, 2},
		"?sort=traffic&order=desc":        {1, 4, 3, 2},
		"?sort=transfer_enable&order=asc": {3, 1, 4, 2},
		"?sort=id&order=desc":             {4, 3, 2, 1},
		"?sort=&order=asc":                {1, 4, 2, 3},
		"?order=asc":                      {1, 4, 2, 3},
		"?sort=email&page=2&page_size=2":  {1, 4},
	} {
		s.Equal(want, s.listIDs(s.get(handler, "/admin/users", query)), query)
	}
}

func (s *AdminListSortTestSuite) TestUsersRefuseWhatIsNotListed() {
	handler := NewAdminHandler().GetUserList
	for _, query := range []string{
		"?sort=password", "?sort=" + url.QueryEscape("email; DROP TABLE v2_user"), "?sort=EMAIL", "?sort=email&order=sideways",
	} {
		w := s.get(handler, "/admin/users", query)
		assertPanelTestError(s.T(), w, "无效的排序")
	}
}

func (s *AdminListSortTestSuite) TestOrdersSortByTheColumnsAsked() {
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	paidEarly, paidLate := int64(1_800_000_000), int64(1_800_100_000)
	plan := model.Plan{Name: "Pro"}
	s.Require().NoError(s.db.Create(&plan).Error)
	user := model.User{Email: "buyer@example.test", Password: "x", Token: "tb", UUID: "ub"}
	s.Require().NoError(s.db.Create(&user).Error)
	orders := []model.Order{
		{ID: 1, UserID: user.ID, PlanID: plan.ID, TradeNo: "T1", TotalAmount: 300, Status: 1, Type: 1, PaidAt: &paidLate, CreatedAt: base},
		{ID: 2, UserID: user.ID, PlanID: plan.ID, TradeNo: "T2", TotalAmount: 100, Status: 0, Type: 2, CreatedAt: base.Add(time.Hour)},
		{ID: 3, UserID: user.ID, PlanID: plan.ID, TradeNo: "T3", TotalAmount: 300, Status: 1, Type: 1, PaidAt: &paidEarly, CreatedAt: base.Add(2 * time.Hour)},
		{ID: 4, UserID: user.ID, PlanID: plan.ID, TradeNo: "T4", TotalAmount: 200, Status: 2, Type: 1, CreatedAt: base.Add(2 * time.Hour)},
	}
	s.Require().NoError(s.db.Create(&orders).Error)
	handler := NewAdminHandler().GetOrderList
	for query, want := range map[string][]uint{
		"":                                      {4, 3, 2, 1},
		"?sort=total_amount&order=desc":         {3, 1, 4, 2},
		"?sort=total_amount":                    {2, 4, 3, 1},
		"?sort=created_at":                      {1, 2, 4, 3},
		"?sort=status&order=desc":               {4, 3, 1, 2},
		"?sort=paid_at":                         {3, 1, 4, 2},
		"?sort=paid_at&order=desc":              {4, 2, 1, 3},
		"?sort=trade_no&order=desc":             {4, 3, 2, 1},
		"?sort=id":                              {1, 2, 3, 4},
		"?sort=total_amount&status=1":           {3, 1},
		"?sort=total_amount&page=2&page_size=3": {1},
	} {
		s.Equal(want, s.listIDs(s.get(handler, "/admin/orders", query)), query)
	}
	for _, query := range []string{"?sort=user", "?sort=plan_id", "?sort=id%3B%20DROP%20TABLE%20v2_order", "?sort=id&order=up"} {
		assertPanelTestError(s.T(), s.get(handler, "/admin/orders", query), "无效的排序")
	}
}

func (s *AdminListSortTestSuite) TestNodesSortByTheColumnsAsked() {
	now := time.Now().Unix()
	recent, stale := now-10, now-10_000
	nodes := []model.Node{
		{ID: 1, Name: "beta", Host: "b.example.test", APIKey: "k1", Sort: 2, LastCheckAt: &recent, CPUUsage: 30, OnlineUsers: 5},
		{ID: 2, Name: "alpha", Host: "c.example.test", APIKey: "k2", Sort: 1, LastCheckAt: &stale, CPUUsage: 90, OnlineUsers: 1},
		{ID: 3, Name: "gamma", Host: "a.example.test", APIKey: "k3", Sort: 2, CPUUsage: 10, OnlineUsers: 9},
	}
	s.Require().NoError(s.db.Create(&nodes).Error)
	handler := NewNodeHandler().GetNodes
	for query, want := range map[string][]uint{
		"":                               {2, 3, 1},
		"?sort=name":                     {2, 1, 3},
		"?sort=name&order=desc":          {3, 1, 2},
		"?sort=host":                     {3, 1, 2},
		"?sort=sort&order=desc":          {3, 1, 2},
		"?sort=last_check_at":            {2, 1, 3},
		"?sort=last_check_at&order=desc": {3, 1, 2},
		"?sort=cpu_usage&order=desc":     {2, 1, 3},
		"?sort=online_users&order=desc":  {3, 1, 2},
		"?sort=id&order=desc":            {3, 2, 1},
		"?sort=&order=desc":              {2, 3, 1},
		"?sort=name&page=2&page_size=2":  {3},
	} {
		w := s.get(handler, "/admin/nodes", query)
		s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
		s.Equal(want, s.listIDs(w), query)
	}
	for _, query := range []string{"?sort=status", "?sort=api_key", "?sort=id&order=up"} {
		w := s.get(handler, "/admin/nodes", query)
		s.Equal(http.StatusBadRequest, w.Code, query)
		var answer map[string]any
		s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &answer))
		s.Contains(answer["message"], "无效的排序", query)
	}
}

func TestAdminListSortTestSuite(t *testing.T) {
	suite.Run(t, new(AdminListSortTestSuite))
}
