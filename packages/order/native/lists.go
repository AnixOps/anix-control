package native

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// Order list and detail routes, by route id.
const (
	AdminOrdersRouteID = "order.admin.orders.get"
	AdminOrderRouteID  = "order.admin.orders.id.get"
	UserOrdersRouteID  = "order.user.order.get"
	UserOrderRouteID   = "order.user.order.id.get"
)

// PlanName is a row of kapi_plan_name_v1: a plan's id and name, which an
// order answer shows as its plan.
type PlanName struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `json:"name"`
}

// TableName is the kernel view.
func (PlanName) TableName() string { return "kapi_plan_name_v1" }

// Buyer is the part of a kapi_user_directory_v1 row an administrator's
// order answer shows as its buyer.
type Buyer struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Email string `json:"email"`
}

// TableName is the kernel view.
func (Buyer) TableName() string { return "kapi_user_directory_v1" }

// OrderView is an order as the list and detail routes answer it, as the
// kernel's service.OrderView: the order's columns, its plan's id and name,
// and for an administrator its buyer's id and e-mail. A plan or buyer that
// no longer exists is left out.
type OrderView struct {
	Order
	User *Buyer    `json:"user,omitempty"`
	Plan *PlanName `json:"plan,omitempty"`
}

// OrderList is a page of orders and how many match.
type OrderList struct {
	Total int64       `json:"total"`
	List  []OrderView `json:"list"`
}

// Pagination as the kernel's handler.ClampPagination.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

func clampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

// defaultQuery is gin's Context.DefaultQuery: the first value of a query
// parameter that is present, even empty, else fallback.
func defaultQuery(request pluginhostsdk.NativeRequest, key, fallback string) string {
	if values, ok := request.Metadata.Query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return fallback
}

// listParams filters and pages an order list, as the kernel's
// service.OrderListParams.
type listParams struct {
	page, pageSize int
	userID         *uint
	status, kind   *int
	tradeNo, email string
	// orderBy is an ORDER BY clause from v2compat.ParseListSort over
	// OrderSortColumns, never request text; empty lists newest first.
	orderBy string
}

// OrderSortColumns are the columns the administrator's order list sorts by:
// the keys and the meaning of the kernel's service.OrderSortColumns. The
// buyer's e-mail and the plan's name are other tables' and are not
// sortable; an unpaid order (NULL paid_at) sorts last ascending.
var OrderSortColumns = map[string]v2compat.SortColumn{
	"id":           {Expr: "id", Unique: true},
	"trade_no":     {Expr: "trade_no", Unique: true},
	"status":       {Expr: "status"},
	"type":         {Expr: "type"},
	"total_amount": {Expr: "total_amount"},
	"created_at":   {Expr: "created_at"},
	"paid_at":      {Expr: "paid_at", Nullable: true},
}

// OrderSortTiebreaker orders orders that share a sorted value, so pages
// neither repeat nor skip an order.
const OrderSortTiebreaker = "id DESC"

// AdminOrders is GET /api/v2/admin/orders: every order, newest first unless
// sort and order say otherwise (the columns of OrderSortColumns), each with
// its plan and buyer, filtered by buyer, status, type, trade number or part
// of the buyer's e-mail. A filter that does not parse filters by what
// strconv returns, as in the kernel.
func (s *Service) AdminOrders(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	params := listParams{tradeNo: defaultQuery(request, "trade_no", ""), email: defaultQuery(request, "email", "")}
	params.page, params.pageSize = clampPagination(page, pageSize)
	orderBy, err := v2compat.ParseListSort(defaultQuery(request, "sort", ""), defaultQuery(request, "order", ""), OrderSortColumns, OrderSortTiebreaker)
	if err != nil {
		return s.panelError(err.Error())
	}
	params.orderBy = orderBy
	if raw := defaultQuery(request, "status", ""); raw != "" {
		status, _ := strconv.Atoi(raw)
		params.status = &status
	}
	if raw := defaultQuery(request, "type", ""); raw != "" {
		kind, _ := strconv.Atoi(raw)
		params.kind = &kind
	}
	if raw := defaultQuery(request, "user_id", ""); raw != "" {
		id, _ := strconv.ParseUint(raw, 10, 32)
		userID := uint(id)
		params.userID = &userID
	}
	result, err := s.listOrders(ctx, params, true)
	if err != nil {
		return s.panelError("获取订单列表失败: " + err.Error())
	}
	return s.panel(result)
}

