// Package ticketcompat proves the ticket package's native routes answer
// exactly as the kernel's legacy handlers, on SQLite and PostgreSQL: the
// same bytes (creation times masked) and the same resulting rows.
package ticketcompat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/ticket/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var (
	admin  = pluginhostsdk.Principal{ActorID: 1, Admin: true}
	member = pluginhostsdk.Principal{ActorID: 2}
	other  = pluginhostsdk.Principal{ActorID: 3}
)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.User{}, &model.Ticket{}, &model.TicketMessage{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// seed writes tickets of two members: open, answered and closed.
func seed(t testing.TB, db *gorm.DB) {
	seedUsers(t, db)
	require.NoError(t, db.Create(&[]model.Ticket{
		{ID: 1, UserID: 2, Subject: "open", Level: 1, Status: 0, CreatedAt: seeded, UpdatedAt: seeded.Add(3 * time.Hour)},
		{ID: 2, UserID: 2, Subject: "closed", Level: 2, Status: 2, CreatedAt: seeded.Add(time.Hour), UpdatedAt: seeded.Add(time.Hour)},
		{ID: 3, UserID: 3, Subject: "other", Level: 0, Status: 1, CreatedAt: seeded.Add(2 * time.Hour), UpdatedAt: seeded.Add(2 * time.Hour)},
	}).Error)
	require.NoError(t, db.Create(&[]model.TicketMessage{
		{ID: 1, TicketID: 1, UserID: 2, Message: "help", CreatedAt: seeded},
		{ID: 2, TicketID: 1, UserID: 1, Message: "on it", IsAdmin: 1, CreatedAt: seeded.Add(time.Hour)},
		{ID: 3, TicketID: 3, UserID: 3, Message: "hi", CreatedAt: seeded},
	}).Error)
	if db.Name() == "postgres" {
		for _, table := range []string{"v2_ticket", "v2_ticket_message"} {
			require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence(?, 'id'), (SELECT MAX(id) FROM "+table+"))", table).Error)
		}
	}
}

// seedUsers writes the members tickets refer to (PostgreSQL enforces the
// kernel's foreign key).
func seedUsers(t testing.TB, db *gorm.DB) {
	for id := uint(1); id <= 3; id++ {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id)}).Error)
	}
}

// rows is the state after a write, without the times handlers take from
// their clock.
func rows(t testing.TB, db *gorm.DB) any {
	var tickets []struct {
		ID      uint
		UserID  uint
		Subject string
		Level   int
		Status  int
	}
	require.NoError(t, db.Model(&model.Ticket{}).Order("id").Find(&tickets).Error)
	var messages []struct {
		ID       uint
		TicketID uint
		UserID   uint
		Message  string
		IsAdmin  int
	}
	require.NoError(t, db.Model(&model.TicketMessage{}).Order("id").Find(&messages).Error)
	return map[string]any{"tickets": tickets, "messages": messages}
}

func read(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seed
		}
		packagecompat.RunRead(t, r, c)
	}
}

func write(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Seed, c.Snapshot = seed, rows
		packagecompat.RunWrite(t, r, c)
	}
}

func TestUserRoutesParity(t *testing.T) {
	user := handler.NewTicketHandler()
	read(t, route("GET", "/api/v2/user/ticket", "ticket.user.ticket.get", user.GetTickets), []packagecompat.Case{
		{Name: "own tickets by update time", Path: "/api/v2/user/ticket", Principal: member},
		{Name: "no tickets answers []", Path: "/api/v2/user/ticket", Principal: pluginhostsdk.Principal{ActorID: 9}},
	})
	read(t, route("GET", "/api/v2/user/ticket/:id", "ticket.user.ticket.id.get", user.GetTicket), []packagecompat.Case{
		{Name: "own ticket with messages", Path: "/api/v2/user/ticket/1", Principal: member},
		{Name: "own ticket without messages", Path: "/api/v2/user/ticket/2", Principal: member},
		{Name: "another member's ticket", Path: "/api/v2/user/ticket/3", Principal: member},
		{Name: "invalid id", Path: "/api/v2/user/ticket/x", Principal: member},
	})
	write(t, route("POST", "/api/v2/user/ticket", "ticket.user.ticket.post", user.CreateTicket), []packagecompat.Case{
		{Name: "create", Path: "/api/v2/user/ticket", Principal: member, Mask: []string{"data.created_at", "data.updated_at"},
			Body: []byte(`{"subject":"new","level":2,"message":"first"}`)},
		{Name: "missing subject", Path: "/api/v2/user/ticket", Principal: member, Body: []byte(`{"message":"first"}`)},
	})
	write(t, route("POST", "/api/v2/user/ticket/:id/reply", "ticket.user.ticket.id.reply.post", user.ReplyTicket), []packagecompat.Case{
		{Name: "reply reopens", Path: "/api/v2/user/ticket/1/reply", Principal: member, Body: []byte(`{"message":"more"}`)},
		{Name: "closed ticket", Path: "/api/v2/user/ticket/2/reply", Principal: member, Body: []byte(`{"message":"more"}`)},
		{Name: "another member's ticket", Path: "/api/v2/user/ticket/3/reply", Principal: member, Body: []byte(`{"message":"more"}`)},
		{Name: "missing message", Path: "/api/v2/user/ticket/1/reply", Principal: member, Body: []byte(`{}`)},
	})
	write(t, route("POST", "/api/v2/user/ticket/:id/close", "ticket.user.ticket.id.close.post", user.CloseTicket), []packagecompat.Case{
		{Name: "close", Path: "/api/v2/user/ticket/1/close", Principal: member},
		{Name: "another member's ticket", Path: "/api/v2/user/ticket/1/close", Principal: other},
		{Name: "invalid id", Path: "/api/v2/user/ticket/x/close", Principal: member},
	})
}

func TestAdminRoutesParity(t *testing.T) {
	adminHandler := handler.NewAdminTicketHandler()
	read(t, route("GET", "/api/v2/admin/ticket", "ticket.admin.ticket.get", adminHandler.GetTickets), []packagecompat.Case{
		{Name: "every ticket by creation time", Path: "/api/v2/admin/ticket", Principal: admin},
		{Name: "no tickets answers []", Path: "/api/v2/admin/ticket", Principal: admin, Seed: seedUsers},
	})
	write(t, route("POST", "/api/v2/admin/ticket/reply", "ticket.admin.ticket.reply.post", adminHandler.ReplyTicket), []packagecompat.Case{
		{Name: "reply answers", Path: "/api/v2/admin/ticket/reply", Principal: admin, Body: []byte(`{"ticket_id":1,"message":"done"}`)},
		{Name: "unknown ticket", Path: "/api/v2/admin/ticket/reply", Principal: admin, Body: []byte(`{"ticket_id":99,"message":"done"}`)},
		{Name: "missing message", Path: "/api/v2/admin/ticket/reply", Principal: admin, Body: []byte(`{"ticket_id":1}`)},
	})
	write(t, route("POST", "/api/v2/admin/ticket/:id/close", "ticket.admin.ticket.id.close.post", adminHandler.CloseTicket), []packagecompat.Case{
		{Name: "close", Path: "/api/v2/admin/ticket/3/close", Principal: admin},
		{Name: "unknown ticket", Path: "/api/v2/admin/ticket/99/close", Principal: admin},
		{Name: "invalid id", Path: "/api/v2/admin/ticket/x/close", Principal: admin},
	})
}
