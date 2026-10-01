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

// OrderResponseTestSuite holds the regression tests for the order list and
// detail answers, which embedded the buyer's whole v2_user row
// (subscription token and UUID included) and the whole v2_plan row. They now
// carry the order, its plan's id and name and, for an administrator, its
// buyer's id and e-mail. A user sees only their own orders.
type OrderResponseTestSuite struct {
	HandlerTestSuite
	buyer, other *model.User
	plan         *model.Plan
	// own is the buyer's order, foreign another user's.
	own, foreign *model.Order
}

// orderColumns are the keys of an order's own columns in an answer.
var orderColumns = []string{
	"balance", "callback_no", "commission_balance", "commission_status", "coupon_id", "created_at", "discount_amount", "id",
	"invite_user_id", "paid_at", "payment_id", "period", "plan_id", "refund_amount", "status", "surplus_amount",
	"surplus_order", "total_amount", "trade_no", "type", "updated_at", "user_id",
}

// Secrets and row fields of the buyer and the plan that no answer may carry.
const (
	buyerToken = "buyer-subscription-token"
	buyerUUID  = "8f0e5b3c-buyer-proxy-uuid"
	otherToken = "other-subscription-token"
	otherUUID  = "2a9d7c4e-other-proxy-uuid"
	remark     = "buyer remark only admins wrote"
	planText   = "plan description content"
)

func (s *OrderResponseTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	note := remark
	s.buyer = &model.User{Email: "buyer@example.test", Password: "hash", Token: buyerToken, UUID: buyerUUID, Balance: 4200, RemarkContent: &note}
	s.other = &model.User{Email: "other@example.test", Password: "hash", Token: otherToken, UUID: otherUUID}
	s.Require().NoError(s.db.Create(s.buyer).Error)
	s.Require().NoError(s.db.Create(s.other).Error)
	content, price := planText, int64(3000)
	s.plan = &model.Plan{Name: "Pro", Content: &content, MonthPrice: &price, TransferEnable: 100}
	s.Require().NoError(s.db.Create(s.plan).Error)
	created := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	s.own = &model.Order{UserID: s.buyer.ID, PlanID: s.plan.ID, Period: "month", TradeNo: "OWN1", TotalAmount: 3000, CreatedAt: created}
	s.foreign = &model.Order{UserID: s.other.ID, PlanID: s.plan.ID, Period: "month", TradeNo: "FOREIGN1", TotalAmount: 3000, CreatedAt: created.Add(time.Minute)}
	s.Require().NoError(s.db.Create(s.own).Error)
	s.Require().NoError(s.db.Create(s.foreign).Error)
}

