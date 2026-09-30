package identitycompat

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func adminRoute(method, pattern, routeID string, legacy func(*handler.AdminHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Models: Models,
		Legacy: func(c *gin.Context) { legacy(handler.NewAdminHandler(), c) },
		Native: nativeRoute(routeID),
	}
}

// kernelState is what administration leaves in Control: subscribers, their
// revocations and the legacy MFA rows the credential mirror must keep.
func kernelState(t testing.TB, db *gorm.DB) any {
	var users []struct {
		ID             uint
		Email          string
		IsAdmin        int
		Banned         int
		PlanID         *uint
		GroupID        *uint
		TransferEnable int64
		SpeedLimit     *int64
		DeviceLimit    *int
		Balance        int64
		ExpiredAt      *int64
		FlowResetTime  int64
		RemarkContent  *string
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	var revocations []struct {
		UserID uint
		Reason string
	}
	require.NoError(t, db.Model(&model.IdentityRevocation{}).Order("user_id").Find(&revocations).Error)
	var mfas []struct {
		UserID     uint
		Enabled    bool
		TOTPSecret string `gorm:"column:totp_secret"`
	}
	require.NoError(t, db.Model(&model.UserMFA{}).Order("user_id").Find(&mfas).Error)
	return map[string]any{"users": users, "revocations": revocations, "mfa": mfas}
}

func runWrite(t *testing.T, route packagecompat.Route, cases []packagecompat.Case) {
	t.Helper()
	for _, c := range cases {
		if c.Seed == nil {
			c.Seed = seedUsers(nil, nil)
		}
		c.Principal = admin
		c.Snapshot = kernelState
		packagecompat.RunWrite(t, route, c)
	}
}

func TestAdminCreateUserParity(t *testing.T) {
	route := adminRoute("POST", "/api/v2/admin/users", "identity.admin.users.post", (*handler.AdminHandler).CreateUser)
	generated := []string{"data.uuid", "data.token", "data.created_at", "data.updated_at"}
	runWrite(t, route, []packagecompat.Case{
		{Name: "administrator with entitlements", Path: "/api/v2/admin/users", Mask: generated,
			Body: []byte(`{"email":"New.Admin@example.test","password":"secret1","is_admin":1,"group_id":3,"transfer_enable":100,"speed_limit":10,"device_limit":2,"flowResetTime":5}`)},
		{Name: "plain member", Path: "/api/v2/admin/users", Mask: generated, Body: []byte(`{"email":"plain@example.test","password":"secret1"}`)},
		{Name: "existing email", Path: "/api/v2/admin/users", Body: []byte(`{"email":"member@example.test","password":"secret1"}`)},
		{Name: "invalid email", Path: "/api/v2/admin/users", Body: []byte(`{"email":"nope","password":"secret1"}`)},
		{Name: "short password", Path: "/api/v2/admin/users", Body: []byte(`{"email":"x@example.test","password":"123"}`)},
	})
}

func withPlan(t testing.TB, db *gorm.DB) {
	speed := int64(50)
	require.NoError(t, db.Create(&model.Plan{ID: 7, Name: "Pro", GroupID: 4, TransferEnable: 20, SpeedLimit: &speed}).Error)
}

func TestAdminUpdateUserParity(t *testing.T) {
	route := adminRoute("PUT", "/api/v2/admin/users/:id", "identity.admin.users.id.put", (*handler.AdminHandler).UpdateUser)
	runWrite(t, route, []packagecompat.Case{
		{Name: "invalid id", Path: "/api/v2/admin/users/abc", Body: []byte(`{}`)},
		{Name: "unknown user", Path: "/api/v2/admin/users/99", Body: []byte(`{"banned":1}`)},
		{Name: "plan id zero", Path: "/api/v2/admin/users/2", Body: []byte(`{"plan_id":0}`)},
		{Name: "ban through update", Path: "/api/v2/admin/users/2", Body: []byte(`{"banned":1}`)},
		{Name: "email change", Path: "/api/v2/admin/users/2", Body: []byte(`{"email":"renamed@example.test"}`)},
		{Name: "password change keeps MFA", Path: "/api/v2/admin/users/5", Body: []byte(`{"password":"new-password"}`)},
		{Name: "password change without MFA", Path: "/api/v2/admin/users/2", Body: []byte(`{"password":"new-password"}`)},
		{Name: "promotion", Path: "/api/v2/admin/users/2", Body: []byte(`{"is_admin":1}`)},
		{Name: "entitlements only", Path: "/api/v2/admin/users/2", Body: []byte(`{"balance":500,"remark_content":"vip","expired_at":1900000000}`)},
		{Name: "plan fills group and traffic", Path: "/api/v2/admin/users/2", Seed: seedUsers(nil, withPlan), Body: []byte(`{"plan_id":7,"device_limit":3}`)},
		{Name: "unchanged values revoke nothing", Path: "/api/v2/admin/users/2", Body: []byte(`{"email":"member@example.test","banned":0,"is_admin":0}`)},
		{Name: "empty update", Path: "/api/v2/admin/users/2", Body: []byte(`{}`)},
	})
}

func TestAdminBanUnbanDeleteParity(t *testing.T) {
	ban := adminRoute("POST", "/api/v2/admin/users/:id/ban", "identity.admin.users.id.ban.post", (*handler.AdminHandler).BanUser)
	runWrite(t, ban, []packagecompat.Case{
		{Name: "ban", Path: "/api/v2/admin/users/2/ban"},
		{Name: "ban again", Path: "/api/v2/admin/users/3/ban"},
		{Name: "ban unknown", Path: "/api/v2/admin/users/99/ban"},
		{Name: "ban invalid id", Path: "/api/v2/admin/users/x/ban"},
	})
	unban := adminRoute("POST", "/api/v2/admin/users/:id/unban", "identity.admin.users.id.unban.post", (*handler.AdminHandler).UnbanUser)
	runWrite(t, unban, []packagecompat.Case{
		{Name: "unban", Path: "/api/v2/admin/users/3/unban"},
		{Name: "unban unknown", Path: "/api/v2/admin/users/99/unban"},
	})
	remove := adminRoute("DELETE", "/api/v2/admin/users/:id", "identity.admin.users.id.delete", (*handler.AdminHandler).DeleteUser)
	runWrite(t, remove, []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/users/2"},
		{Name: "delete unknown", Path: "/api/v2/admin/users/99"},
		{Name: "delete id zero", Path: "/api/v2/admin/users/0"},
		{Name: "delete invalid id", Path: "/api/v2/admin/users/x"},
	})
}
