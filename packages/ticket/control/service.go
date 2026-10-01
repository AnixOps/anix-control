package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/ticket/native"
	"gorm.io/gorm"
)

// ticketBridge is what the ticket host needs from the package bridge.
type ticketBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newTicketService returns the ticket host's router. Every route has a
// native handler on the adopted v2_ticket and v2_ticket_message tables; a
// route serves natively once the kernel sets its mode, and falls back to the
// legacy handler otherwise.
func newTicketService(bridge ticketBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "ticket", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, allowed := ticketRoutes[routeID]
			return allowed
		},
		Native: service.Handlers(),
	})
}

// ticketRoutes are the package's compatibility routes; each has a native
// handler.
var ticketRoutes = map[string]struct{}{
	"ticket.admin.ticket.get":           {},
	"ticket.admin.ticket.reply.post":    {},
	"ticket.admin.ticket.id.close.post": {},
	"ticket.user.ticket.get":            {},
	"ticket.user.ticket.post":           {},
	"ticket.user.ticket.id.get":         {},
	"ticket.user.ticket.id.reply.post":  {},
	"ticket.user.ticket.id.close.post":  {},
}