// get serves one request as user (0 for none) and returns the raw body and
// its data.
func (s *OrderResponseTestSuite) get(pattern, path string, user uint, serve gin.HandlerFunc) (string, any) {
	router := gin.New()
	router.GET(pattern, func(c *gin.Context) {
		if user != 0 {
			c.Set("user_id", user)
		}
		serve(c)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	s.Require().Equal(http.StatusOK, w.Code)
	var answer struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data any    `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &answer), w.Body.String())
	if answer.Code != 0 {
		return w.Body.String(), answer.Msg
	}
	return w.Body.String(), answer.Data
}

func keys(object map[string]any) []string {
	out := make([]string, 0, len(object))
	for key := range object {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// requireSlim checks an order answer: the order's columns, the plan's id
// and name, and the buyer's id and e-mail when withBuyer.
func (s *OrderResponseTestSuite) requireSlim(raw string, order any, withBuyer bool) {
	for _, secret := range []string{buyerToken, buyerUUID, otherToken, otherUUID, remark, planText, "hash"} {
		s.NotContains(raw, secret, "the answer carries no buyer or plan row field")
	}
	object, ok := order.(map[string]any)
	s.Require().True(ok, "%v", order)
	want := append([]string{"plan"}, orderColumns...)
	if withBuyer {
		want = append(want, "user")
	}
	sort.Strings(want)
	s.Equal(want, keys(object))
	plan, ok := object["plan"].(map[string]any)
	s.Require().True(ok)
	s.Equal([]string{"id", "name"}, keys(plan))
	s.Equal("Pro", plan["name"])
	s.EqualValues(s.plan.ID, plan["id"])
	if withBuyer {
		buyer, ok := object["user"].(map[string]any)
		s.Require().True(ok)
		s.Equal([]string{"email", "id"}, keys(buyer))
		s.EqualValues(object["user_id"], buyer["id"])
	}
}

func (s *OrderResponseTestSuite) list(data any) []any {
	page, ok := data.(map[string]any)
	s.Require().True(ok, "%v", data)
	s.Equal([]string{"list", "total"}, keys(page))
	list, ok := page["list"].([]any)
	s.Require().True(ok, "%v", page)
	return list
}

func (s *OrderResponseTestSuite) TestAdminListNamesPlanAndBuyerOnly() {
	raw, data := s.get("/admin/orders", "/admin/orders", 1, NewAdminHandler().GetOrderList)
	list := s.list(data)
	s.Require().Len(list, 2)
	for _, order := range list {
		s.requireSlim(raw, order, true)
	}
	s.Equal("FOREIGN1", list[0].(map[string]any)["trade_no"], "newest first")
	s.Equal(map[string]any{"id": float64(s.other.ID), "email": "other@example.test"}, list[0].(map[string]any)["user"])
}

// The e-mail filter joined v2_user, which made the ordering's created_at
// ambiguous: every search by e-mail failed.
func (s *OrderResponseTestSuite) TestAdminListFiltersByEmail() {
	_, data := s.get("/admin/orders", "/admin/orders?email=buyer@", 1, NewAdminHandler().GetOrderList)
	list := s.list(data)
	s.Require().Len(list, 1)
	s.Equal("OWN1", list[0].(map[string]any)["trade_no"])
	s.EqualValues(1, data.(map[string]any)["total"])
	_, data = s.get("/admin/orders", "/admin/orders?email=nobody", 1, NewAdminHandler().GetOrderList)
	s.Empty(s.list(data))
}

func (s *OrderResponseTestSuite) TestAdminDetailNamesPlanAndBuyerOnly() {
	raw, data := s.get("/admin/orders/:id", fmt.Sprintf("/admin/orders/%d", s.own.ID), 1, NewAdminHandler().GetOrder)
	s.requireSlim(raw, data, true)
	s.Equal(map[string]any{"id": float64(s.buyer.ID), "email": "buyer@example.test"}, data.(map[string]any)["user"])
	_, message := s.get("/admin/orders/:id", "/admin/orders/999", 1, NewAdminHandler().GetOrder)
	s.Equal("订单不存在", message)
}

// A plan or buyer that no longer exists is left out of the answer.
func (s *OrderResponseTestSuite) TestMissingPlanAndBuyerAreLeftOut() {
	s.Require().NoError(s.db.Delete(&model.Plan{}, s.plan.ID).Error)
	s.Require().NoError(s.db.Delete(&model.User{}, s.other.ID).Error)
	_, data := s.get("/admin/orders/:id", fmt.Sprintf("/admin/orders/%d", s.foreign.ID), 1, NewAdminHandler().GetOrder)
	object := data.(map[string]any)
	s.NotContains(object, "plan")
	s.NotContains(object, "user")
	_, data = s.get("/admin/orders/:id", fmt.Sprintf("/admin/orders/%d", s.own.ID), 1, NewAdminHandler().GetOrder)
	s.Contains(data.(map[string]any), "user")
}

func (s *OrderResponseTestSuite) TestUserListShowsOwnOrdersWithoutTheBuyer() {
	raw, data := s.get("/user/order", "/user/order", s.buyer.ID, NewOrderHandler().GetOrders)
	list := s.list(data)
	s.Require().Len(list, 1, "only the caller's orders")
	s.requireSlim(raw, list[0], false)
	s.Equal("OWN1", list[0].(map[string]any)["trade_no"])
	// Query parameters cannot widen the list to another user.
	_, data = s.get("/user/order", fmt.Sprintf("/user/order?user_id=%d", s.other.ID), s.buyer.ID, NewOrderHandler().GetOrders)
	s.Len(s.list(data), 1)
	s.Equal("OWN1", s.list(data)[0].(map[string]any)["trade_no"])
	// No principal names no one.
	_, data = s.get("/user/order", "/user/order", 0, NewOrderHandler().GetOrders)
	s.Empty(s.list(data))
}

// The page size is clamped as on the administrator's list: 0 or less is 20
// (0 answered nothing and a negative size every order), at most 100.
func (s *OrderResponseTestSuite) TestUserListClampsThePageSize() {
	for i := range 25 {
		s.Require().NoError(s.db.Create(&model.Order{UserID: s.buyer.ID, PlanID: s.plan.ID, Period: "month", TradeNo: fmt.Sprintf("BULK%d", i)}).Error)
	}
	for _, size := range []string{"0", "-1", "x"} {
		_, data := s.get("/user/order", "/user/order?page_size="+size, s.buyer.ID, NewOrderHandler().GetOrders)
		s.Len(s.list(data), 20, "page_size=%s", size)
		s.EqualValues(26, data.(map[string]any)["total"])
	}
	_, data := s.get("/user/order", "/user/order?page=2&page_size=20", s.buyer.ID, NewOrderHandler().GetOrders)
	s.Len(s.list(data), 6)
	_, data = s.get("/user/order", "/user/order?page=-3", s.buyer.ID, NewOrderHandler().GetOrders)
	s.Len(s.list(data), 10, "page 1 of the default 10")
}

func (s *OrderResponseTestSuite) TestUserDetailIsTheCallersOwnOrder() {
	raw, data := s.get("/user/order/:id", fmt.Sprintf("/user/order/%d", s.own.ID), s.buyer.ID, NewOrderHandler().GetOrderDetail)
	s.requireSlim(raw, data, false)
	for _, path := range []string{
		fmt.Sprintf("/user/order/%d", s.foreign.ID), // another user's order
		"/user/order/999", // an unknown order
		"/user/order/0",
	} {
		_, message := s.get("/user/order/:id", path, s.buyer.ID, NewOrderHandler().GetOrderDetail)
		s.Equal("订单不存在", message, path)
	}
	_, message := s.get("/user/order/:id", "/user/order/x", s.buyer.ID, NewOrderHandler().GetOrderDetail)
	s.Equal("ID无效", message)
	// An order without a buyer is no one's, not the caller's without an id.
	orphan := &model.Order{UserID: 0, PlanID: s.plan.ID, Period: "month", TradeNo: "NOBODY"}
	s.Require().NoError(s.db.Create(orphan).Error)
	_, message = s.get("/user/order/:id", fmt.Sprintf("/user/order/%d", orphan.ID), 0, NewOrderHandler().GetOrderDetail)
	s.Equal("订单不存在", message)
}

func TestOrderResponses(t *testing.T) {
	suite.Run(t, new(OrderResponseTestSuite))
}