// UserOrders is GET /api/v2/user/order: the caller's own orders, newest
// first, each with its plan. A request with no caller lists nothing.
func (s *Service) UserOrders(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "10"))
	page, pageSize = clampPagination(page, pageSize)
	userID := request.Principal.ActorID
	if userID == 0 {
		return s.panel(&OrderList{List: []OrderView{}})
	}
	result, err := s.listOrders(ctx, listParams{page: page, pageSize: pageSize, userID: &userID}, false)
	if err != nil {
		log.Printf("user order list failed: %v", err)
		return s.panelError("获取订单列表失败")
	}
	return s.panel(result)
}

func (s *Service) listOrders(ctx context.Context, params listParams, withBuyer bool) (*OrderList, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	query := db.Model(&Order{})
	if params.userID != nil {
		query = query.Where("user_id = ?", *params.userID)
	}
	if params.status != nil {
		query = query.Where("status = ?", *params.status)
	}
	if params.kind != nil {
		query = query.Where("type = ?", *params.kind)
	}
	if params.tradeNo != "" {
		query = query.Where("trade_no = ?", params.tradeNo)
	}
	if params.email != "" {
		query = query.Where("user_id IN (?)", db.Model(&Buyer{}).Select("id").Where("email LIKE ?", "%"+params.email+"%"))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var orders []Order
	offset := (params.page - 1) * params.pageSize
	orderBy := "created_at DESC, id DESC"
	if params.orderBy != "" {
		orderBy = params.orderBy
	}
	if err := query.Order(orderBy).Offset(offset).Limit(params.pageSize).Find(&orders).Error; err != nil {
		return nil, err
	}
	views, err := orderViews(db, orders, withBuyer)
	if err != nil {
		return nil, err
	}
	return &OrderList{Total: total, List: views}, nil
}

// orderViews names each order's plan and, withBuyer, its buyer, through the
// kernel views.
func orderViews(db *gorm.DB, orders []Order, withBuyer bool) ([]OrderView, error) {
	planIDs := make([]uint, 0, len(orders))
	userIDs := make([]uint, 0, len(orders))
	for _, order := range orders {
		planIDs = append(planIDs, order.PlanID)
		userIDs = append(userIDs, order.UserID)
	}
	plans := map[uint]*PlanName{}
	if len(planIDs) > 0 {
		var rows []PlanName
		if err := db.Select("id", "name").Where("id IN ?", planIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			plans[rows[i].ID] = &rows[i]
		}
	}
	buyers := map[uint]*Buyer{}
	if withBuyer && len(userIDs) > 0 {
		var rows []Buyer
		if err := db.Select("id", "email").Where("id IN ?", userIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			buyers[rows[i].ID] = &rows[i]
		}
	}
	views := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		views = append(views, OrderView{Order: order, User: buyers[order.UserID], Plan: plans[order.PlanID]})
	}
	return views, nil
}

// AdminOrder is GET /api/v2/admin/orders/:id: the order with its plan and
// buyer.
func (s *Service) AdminOrder(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("无效的订单ID")
	}
	view, err := s.orderView(ctx, uint(id), nil)
	if err != nil {
		return s.adminOrderError("获取订单失败", err)
	}
	return s.panel(view)
}

// UserOrder is GET /api/v2/user/order/:id: the caller's own order with its
// plan. Another user's order is not found, as an unknown one.
func (s *Service) UserOrder(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, err := pathID(request)
	if err != nil {
		return s.panelError("ID无效")
	}
	owner := request.Principal.ActorID
	view, err := s.orderView(ctx, uint(id), &owner)
	if err != nil {
		if !errors.Is(err, errOrderNotFound) {
			log.Printf("user order detail lookup failed: %v", err)
		}
		return s.panelError(errOrderNotFound.Error())
	}
	return s.panel(view)
}

// orderView reads one order: any order with its buyer for an administrator
// (owner nil), else only the owner's, without the buyer. Order and owner id
// zero name no order.
func (s *Service) orderView(ctx context.Context, id uint, owner *uint) (*OrderView, error) {
	if id == 0 || (owner != nil && *owner == 0) {
		return nil, errOrderNotFound
	}
	db, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	query := db
	if owner != nil {
		query = db.Where("user_id = ?", *owner)
	}
	var order Order
	if err := query.First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errOrderNotFound
		}
		return nil, err
	}
	views, err := orderViews(db, []Order{order}, owner == nil)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}
