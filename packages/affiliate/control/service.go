package main

import (
	"context"
	"log"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/affiliate/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// affiliateBridge is what the affiliate host needs from the package bridge.
type affiliateBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newAffiliateService returns the affiliate host's router. The routes in
// affiliateRoutes have a native handler on the adopted
// v2_commission_record, v2_commission_withdraw, v2_invite_config and
// v2_invite_code tables and the referral, order billing, entitlement and
// settings views; such a route serves
// natively once the kernel sets its mode, and falls back to the legacy
// handler otherwise. Withdrawing and processing a withdrawal change a
// commission balance through the kernel's KernelSubscriber, and updating
// the configuration writes the frontend settings through its
// KernelSettings, over the bridge connection (local socket or module
// listener); a bridge without one leaves them legacy.
func newAffiliateService(bridge affiliateBridge, leaseID string) (*pluginhostsdk.Router, error) {
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
		service.KernelSettings = kernelsettingsv1.NewKernelSettingsClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "affiliate", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := affiliateRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// affiliateRoutes are the package's compatibility routes with a native
// handler: all of them.
var affiliateRoutes = map[string]struct{}{
	"affiliate.admin.invite.config.get":                  {},
	"affiliate.admin.invite.config.put":                  {},
	"affiliate.admin.invite.stats.get":                   {},
	"affiliate.admin.invite.withdrawals.get":             {},
	"affiliate.admin.invite.withdrawals.id.process.post": {},
	"affiliate.user.invite.commissions.get":              {},
	"affiliate.user.invite.withdraw.post":                {},
	"affiliate.user.invite.withdrawals.get":              {},
	// The caller's invite codes and their generation (v2_invite_code),
	// moved from identity-platform.
	"affiliate.user.invite.get":           {},
	"affiliate.user.invite.generate.post": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler. There are
// none left.
var bridgedRoutes = map[string]struct{}{}
