package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/payment/native"
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
// legacy handler otherwise. The routes in bridgedRoutes always relay to the
// legacy handler.
func newPaymentService(bridge paymentBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "payment", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := paymentRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// paymentRoutes are the package's compatibility routes with a native
// handler.
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
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. A paid
// callback or webhook marks the payment record and the gateway statistics,
// marks the order paid and completes it (granting its plan), all in one
// kernel transaction. The order is the order package's table and there is
// no contract for those writes yet, so the callbacks stay with the kernel,
// and with them the provider signature checks. The PayPal webhook also calls
// PayPal's API to verify each delivery.
var bridgedRoutes = map[string]struct{}{
	"payment.callback":                    {},
	"payment.payment.x402.callback.post":  {},
	"payment.payment.stripe.webhook.post": {},
	"payment.payment.paypal.webhook.post": {},
}
