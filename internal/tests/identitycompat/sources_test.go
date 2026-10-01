package identitycompat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// controlWithIdentity is one database holding Control's tables, seeded, and
// identity's, imported from them, with the native service on it.
func controlWithIdentity(t *testing.T) (*gorm.DB, *native.Service, *native.Stores) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "control.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(Models...))
	seedUsers(nil, withSubscription)(t, db)
	service := nativeService(db)
	stores, err := service.Open(context.Background())
	require.NoError(t, err)
	return db, service, stores
}

func answer(t *testing.T, service *native.Service, route string, principal pluginhostsdk.Principal, params map[string]string, headers map[string][]string) map[string]any {
	t.Helper()
	response, err := service.Handlers()[route](context.Background(), pluginhostsdk.NativeRequest{
		RouteID: route, Principal: principal, Metadata: pluginhostsdk.RequestMetadata{PathParams: params, Headers: headers},
	})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &decoded), "%s", response.Body)
	return decoded
}

func data(t *testing.T, decoded map[string]any) map[string]any {
	t.Helper()
	fields, ok := decoded["data"].(map[string]any)
	require.True(t, ok, "%v", decoded)
	return fields
}

// The account reads answer identity's account, not Control's projection of
// it: here identity changed the member's email and banned the MFA member
// without projecting either, so Control still holds the old values.
func TestAccountReadsTakeTheAccountFromIdentity(t *testing.T) {
	db, service, stores := controlWithIdentity(t)
	ctx := context.Background()
	renamed, banned := "identity@example.test", true
	_, err := stores.Accounts.Update(ctx, 2, account.Changes{Email: &renamed})
	require.NoError(t, err)
	_, err = stores.Accounts.Update(ctx, 5, account.Changes{Banned: &banned})
	require.NoError(t, err)
	var projected model.User
	require.NoError(t, db.Take(&projected, 2).Error)
	require.Equal(t, "member@example.test", projected.Email, "nothing was projected")

	profile := data(t, answer(t, service, native.ProfileRouteID, member, nil, nil))
	require.Equal(t, renamed, profile["email"])
	require.Equal(t, "t2", profile["token"], "the subscription token is Control's")
	require.Equal(t, "u2", profile["uuid"])

	dashboard := data(t, answer(t, service, native.DashboardRouteID, member, nil, nil))
	require.Equal(t, renamed, dashboard["subscription"].(map[string]any)["email"])
	require.Equal(t, "Pro", dashboard["subscription"].(map[string]any)["plan_name"])

	detail := data(t, answer(t, service, native.AdminUserRouteID, admin, map[string]string{"id": "2"}, nil))
	require.Equal(t, renamed, detail["email"])
	require.EqualValues(t, 250, detail["balance"], "entitlements are Control's")
	detail = data(t, answer(t, service, native.AdminUserRouteID, admin, map[string]string{"id": "5"}, nil))
	require.EqualValues(t, 1, detail["banned"])
}

// A subscriber identity has no account for is not a user to the user routes;
// the administrator still sees the subscriber as Control holds it.
func TestAccountReadsOfASubscriberWithoutAccount(t *testing.T) {
	_, service, stores := controlWithIdentity(t)
	require.NoError(t, stores.Accounts.Delete(context.Background(), 3))

	profile := answer(t, service, native.ProfileRouteID, pluginhostsdk.Principal{ActorID: 3}, nil, nil)
	require.Equal(t, "用户不存在", profile["msg"])
	dashboard := answer(t, service, native.DashboardRouteID, pluginhostsdk.Principal{ActorID: 3}, nil, nil)
	require.Equal(t, "用户不存在", dashboard["msg"])
	detail := data(t, answer(t, service, native.AdminUserRouteID, admin, map[string]string{"id": "3"}, nil))
	require.Equal(t, "banned@example.test", detail["email"])
	require.EqualValues(t, 1, detail["banned"])
}

// A retried subscription reset answers the token the first one issued, the
// one Control stores; a new request issues another.
func TestRetriedSubscriptionResetAnswersTheIssuedToken(t *testing.T) {
	db, service, _ := controlWithIdentity(t)
	reset := func(key string) string {
		decoded := answer(t, service, native.ResetSubscribeRouteID, admin, map[string]string{"id": "2"}, map[string][]string{"Idempotency-Key": {key}})
		token, ok := data(t, decoded)["token"].(string)
		require.True(t, ok, "%v", decoded)
		return token
	}
	first := reset("key-1")
	require.NotEqual(t, "t2", first)
	require.Equal(t, first, reset("key-1"))
	var stored model.User
	require.NoError(t, db.Take(&stored, 2).Error)
	require.Equal(t, first, stored.Token)
	require.NotEqual(t, first, reset("key-2"))
}
