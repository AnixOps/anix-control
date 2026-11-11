package control

import (
	"context"
	"log"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/packages/plan/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// planBridge is what the plan host needs from the package bridge.
type planBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newPlanService returns the plan host's router. The routes in planRoutes
// have a native handler on the adopted v2_plan and v2_event tables and serve
// natively from the start: the kernel keeps no legacy handler for them. The
// assignment calls the kernel's KernelSubscriber over the bridge connection
// (local socket or module listener).
func newPlanService(bridge planBridge, leaseID string) (*pluginhostsdk.Router, error) {
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
		PackageID: "plan", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := planRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(), NativeOnly: true,
	})
}

// planRoutes are the package's compatibility routes with a native handler.
var planRoutes = map[string]struct{}{
	"plan.admin.plans.get":            {},
	"plan.admin.plans.post":           {},
	"plan.admin.plans.id.get":         {},
	"plan.admin.plans.id.put":         {},
	"plan.admin.plans.id.delete":      {},
	"plan.admin.plans.id.assign.post": {},
	"plan.user.plan.get":              {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler. There are none left: the speed-limit routes are Flux forward limits, whose rows name
// forward tunnels, and moved to the forward package.
var bridgedRoutes = map[string]struct{}{}
