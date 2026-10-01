package main

import (
	"context"
	"log"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// paymentBridge is what the payment host needs from the package bridge.
type paymentBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newPaymentService returns the payment host's router. The routes in
// paymentRoutes have a native handler on the adopted v2_payment_gateway,
// v2_payment_record and v2_payment tables and the order view; such a route
// serves natively once the kernel sets its mode, and falls back to the
// legacy handler otherwise. The callbacks complete orders through the
// kernel's KernelOrder over the bridge connection (local socket or module
// listener); a bridge without one leaves them legacy.
func newPaymentService(bridge paymentBridge, leaseID string) (*pluginhostsdk.Router, error) {
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
		service.Orders = kernelorderv1.NewKernelOrderClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "payment", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, allowed := paymentRoutes[routeID]
			return allowed
		},
		Native: service.Handlers(),
	})
}

// paymentRoutes are the package's compatibility routes, each with a native
// handler; the callbacks' need the bridge connection.
var paymentRoutes = map[string]struct{}{
	"payment.admin.payment.gateways.get":            {},
	"payment.admin.payment.gateways.post":           {},
	"payment.admin.payment.gateways.id.put":         {},
	"payment.admin.payment.gateways.id.delete":      {},
	"payment.admin.payment.gateways.id.toggle.post": {},
	"payment.admin.payment.records.get":             {},
	"payment.admin.payment.stats.get":               {},
	"payment.user.payment.channels.get":             {},
	"payment.user.payment.create.post":              {},
	"payment.user.payment.status.trade_no.get":      {},
	"payment.user.payment.records.get":              {},
	"payment.payment.methods.get":                   {},
	"payment.payment.status.trade_no.get":           {},
	"payment.payment.x402.create.post":              {},
	"payment.payment.x402.check.id.get":             {},
	"payment.payment.fiat.create.post":              {},
	"payment.callback":                              {},
	"payment.payment.x402.callback.post":            {},
	"payment.payment.stripe.webhook.post":           {},
	"payment.payment.paypal.webhook.post":           {},
}
