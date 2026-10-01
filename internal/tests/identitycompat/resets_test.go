package identitycompat

import (
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withTraffic gives the member used traffic to reset.
func withTraffic(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).UpdateColumns(map[string]any{"transfer_enable": 1000, "u": 100, "d": 300}).Error)
}

// resetState is what a reset leaves in Control: the counters, whether each
// subscription token is still the seeded one (a new one is random), the
// proxy uuids, the request ledger, the change log and revocations.
func resetState(t testing.TB, db *gorm.DB) any {
	var users []struct {
		ID    uint
		U     int64
		D     int64
		Token string
		UUID  string
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	type shown struct {
		ID         uint
		U, D       int64
		NewToken   bool
		UUID       string
		TokenEmpty bool
	}
	subscribers := make([]shown, 0, len(users))
	for _, user := range users {
		subscribers = append(subscribers, shown{
			ID: user.ID, U: user.U, D: user.D, NewToken: user.Token != fmt.Sprintf("t%d", user.ID), UUID: user.UUID, TokenEmpty: user.Token == "",
		})
	}
	var requests []struct {
		RequestID string
		Method    string
		UserID    uint
		Result    string
	}
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Order("request_id").Find(&requests).Error)
	var changes []struct {
		UserID  uint
		Deleted bool
	}
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Find(&changes).Error)
	var revocations int64
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Count(&revocations).Error)
	return map[string]any{"subscribers": subscribers, "requests": requests, "changes": changes, "revocations": revocations}
}

func runResets(t *testing.T, route packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		c.Seed = seedUsers(nil, withTraffic)
		c.Principal = admin
		c.Snapshot = resetState
		c.RequestHeaders = map[string]string{"Idempotency-Key": "reset-1"}
		packagecompat.RunWrite(t, route, c)
	}
}

func TestAdminResetTrafficParity(t *testing.T) {
	route := adminRoute("POST", "/api/v2/admin/users/:id/reset-traffic", native.ResetTrafficRouteID, (*handler.AdminHandler).ResetUserTraffic)
	runResets(t, route, []packagecompat.Case{
		{Name: "reset", Path: "/api/v2/admin/users/2/reset-traffic"},
		{Name: "retried request applies once", Path: "/api/v2/admin/users/2/reset-traffic", Warmup: [][]byte{nil}},
		{Name: "unknown user", Path: "/api/v2/admin/users/99/reset-traffic"},
		{Name: "id zero", Path: "/api/v2/admin/users/0/reset-traffic"},
		{Name: "invalid id", Path: "/api/v2/admin/users/x/reset-traffic"},
	})
}

func TestAdminResetSubscribeParity(t *testing.T) {
	route := adminRoute("POST", "/api/v2/admin/users/:id/reset-subscribe", native.ResetSubscribeRouteID, (*handler.AdminHandler).ResetUserSubscribe)
	issued := []string{"data.token"}
	runResets(t, route, []packagecompat.Case{
		{Name: "reset", Path: "/api/v2/admin/users/2/reset-subscribe", Mask: issued},
		{Name: "retried request applies once", Path: "/api/v2/admin/users/2/reset-subscribe", Mask: issued, Warmup: [][]byte{nil}},
		{Name: "unknown user", Path: "/api/v2/admin/users/99/reset-subscribe"},
		{Name: "id zero", Path: "/api/v2/admin/users/0/reset-subscribe"},
		{Name: "invalid id", Path: "/api/v2/admin/users/x/reset-subscribe"},
	})
}
