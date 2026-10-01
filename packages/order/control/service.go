package main

import (
	"context"
	"log"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/order/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// orderBridge is what the order host needs from the package bridge.
type orderBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newOrderService returns the order host's router. The routes in
// orderRoutes have a native handler on the adopted v2_order and v2_coupon
// tables and the plan and user views; such a route serves natively once the
// kernel sets its mode, and falls back to the legacy handler otherwise.
// Completing an order calls the kernel's KernelSubscriber over the bridge
// connection (local socket or module listener); a bridge without one leaves
// it legacy. The routes in bridgedRoutes always relay to the legacy handler.
func newOrderService(bridge orderBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Subscriber = kernelsubscriberv1.NewKernelSubscriberClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "order", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := orderRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// orderRoutes are the package's compatibility routes with a native handler.
var orderRoutes = map[string]struct{}{
	"order.admin.coupon.get":            {},
	"order.admin.coupon.post":           {},
	"order.admin.coupon.id.delete":      {},
	"order.user.coupon.check.post":      {},
	"order.admin.orders.stats.get":      {},
	"order.admin.orders.id.status.put":  {},
	"order.admin.orders.id.cancel.post": {},
	"order.admin.orders.id.paid.post":   {},
	"order.user.order.save.post":        {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. The order list
// and detail answers embed the buyer's whole v2_user row (subscription
// token, proxy UUID, balances, remark) and the plan's; no kernel view may
// expose the token or UUID, and the package does not adopt v2_user. They
// stay with the kernel until those answers change or identity serves them.
var bridgedRoutes = map[string]struct{}{
	"order.admin.orders.get":    {},
	"order.admin.orders.id.get": {},
	"order.user.order.get":      {},
	"order.user.order.id.get":   {},
}
