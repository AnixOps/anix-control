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

// newOrderService returns the order host's router. Every route in
// orderRoutes has a native handler on the adopted v2_order and v2_coupon
// tables and the plan and user views; such a route serves natively once the
// kernel sets its mode, and falls back to the legacy handler otherwise.
// Completing an order calls the kernel's KernelSubscriber over the bridge
// connection (local socket or module listener); a bridge without one leaves
// it legacy.
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
			_, ok := orderRoutes[routeID]
			return ok
		},
		Native: service.Handlers(),
	})
}

// orderRoutes are the package's compatibility routes, all with a native
// handler. The order list and detail routes answer the order, its plan's id
// and name and, for an administrator, its buyer's id and e-mail, which the
// kernel views show; they embedded the buyer's whole v2_user row
// (subscription token and proxy UUID included) and stayed bridged until the
// kernel's answers were slimmed to that.
var orderRoutes = map[string]struct{}{
	"order.admin.coupon.get":            {},
	"order.admin.coupon.post":           {},
	"order.admin.coupon.id.delete":      {},
	"order.user.coupon.check.post":      {},
	"order.admin.orders.get":            {},
	"order.admin.orders.id.get":         {},
	"order.admin.orders.stats.get":      {},
	"order.admin.orders.id.status.put":  {},
	"order.admin.orders.id.cancel.post": {},
	"order.admin.orders.id.paid.post":   {},
	"order.user.order.get":              {},
	"order.user.order.id.get":           {},
	"order.user.order.save.post":        {},
}
