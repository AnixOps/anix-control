package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// UserListResponseTestSuite holds the regression tests for the
// administrator's user list, which answered whole v2_user rows (subscription
// token and proxy uuid included) with their plan rows. It now answers the
// account and subscription summary of each user with its plan's id and
// name, and users created in the same instant keep a stable order across
// pages.
type UserListResponseTestSuite struct {
	HandlerTestSuite
}

// userListFields are the keys of a listed user.
var userListFields = []string{
	"balance", "banned", "commission_balance", "created_at", "d", "device_limit", "email", "expired_at", "flowResetTime",
	"group_id", "id", "is_admin", "is_staff", "plan_id", "speed_limit", "transfer_enable", "u",
}

// list answers GET /admin/users with query and returns the raw body and the
// decoded data.
func (s *UserListResponseTestSuite) list(query string) (string, struct {
	Total int64            `json:"total"`
	List  []map[string]any `json:"list"`
}) {
	router := gin.New()
	router.GET("/admin/users", NewAdminHandler().GetUserList)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/users"+query, nil))
	s.Require().Equal(http.StatusOK, w.Code)
	var answer struct {
		Code int `json:"code"`
		Data struct {
			Total int64            `json:"total"`
			List  []map[string]any `json:"list"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &answer), w.Body.String())
	s.Require().Zero(answer.Code, w.Body.String())
	return w.Body.String(), answer.Data
}

// The list carries no subscription token, proxy uuid or other column of the
// user row, and of the plan only its id and name; each user has exactly the
// listed fields, "plan" while the plan exists.
func (s *UserListResponseTestSuite) TestListShowsNoCredentials() {
	note, content := "remark only the detail shows", "plan description"
	plan := &model.Plan{Name: "Pro", Content: &content}
	s.Require().NoError(s.db.Create(plan).Error)
	user := &model.User{
		Email: "listed@example.test", Password: "hash", Token: "listed-subscription-token", UUID: "listed-proxy-uuid",
		PlanID: &plan.ID, RemarkContent: &note, Balance: 4200, CommissionBalance: 70, TransferEnable: 1000, U: 1, D: 2,
	}
	s.Require().NoError(s.db.Create(user).Error)
	gone := plan.ID + 100
	orphan := &model.User{Email: "orphan@example.test", Password: "hash", Token: "orphan-token", UUID: "orphan-uuid", PlanID: &gone}
	s.Require().NoError(s.db.Create(orphan).Error)

	raw, data := s.list("")
	for _, hidden := range []string{"listed-subscription-token", "listed-proxy-uuid", "orphan-token", "orphan-uuid", note, content, "\"token\"", "\"uuid\"", "\"content\""} {
		s.NotContains(raw, hidden)
	}
	s.Require().EqualValues(2, data.Total)
	s.Require().Len(data.List, 2)
	listed, orphaned := data.List[1], data.List[0]
	s.Equal("listed@example.test", listed["email"])
	withPlan := append(append([]string(nil), userListFields...), "plan")
	sort.Strings(withPlan)
	s.Equal(withPlan, userListKeys(listed))
	s.Equal(map[string]any{"id": float64(plan.ID), "name": "Pro"}, listed["plan"])
	s.EqualValues(plan.ID, listed["plan_id"])
	s.EqualValues(4200, listed["balance"])
	s.EqualValues(70, listed["commission_balance"])
	s.EqualValues(1000, listed["transfer_enable"])
	s.Equal("orphan@example.test", orphaned["email"])
	s.Equal(userListFields, userListKeys(orphaned), "a plan that no longer exists is left out")
	s.EqualValues(gone, orphaned["plan_id"])
}

// userListKeys are the sorted keys of a listed user.
func userListKeys(user map[string]any) []string {
	keys := make([]string, 0, len(user))
	for key := range user {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Users created in the same instant are ordered by id, newest first, so
// pages neither repeat nor skip a user.
func (s *UserListResponseTestSuite) TestPagesOfUsersCreatedTogether() {
	created := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		user := &model.User{
			Email: fmt.Sprintf("tie%d@example.test", i), Password: "hash", Token: fmt.Sprintf("tie-token-%d", i),
			UUID: fmt.Sprintf("tie-uuid-%d", i), CreatedAt: created,
		}
		s.Require().NoError(s.db.Create(user).Error)
	}
	var emails []any
	for page := 1; page <= 3; page++ {
		_, data := s.list(fmt.Sprintf("?page=%d&page_size=2", page))
		s.EqualValues(5, data.Total)
		for _, user := range data.List {
			emails = append(emails, user["email"])
		}
	}
	s.Equal([]any{"tie5@example.test", "tie4@example.test", "tie3@example.test", "tie2@example.test", "tie1@example.test"}, emails)
}

func TestUserListResponseTestSuite(t *testing.T) {
	suite.Run(t, new(UserListResponseTestSuite))
}
